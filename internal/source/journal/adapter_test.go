package journal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func stubJournal(t *testing.T, script string, capture *[]string) {
	t.Helper()
	prev := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if capture != nil {
			*capture = append([]string{name}, args...)
		}
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
	t.Cleanup(func() { commandContext = prev })
}

func recordJSON(i int) string {
	return fmt.Sprintf(
		`{"__REALTIME_TIMESTAMP":"%d","__MONOTONIC_TIMESTAMP":"%d","__CURSOR":"s=1;i=%d","_TRANSPORT":"stdout","_PID":"4242","MESSAGE":"line %d"}`,
		1792142671274265+int64(i), 4698621544+int64(i), i, i)
}

func TestAdapterUsesArgArrayWithoutShell(t *testing.T) {
	var argv []string
	stubJournal(t, "echo '"+recordJSON(1)+"'", &argv)
	it, err := New().Query(context.Background(), source.Query{
		Since:  time.UnixMicro(1792142671274265).UTC(),
		BootID: "30107f27-d01e-4e42-b248-46e4da81db1a",
		Limit:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	if len(argv) == 0 || argv[0] != "journalctl" {
		t.Fatalf("argv = %v", argv)
	}
	joined := strings.Join(argv, " ")
	for _, want := range []string{"--output=json", "--no-pager", "--since", "@1792142671.274265", "-b", "30107f27-d01e-4e42-b248-46e4da81db1a", "-n", "5"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q must contain %q", joined, want)
		}
	}
	for _, a := range argv {
		if strings.ContainsAny(a, " \t\n\"'|;&$<>") {
			t.Errorf("argv element %q looks like a shell string", a)
		}
	}

	ev, err := it.Next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Message != "line 1" || ev.Process.PID == nil || *ev.Process.PID != 4242 {
		t.Errorf("event = %+v", ev)
	}
}

func TestAdapterIteratorStopsAtLimit(t *testing.T) {
	lines := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		lines = append(lines, recordJSON(i))
	}
	path := filepath.Join(t.TempDir(), "many.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stubJournal(t, "cat "+path, nil)

	it, err := New().Query(context.Background(), source.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	seen := 0
	for {
		if _, err := it.Next(context.Background()); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("Next: %v", err)
		}
		seen++
		if seen > 50 {
			t.Fatal("the iterator ignored the limit")
		}
	}
	if seen != 10 {
		t.Fatalf("events = %d, want 10", seen)
	}
}

func TestAdapterEmptyResultIsSuccess(t *testing.T) {
	stubJournal(t, "true", nil)
	it, err := New().Query(context.Background(), source.Query{Since: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := it.Next(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("Next = %v, want io.EOF", err)
	}
	if err := it.Close(); err != nil {
		t.Fatal(err)
	}
	if st := New().Available(context.Background()); !st.Available || st.Err != nil {
		t.Fatalf("status = %+v", st)
	}
}

func TestAdapterClassifiesPermissionDenied(t *testing.T) {
	stubJournal(t, "echo 'Failed to open /var/log/journal: Permission denied' >&2; exit 1", nil)
	st := New().Available(context.Background())
	if st.Available {
		t.Fatalf("status = %+v", st)
	}
	var e *exit.Error
	if !errors.As(st.Err, &e) || e.Code != exit.CodeSourceUnavailable {
		t.Fatalf("err = %v", st.Err)
	}
	if !strings.Contains(st.Err.Error(), "Permission denied") {
		t.Errorf("the raw reason must survive: %v", st.Err)
	}

	it, err := New().Query(context.Background(), source.Query{})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	if _, err := it.Next(context.Background()); err == nil {
		t.Fatal("a failed query must surface an error, not an empty timeline")
	}
}

func TestAdapterCancellationStopsProcess(t *testing.T) {
	stubJournal(t, "sleep 5", nil)
	ctx, cancel := context.WithCancel(context.Background())
	it, err := New().Query(ctx, source.Query{Since: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	start := time.Now()
	if err := it.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Close took %s, want under 2s", elapsed)
	}
}

func TestAdapterAcceptsFixtureFileEnv(t *testing.T) {
	t.Setenv(fixtureEnv, "testdata/handmade.jsonl")
	stubJournal(t, "echo SHOULD_NOT_RUN; exit 1", nil)

	it, err := New().Query(context.Background(), source.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	var got []event.Event
	for {
		ev, err := it.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ev)
	}
	if len(got) != 3 {
		t.Fatalf("events = %d, want 3", len(got))
	}
	if got[0].Message != "hello" {
		t.Errorf("first message = %q", got[0].Message)
	}
	if got[1].Category != event.CategoryService {
		t.Errorf("second category = %q", got[1].Category)
	}
	var flagged bool
	for _, l := range got[2].Limitations {
		if l == event.LimitationMessageNonUTF8 {
			flagged = true
		}
	}
	if !flagged {
		t.Errorf("a byte array message must stay flagged: %v", got[2].Limitations)
	}
}

func TestAdapterFixtureFileMissingIsSourceUnavailable(t *testing.T) {
	t.Setenv(fixtureEnv, filepath.Join(t.TempDir(), "absent.jsonl"))
	stubJournal(t, "true", nil)
	st := New().Available(context.Background())
	if st.Available {
		t.Fatalf("status = %+v", st)
	}
	var e *exit.Error
	if !errors.As(st.Err, &e) || e.Code != exit.CodeSourceUnavailable {
		t.Fatalf("err = %v", st.Err)
	}
}
