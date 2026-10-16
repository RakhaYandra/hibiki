package journal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeparse"
)

const (
	fixtureEnv   = "HIBIKI_JOURNAL_FILE"
	waitDelay    = 2 * time.Second
	closeTimeout = 3 * time.Second
	eventBuffer  = 64
)

var errLimitReached = errors.New("event limit reached")

// Single seam for tests. Production always passes an argument array, never a
// shell string (PRD 13.2).
var commandContext = exec.CommandContext

type Adapter struct{}

func New() source.Source { return Adapter{} }

func (Adapter) Name() string { return sourceType }

func (Adapter) Available(ctx context.Context) source.Status {
	st := source.Status{Name: sourceType}

	if path := fixturePath(); path != "" {
		if _, err := os.Stat(path); err != nil {
			st.Detail = err.Error()
			st.Err = exit.SourceUnavailable("cannot read %s=%s: %v", fixtureEnv, path, err)
			return st
		}
		st.Available = true
		st.Detail = "reading " + path + " from " + fixtureEnv
		return st
	}

	if _, err := exec.LookPath("journalctl"); err != nil {
		st.Detail = "journalctl is not on PATH"
		st.Err = exit.SourceUnavailable("journalctl is not installed or not on PATH")
		return st
	}

	it, err := Adapter{}.Query(ctx, source.Query{Limit: 1})
	if err != nil {
		st.Detail = err.Error()
		st.Err = err
		return st
	}
	defer it.Close()

	if _, err := it.Next(ctx); err != nil && !errors.Is(err, io.EOF) {
		st.Detail = err.Error()
		st.Err = err
		if isUnavailable(err) {
			return st
		}
		st.Available = true
		return st
	}
	st.Available = true
	st.Detail = "journal is readable"
	return st
}

func (Adapter) Query(ctx context.Context, q source.Query) (source.Iterator, error) {
	ctx, cancel := context.WithCancel(ctx)
	it := &iterator{
		ctx:    ctx,
		cancel: cancel,
		items:  make(chan item, eventBuffer),
		done:   make(chan struct{}),
		limit:  q.Limit,
	}

	if path := fixturePath(); path != "" {
		f, err := os.Open(path)
		if err != nil {
			cancel()
			return nil, exit.SourceUnavailable("cannot read %s=%s: %v", fixtureEnv, path, err)
		}
		it.file = f
		it.out = f
		go it.pump()
		return it, nil
	}

	cmd := commandContext(ctx, "journalctl", journalArgs(q)...)
	cmd.WaitDelay = waitDelay
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, classify("journalctl", err, "")
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, classify("journalctl", err, stderr.String())
	}
	it.cmd = cmd
	it.stderr = stderr
	it.out = stdout
	go it.pump()
	return it, nil
}

// -n is only an optimisation: journalctl returns the newest N records of the
// window, and the iterator still enforces the bound itself (PRD 14 strategy 5).
func journalArgs(q source.Query) []string {
	args := []string{"--output=json", "--no-pager"}
	if !q.Since.IsZero() {
		args = append(args, "--since", timeparse.FormatJournalArg(q.Since))
	}
	if !q.Until.IsZero() {
		args = append(args, "--until", timeparse.FormatJournalArg(q.Until))
	}
	if q.BootID != "" {
		// journalctl accepts the 32 hex form only. The event model carries the
		// dashed uuid a reader recognizes, so the adapter undoes its own
		// formatting here instead of leaking it to journalctl.
		args = append(args, "-b", strings.ToLower(strings.ReplaceAll(q.BootID, "-", "")))
	}
	if q.Limit > 0 {
		args = append(args, "-n", strconv.Itoa(q.Limit))
	}
	return args
}

type item struct {
	ev event.Event
}

type iterator struct {
	ctx    context.Context
	cancel context.CancelFunc
	items  chan item
	done   chan struct{}

	limit  int
	cmd    *exec.Cmd
	file   *os.File
	stderr *bytes.Buffer
	out    io.Reader

	mu  sync.Mutex
	err error
}

func (it *iterator) Next(ctx context.Context) (event.Event, error) {
	select {
	case <-ctx.Done():
		return event.Event{}, ctx.Err()
	case got, ok := <-it.items:
		if !ok {
			if err := it.finalErr(); err != nil {
				return event.Event{}, err
			}
			return event.Event{}, io.EOF
		}
		return got.ev, nil
	}
}

func (it *iterator) Close() error {
	it.cancel()
	select {
	case <-it.done:
	case <-time.After(closeTimeout):
		if it.cmd != nil && it.cmd.Process != nil {
			_ = it.cmd.Process.Kill()
		}
		<-it.done
	}
	return nil
}

func (it *iterator) pump() {
	defer close(it.done)
	defer close(it.items)

	sent := 0
	malformed := 0
	firstMalformedLine := 0
	var firstMalformedErr error
	err := Decode(it.out, func(fields map[string]string) error {
		ev, nerr := Normalize(fields)
		if nerr != nil {
			return nerr
		}
		if it.limit > 0 && sent >= it.limit {
			return errLimitReached
		}
		sent++
		select {
		case it.items <- item{ev: ev}:
			return nil
		case <-it.ctx.Done():
			return it.ctx.Err()
		}
	}, func(line int, merr error) {
		if malformed == 0 {
			firstMalformedLine, firstMalformedErr = line, merr
		}
		malformed++
	})
	limited := errors.Is(err, errLimitReached)
	if limited {
		it.cancel()
		err = nil
	}
	if it.file != nil {
		_ = it.file.Close()
	}
	if it.cmd != nil {
		waitErr := it.cmd.Wait()
		if err == nil && waitErr != nil && it.ctx.Err() == nil {
			err = classify("journalctl", waitErr, it.stderr.String())
		}
	}
	// Damaged lines do not end the query: the events that did decode are still
	// an answer. Only a stream with nothing usable at all is bad data.
	if err == nil && !limited && malformed > 0 {
		if sent == 0 {
			err = exit.BadData("malformed journal record on line %d: %v", firstMalformedLine, firstMalformedErr)
		} else {
			err = &source.Limitation{Message: "malformed_records:" + strconv.Itoa(malformed)}
		}
	}
	it.setErr(err)
}

func (it *iterator) setErr(err error) {
	it.mu.Lock()
	it.err = err
	it.mu.Unlock()
}

func (it *iterator) finalErr() error {
	it.mu.Lock()
	defer it.mu.Unlock()
	return it.err
}

func classify(tool string, err error, stderr string) error {
	var exitErr *exec.ExitError
	msg := strings.TrimSpace(stderr)
	switch {
	case strings.Contains(msg, "Permission denied"):
		return exit.SourceUnavailable(
			"%s cannot read the journal: %s (check journal read access: systemd-journal or adm group membership, or a read ACL on /var/log/journal)",
			tool, msg)
	case errors.As(err, &exitErr):
		if msg != "" {
			return exit.SourceUnavailable("%s failed: %s", tool, msg)
		}
		return exit.SourceUnavailable("%s exited with status %d", tool, exitErr.ExitCode())
	case errors.Is(err, exec.ErrNotFound):
		return exit.SourceUnavailable("%s is not installed or not on PATH", tool)
	default:
		return exit.SourceUnavailable("%s could not be run: %v", tool, err)
	}
}

func isUnavailable(err error) bool {
	var e *exit.Error
	return errors.As(err, &e) && e.Code == exit.CodeSourceUnavailable
}

func fixturePath() string { return os.Getenv(fixtureEnv) }
