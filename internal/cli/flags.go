package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/output"
	"github.com/RakhaYandra/hibiki/internal/session"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/source/journal"
	"github.com/RakhaYandra/hibiki/internal/source/systemd"
	"github.com/RakhaYandra/hibiki/internal/timeline"
	"github.com/RakhaYandra/hibiki/internal/timeparse"
	"github.com/spf13/cobra"
)

const (
	defaultSince = "30m"
	defaultLimit = 200
)

// Process level wiring lives behind these seams so command logic stays a pure
// function of its flags.
var (
	newSources   = func() []source.Source { return []source.Source{journal.New()} }
	lookupUnit   = systemd.UnitState
	nowFunc      = time.Now
	saveSnapshot = func(res timeline.Result) (string, error) {
		return session.Save(session.DefaultDir(), res)
	}
)

type filterFlags struct {
	Since    string
	Until    string
	Category string
	Type     string
	Severity string
	Service  string
	PID      int
	Query    string
	Boot     string
	Limit    int
	Cursor   string
	Format   string
	Save     string

	pidSet bool
}

func addFilterFlags(cmd *cobra.Command) *filterFlags {
	f := &filterFlags{}
	flags := cmd.Flags()
	flags.StringVar(&f.Since, "since", defaultSince, "start of the window: a span (30m, 2h, 1h30m, 2d) or an RFC 3339 timestamp")
	flags.StringVar(&f.Until, "until", "", "end of the window, same syntax as --since (default: now)")
	flags.StringVar(&f.Category, "category", "", "comma separated categories: system, service, application, process, security, unknown")
	flags.StringVar(&f.Type, "type", "", "alias for --category")
	flags.StringVar(&f.Severity, "severity", "", "comma separated severities: debug, info, warning, error, critical, unknown")
	flags.StringVar(&f.Service, "service", "", "comma separated systemd unit names")
	flags.IntVar(&f.PID, "pid", 0, "only events from this process id")
	flags.StringVar(&f.Query, "query", "", "case insensitive keyword in the message")
	flags.StringVar(&f.Boot, "boot", "", "only events from this boot id")
	flags.IntVar(&f.Limit, "limit", defaultLimit, "maximum number of events to return, 0 means no limit")
	flags.StringVar(&f.Cursor, "cursor", "", "continue a truncated result from this cursor")
	flags.StringVar(&f.Format, "format", "text", "output format: text or json")
	flags.StringVar(&f.Save, "save", "", "write the full result envelope to this file")

	previous := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		f.pidSet = c.Flags().Changed("pid")
		if previous != nil {
			return previous(c, args)
		}
		return nil
	}
	return f
}

func (f *filterFlags) toFilter(now time.Time) (timeline.Filter, error) {
	var filter timeline.Filter

	since, err := timeparse.ParseSince(f.Since, now)
	if err != nil {
		return filter, err
	}
	filter.Since = since

	if strings.TrimSpace(f.Until) != "" {
		until, err := timeparse.ParseUntil(f.Until, now)
		if err != nil {
			return filter, err
		}
		if until.Before(since) {
			return filter, exit.Usage("--until %s is before --since %s", until.Format(time.RFC3339), since.Format(time.RFC3339))
		}
		filter.Until = until
	}

	categories, err := parseCategories(f.Category, f.Type)
	if err != nil {
		return filter, err
	}
	filter.Categories = categories

	severities, err := parseSeverities(f.Severity)
	if err != nil {
		return filter, err
	}
	filter.Severities = severities

	filter.Units = splitCSV(f.Service)
	if f.pidSet && f.PID <= 0 {
		return filter, exit.Usage("--pid must be a positive process id, got %d", f.PID)
	}
	if f.PID > 0 {
		filter.PIDs = []int{f.PID}
	}
	if f.Limit < 0 {
		return filter, exit.Usage("--limit must not be negative, got %d", f.Limit)
	}
	if f.Format != "text" && f.Format != "json" {
		return filter, exit.Usage("--format must be text or json, got %q", f.Format)
	}
	filter.Keyword = f.Query
	if f.Boot != "" {
		bootID, err := event.NormalizeBootID(f.Boot)
		if err != nil {
			return filter, exit.Usage("--boot %q is not a boot id: %v", f.Boot, err)
		}
		filter.BootID = bootID
	}
	return filter, nil
}

func runQuery(cmd *cobra.Command, f *filterFlags) (timeline.Result, error) {
	res, now, err := fetchQuery(cmd, f)
	if err != nil {
		return res, err
	}
	if f.Format == "json" {
		return res, output.RenderJSON(cmd.OutOrStdout(), res)
	}
	return res, output.RenderText(cmd.OutOrStdout(), res, output.TextOptions{Color: isTTY(cmd.OutOrStdout()), Now: now})
}

// fetchQuery is the whole query lifecycle up to the answer: filter, read,
// snapshot, --save. A command that renders something else from the same events
// (incidents) shares it, so both surfaces agree on when a query is unreadable.
func fetchQuery(cmd *cobra.Command, f *filterFlags) (timeline.Result, time.Time, error) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	now := nowFunc()

	filter, err := f.toFilter(now)
	if err != nil {
		return timeline.Result{}, now, err
	}
	warnUnknownUnits(ctx, cmd, filter.Units)

	res, err := timeline.Run(ctx, newSources(), filter, f.Limit, f.Cursor)
	if err != nil {
		return timeline.Result{}, now, err
	}
	// Nothing readable and nothing observed means the query has no answer to
	// give: the source error becomes the command's error, so the exit code
	// says why instead of reporting a quiet empty result.
	if err := unreadableSourceError(res); err != nil {
		return res, now, err
	}

	// A snapshot that cannot be written never fails the query: the answer the
	// user asked for is already in hand.
	if _, err := saveSnapshot(res); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not save the query snapshot: %v\n", err)
	}
	if f.Save != "" {
		if err := writeEnvelopeFile(f.Save, res); err != nil {
			return res, now, err
		}
	}
	return res, now, nil
}

// A unit the user names may be a system unit or a user unit, so both scopes are
// checked before saying it is unknown (spike 5). A warning never fails the
// query and never goes to stdout, so --format json stays clean.
func warnUnknownUnits(ctx context.Context, cmd *cobra.Command, units []string) {
	for _, unit := range units {
		if !unitExists(ctx, unit) {
			fmt.Fprintf(cmd.ErrOrStderr(), "unit not found: %s\n", unit)
		}
	}
}

func unitExists(ctx context.Context, unit string) bool {
	for _, userScope := range []bool{false, true} {
		state, exists, err := lookupUnit(ctx, unit, userScope)
		if err != nil {
			return true
		}
		if exists && state != "not-found" {
			return true
		}
	}
	return false
}

// Nothing readable and nothing observed means the query has no answer to give,
// so the source error becomes the command error and the exit code stays honest.
func unreadableSourceError(res timeline.Result) error {
	if len(res.Events) > 0 || len(res.Sources) == 0 {
		return nil
	}
	for _, s := range res.Sources {
		if s.Available || s.Err == nil {
			return nil
		}
	}
	return res.Sources[0].Err
}

func writeEnvelopeFile(path string, res timeline.Result) error {
	// Streamed to the file for the same reason the JSON output is streamed: a
	// second copy of the whole result must not live in memory (PRD 14).
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := output.RenderJSON(f, res); err != nil {
		f.Close()
		return fmt.Errorf("cannot encode the result: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func parseCategories(categoryFlag, typeFlag string) ([]event.Category, error) {
	fromCategory, err := categoryList(categoryFlag)
	if err != nil {
		return nil, err
	}
	fromType, err := categoryList(typeFlag)
	if err != nil {
		return nil, err
	}
	if len(fromCategory) > 0 && len(fromType) > 0 && !sameCategories(fromCategory, fromType) {
		return nil, exit.Usage("--type and --category disagree, and --type is only an alias for --category")
	}
	if len(fromCategory) > 0 {
		return fromCategory, nil
	}
	return fromType, nil
}

func categoryList(value string) ([]event.Category, error) {
	parts := splitCSV(value)
	out := make([]event.Category, 0, len(parts))
	for _, p := range parts {
		c := event.Category(p)
		if !c.Valid() {
			return nil, exit.Usage("unknown category %q: use system, service, application, process, security or unknown", p)
		}
		out = append(out, c)
	}
	return out, nil
}

func sameCategories(a, b []event.Category) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[event.Category]bool{}
	for _, c := range a {
		seen[c] = true
	}
	for _, c := range b {
		if !seen[c] {
			return false
		}
	}
	return true
}

func parseSeverities(value string) ([]event.Severity, error) {
	parts := splitCSV(value)
	out := make([]event.Severity, 0, len(parts))
	for _, p := range parts {
		s := event.Severity(p)
		if !s.Valid() {
			return nil, exit.Usage("unknown severity %q: use debug, info, warning, error, critical or unknown", p)
		}
		out = append(out, s)
	}
	return out, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
