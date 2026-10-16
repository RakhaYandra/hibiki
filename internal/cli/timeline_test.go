package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func TestTimelineDefaultWindowIs30m(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	src := &recordingSource{name: "test", evs: manyEvents(3)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: now}, "timeline")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if len(src.queries) != 1 {
		t.Fatalf("queries = %d", len(src.queries))
	}
	if got := now.Sub(src.queries[0].Since); got < 29*time.Minute || got > 31*time.Minute {
		t.Fatalf("default window = %s, want about 30m", got)
	}
	if src.queries[0].Since.Location() != time.UTC {
		t.Errorf("the query window must be UTC, got %s", src.queries[0].Since.Location())
	}
}

func TestTimelineRejectsBadSpanWithExit2(t *testing.T) {
	res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test"}}}, "timeline", "--since", "30m ago")
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr = %s)", res.code, res.stderr)
	}
	for _, want := range []string{"30m", "2h", "RFC 3339"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr must name the accepted formats (%q missing): %s", want, res.stderr)
		}
	}
}

func TestTimelineRejectsBadFormatAndNegativeLimit(t *testing.T) {
	for _, args := range [][]string{
		{"timeline", "--format", "yaml"},
		{"timeline", "--limit", "-5"},
		{"timeline", "--since", "1h", "--until", "2h ago"},
		{"timeline", "--cursor", "!!!"},
	} {
		res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test"}}}, args...)
		if res.code != 2 {
			t.Errorf("%v: exit = %d, want 2 (stderr = %s)", args, res.code, res.stderr)
		}
	}
}

func TestTimelineRendersEventsInOrder(t *testing.T) {
	// A journal source yields ascending records; the engine must still merge two
	// sources into one chronological view.
	early := &recordingSource{name: "early", evs: []event.Event{
		testEvent(at(-30*time.Minute), "first", event.CategorySystem, event.SeverityInfo),
		testEvent(at(-10*time.Minute), "third", event.CategorySystem, event.SeverityInfo),
	}}
	late := &recordingSource{name: "late", evs: []event.Event{
		testEvent(at(-20*time.Minute), "second", event.CategorySystem, event.SeverityInfo),
	}}
	res := runCLI(t, cliDeps{sources: []source.Source{early, late}, now: testNow}, "timeline", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	first, second, third := strings.Index(res.stdout, "first"), strings.Index(res.stdout, "second"), strings.Index(res.stdout, "third")
	if first < 0 || second < 0 || third < 0 || !(first < second && second < third) {
		t.Fatalf("events must be chronological:\n%s", res.stdout)
	}
}

func TestTimelineCursorResumesWithoutRepeating(t *testing.T) {
	evs := manyEvents(5)
	src := &recordingSource{name: "test", evs: evs}
	first := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "timeline", "--limit", "2", "--since", "1h")
	if first.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", first.code, first.stderr)
	}
	cursor := ""
	for _, field := range strings.Fields(first.stdout) {
		if strings.HasPrefix(field, "ey") {
			cursor = field
		}
	}
	if cursor == "" {
		t.Fatalf("no cursor for the user to continue with:\n%s", first.stdout)
	}
	second := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "timeline", "--limit", "2", "--since", "1h", "--cursor", cursor)
	if second.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", second.code, second.stderr)
	}
	if !strings.Contains(second.stdout, "evt-002") || !strings.Contains(second.stdout, "evt-003") {
		t.Fatalf("resume must continue where it stopped:\n%s", second.stdout)
	}
	if strings.Contains(second.stdout, "evt-000") {
		t.Fatalf("resume must not repeat events:\n%s", second.stdout)
	}
}

func TestTimelineJSONFormatIsCleanOnStdout(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(2)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "timeline", "--format", "json", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var envelope struct {
		SchemaVersion int `json:"schema_version"`
		Events        []struct {
			ID string `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, res.stdout)
	}
	if envelope.SchemaVersion != 1 || len(envelope.Events) != 2 {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestTimelineNoColorWhenNotATerminal(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(1)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "timeline", "--since", "1h")
	if strings.Contains(res.stdout, "\x1b[") {
		t.Errorf("color must stay off for a pipe: %q", res.stdout)
	}
}

func TestTimelineSaveWritesRestrictedEnvelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hibiki.json")
	src := &recordingSource{name: "test", evs: manyEvents(2)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "timeline", "--since", "1h", "--save", path)
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("saved file is not an envelope: %v", err)
	}
	if len(env.Events) != 2 {
		t.Fatalf("saved events = %d", len(env.Events))
	}
}
