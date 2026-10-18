package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/session"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/source/journal"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func neighbourhood() []event.Event {
	main := testEvent(at(-5*time.Minute), "evt_main", event.CategoryService, event.SeverityWarning)
	main.Raw = map[string]string{"PRIORITY": "4", "_SYSTEMD_USER_UNIT": "example.service"}
	before := testEvent(at(-5*time.Minute-time.Second), "evt_before", event.CategoryService, event.SeverityInfo)
	after := testEvent(at(-5*time.Minute+time.Second), "evt_after", event.CategoryApplication, event.SeverityError)
	return []event.Event{before, main, after}
}

// The crash-loop fixture is read through the journal adapter so inspect sees
// the same events the CLI reads from a journal, not a hand-built copy.
func crashLoopEvents(t *testing.T) []event.Event {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "test", "fixtures", "regression", "crash-loop.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	evs := []event.Event{}
	if err := journal.Decode(f, func(fields map[string]string) error {
		ev, err := journal.Normalize(fields)
		if err != nil {
			return err
		}
		evs = append(evs, ev)
		return nil
	}, func(line int, err error) {
		t.Errorf("fixture line %d: %v", line, err)
	}); err != nil {
		t.Fatal(err)
	}
	if len(evs) != 6 {
		t.Fatalf("fixture events = %d, want the six of crash-loop.jsonl", len(evs))
	}
	return evs
}

func crashLoopInspect(t *testing.T, cursor, format string) cliResult {
	t.Helper()
	return crashLoopInspectContext(t, cursor, "2m", format)
}

func crashLoopInspectContext(t *testing.T, cursor, window, format string) cliResult {
	t.Helper()
	evs := crashLoopEvents(t)
	target := event.Event{}
	for _, ev := range evs {
		if ev.Source.Cursor != nil && *ev.Source.Cursor == cursor {
			target = ev
		}
	}
	if target.ID == "" {
		t.Fatalf("the fixture has no event at cursor %s", cursor)
	}
	path := saveEnvelope(t, timeline.Result{Events: evs})
	args := []string{"inspect", target.ID, "--file", path, "--context", window}
	if format != "" {
		args = append(args, "--format", format)
	}
	res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test", evs: evs}}, now: testNow}, args...)
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	return res
}

// The field block pads its name column, so the value is what is left of the line.
func fieldValue(out, name string) string {
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "  "+name+" "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func saveEnvelope(t *testing.T, res timeline.Result) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "hibiki")
	path, err := session.Save(dir, res)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectPrintsSectionsFromSavedFile(t *testing.T) {
	src := &recordingSource{name: "test", evs: neighbourhood()}
	path := saveEnvelope(t, timeline.Result{Events: neighbourhood()})
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow},
		"inspect", "evt_main", "--file", path, "--context", "2m")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	for _, want := range []string{"Event evt_main", "message evt_main", "Raw journal fields", "PRIORITY", "_SYSTEMD_USER_UNIT", "Before", "evt_before", "After", "evt_after", "Sources"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("inspect output lost %q:\n%s", want, res.stdout)
		}
	}
	if strings.Contains(res.stdout, "evt_far") {
		t.Errorf("unexpected event:\n%s", res.stdout)
	}
}

func TestInspectExcludesTheInspectedEventFromItsOwnSurroundings(t *testing.T) {
	src := &recordingSource{name: "test", evs: neighbourhood()}
	path := saveEnvelope(t, timeline.Result{Events: neighbourhood()})
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow},
		"inspect", "evt_main", "--file", path, "--context", "2m")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if strings.Count(res.stdout, "message evt_main") != 1 {
		t.Errorf("the event must appear once:\n%s", res.stdout)
	}
}

func TestInspectUnknownIDIsAnError(t *testing.T) {
	path := saveEnvelope(t, timeline.Result{Events: neighbourhood()})
	res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test"}}, now: testNow},
		"inspect", "evt_missing", "--file", path)
	if res.code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr = %s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "not in the last query") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestInspectWithoutSnapshotExplainsWhatToDo(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	src := &recordingSource{name: "test", evs: neighbourhood()}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "inspect", "evt_main")
	if res.code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr = %s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "no previous query") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestInspectUsesTheLastQueryByDefault(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	src := &recordingSource{name: "test", evs: neighbourhood()}
	save := func(res timeline.Result) (string, error) { return session.Save(session.DefaultDir(), res) }

	query := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow, save: save}, "timeline", "--since", "1h")
	if query.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", query.code, query.stderr)
	}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow, save: save}, "inspect", "evt_main")
	if res.code != 0 {
		t.Fatalf("inspect after a query: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "Event evt_main") {
		t.Errorf("stdout = %s", res.stdout)
	}
}

func TestInspectJSONFormatIsAnObject(t *testing.T) {
	src := &recordingSource{name: "test", evs: neighbourhood()}
	path := saveEnvelope(t, timeline.Result{Events: neighbourhood()})
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow},
		"inspect", "evt_main", "--file", path, "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var view struct {
		SchemaVersion int `json:"schema_version"`
		Event         struct {
			ID string `json:"id"`
		} `json:"event"`
		Before []struct {
			ID string `json:"id"`
		} `json:"before"`
		After       []any    `json:"after"`
		Limitations []string `json:"limitations"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &view); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, res.stdout)
	}
	if view.SchemaVersion != 1 || view.Event.ID != "evt_main" || len(view.Before) != 1 || len(view.After) != 1 {
		t.Fatalf("view = %+v", view)
	}
	if view.Limitations == nil {
		t.Error("limitations must be a list, never null")
	}
}

func TestInspectShowsTheIncidentsOfTheEvent(t *testing.T) {
	res := crashLoopInspect(t, "s=cl;i=3", "")
	for _, want := range []string{"Correlations (3)", "restart-loop [high]", "failed-then-error [medium]", "pid-errors [low]"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("inspect output lost %q:\n%s", want, res.stdout)
		}
	}
	if strings.Contains(res.stdout, "v0.3.0") {
		t.Errorf("the v0.3.0 placeholder is gone:\n%s", res.stdout)
	}
	ids := fieldValue(res.stdout, "correlation")
	if !strings.Contains(ids, "inc_") {
		t.Errorf("correlation = %q, want the incident ids the event belongs to:\n%s", ids, res.stdout)
	}
	after := strings.Index(res.stdout, "\nAfter")
	correlations := strings.Index(res.stdout, "\nCorrelations")
	sources := strings.Index(res.stdout, "\nSources")
	if after < 0 || correlations < after || sources < correlations {
		t.Errorf("section order wrong: after = %d, correlations = %d, sources = %d:\n%s", after, correlations, sources, res.stdout)
	}
}

func TestInspectNamesTheWindowEachRuleInspected(t *testing.T) {
	res := crashLoopInspect(t, "s=cl;i=3", "")
	windows := 0
	for _, line := range strings.Split(res.stdout, "\n") {
		rest, ok := strings.CutPrefix(line, "    window inspected ")
		if !ok {
			continue
		}
		windows++
		start, end, ok := strings.Cut(rest, " .. ")
		if !ok {
			t.Fatalf("window line = %q, want a start and an end", line)
		}
		from, err := time.Parse(time.RFC3339Nano, start)
		if err != nil {
			t.Errorf("window start %q: %v", start, err)
			continue
		}
		until, err := time.Parse(time.RFC3339Nano, end)
		if err != nil {
			t.Errorf("window end %q: %v", end, err)
			continue
		}
		if !until.After(from) {
			t.Errorf("window line = %q, want the end after the start", line)
		}
	}
	if windows != 3 {
		t.Errorf("window lines = %d, want one per incident:\n%s", windows, res.stdout)
	}
}

// A narrow context fetches less evidence than the rule claims to have looked
// at, so the reported window is the only way to tell the two apart.
func TestInspectReportsTheRuleWindowWithANarrowContext(t *testing.T) {
	res := crashLoopInspectContext(t, "s=cl;i=3", "5ms", "")
	if !strings.Contains(res.stdout, "window inspected ") {
		t.Errorf("the rule window must be reported even when the evidence is narrow:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, "Correlations (2)") {
		t.Errorf("a 5ms window leaves only the events 2ms apart:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, "restart-loop") {
		t.Errorf("three restart candidates never reached the window:\n%s", res.stdout)
	}
}

func TestInspectExplainsEveryIncidentItPrints(t *testing.T) {
	res := crashLoopInspect(t, "s=cl;i=3", "json")
	var doc struct {
		Correlations []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"correlations"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, res.stdout)
	}
	text := crashLoopInspect(t, "s=cl;i=3", "")
	for _, inc := range doc.Correlations {
		if inc.Reason == "" {
			t.Fatalf("incident %s carries no reason to print:\n%s", inc.ID, res.stdout)
		}
		if !strings.Contains(text.stdout, inc.Reason) {
			t.Errorf("the text left out the reason of %s (%q):\n%s", inc.ID, inc.Reason, text.stdout)
		}
	}
}

// Only the restart-loop incident covers the very first record of the fixture,
// so the report must narrow to what the inspected event takes part in.
func TestInspectReportsOnlyTheIncidentsOfTheEvent(t *testing.T) {
	res := crashLoopInspect(t, "s=cl;i=1", "")
	if !strings.Contains(res.stdout, "Correlations (1)") {
		t.Fatalf("the first record takes part in one incident:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, "restart-loop [high]") {
		t.Errorf("the incident is missing:\n%s", res.stdout)
	}
	for _, rule := range []string{"failed-then-error", "pid-errors"} {
		if strings.Contains(res.stdout, rule) {
			t.Errorf("rule %q covers no record of the first event:\n%s", rule, res.stdout)
		}
	}

	asJSON := crashLoopInspect(t, "s=cl;i=1", "json")
	var view struct {
		Correlations []struct {
			ID   string `json:"id"`
			Rule string `json:"rule"`
		} `json:"correlations"`
	}
	if err := json.Unmarshal([]byte(asJSON.stdout), &view); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, asJSON.stdout)
	}
	if len(view.Correlations) != 1 || view.Correlations[0].Rule != "restart-loop" {
		t.Errorf("correlations = %+v, want only the restart-loop incident", view.Correlations)
	}
}

func TestInspectWithoutAMatchingRulePrintsNone(t *testing.T) {
	src := &recordingSource{name: "test", evs: neighbourhood()}
	path := saveEnvelope(t, timeline.Result{Events: neighbourhood()})
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow},
		"inspect", "evt_main", "--file", path, "--context", "2m")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if got := fieldValue(res.stdout, "correlation"); got != "none" {
		t.Errorf("correlation = %q, want none:\n%s", got, res.stdout)
	}
	if strings.Contains(res.stdout, "Correlations") {
		t.Errorf("a section with no incident must not be printed:\n%s", res.stdout)
	}
}

func TestInspectJSONCarriesTheIncidentsOfTheEvent(t *testing.T) {
	res := crashLoopInspect(t, "s=cl;i=3", "json")
	type neighbour struct {
		ID             string   `json:"id"`
		CorrelationIDs []string `json:"correlation_ids"`
	}
	var view struct {
		Event struct {
			ID             string   `json:"id"`
			CorrelationIDs []string `json:"correlation_ids"`
		} `json:"event"`
		Before       []neighbour `json:"before"`
		After        []neighbour `json:"after"`
		Correlations []struct {
			ID         string      `json:"id"`
			Rule       string      `json:"rule"`
			Confidence string      `json:"confidence"`
			Reason     string      `json:"reason"`
			Primary    neighbour   `json:"primary"`
			Related    []neighbour `json:"related"`
		} `json:"correlations"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &view); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, res.stdout)
	}
	if len(view.Correlations) != 3 {
		t.Fatalf("correlations = %d, want the three the crash-loop fixture fires:\n%s", len(view.Correlations), res.stdout)
	}
	for _, inc := range view.Correlations {
		if inc.Rule != "restart-loop" && inc.Rule != "failed-then-error" && inc.Rule != "pid-errors" {
			t.Errorf("unexpected rule %q:\n%s", inc.Rule, res.stdout)
		}
		if inc.ID == "" || inc.Reason == "" || inc.Confidence == "" {
			t.Errorf("incident %+v lost its identity, its reason or its confidence", inc)
		}
	}
	if len(view.Event.CorrelationIDs) != 3 {
		t.Errorf("event.correlation_ids = %v, want one per incident:\n%s", view.Event.CorrelationIDs, res.stdout)
	}

	stamped := map[string][]string{}
	for _, ev := range append(append([]neighbour{}, view.Before...), view.After...) {
		stamped[ev.ID] = ev.CorrelationIDs
	}
	for _, inc := range view.Correlations {
		evidence := append([]neighbour{inc.Primary}, inc.Related...)
		for _, ev := range evidence {
			ids, neighbour := stamped[ev.ID]
			if !neighbour {
				continue
			}
			if !slices.Contains(ids, inc.ID) {
				t.Errorf("neighbour %s lost incident %s, got %v:\n%s", ev.ID, inc.ID, ids, res.stdout)
			}
		}
	}
}

func TestInspectJSONAlwaysCarriesAnIncidentsArray(t *testing.T) {
	src := &recordingSource{name: "test", evs: neighbourhood()}
	path := saveEnvelope(t, timeline.Result{Events: neighbourhood()})
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow},
		"inspect", "evt_main", "--file", path, "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var view struct {
		Correlations []any `json:"correlations"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &view); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, res.stdout)
	}
	if view.Correlations == nil {
		t.Error("correlations must be an array, never null")
	}
}

func TestInspectTextNeverAssertsCausality(t *testing.T) {
	res := crashLoopInspect(t, "s=cl;i=3", "")
	lowered := strings.ToLower(res.stdout)
	for _, banned := range []string{"because", "caused by", "—"} {
		if strings.Contains(lowered, banned) {
			t.Errorf("rendered output must not assert causality (%q):\n%s", banned, res.stdout)
		}
	}
}

func TestInspectRejectsBadContextWindowAndFormat(t *testing.T) {
	for _, args := range [][]string{
		{"inspect", "evt_main", "--context", "0s"},
		{"inspect", "evt_main", "--context", "-1m"},
		{"inspect", "evt_main", "--format", "yaml"},
	} {
		res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test"}}, now: testNow}, args...)
		if res.code != 2 {
			t.Errorf("%v: exit = %d, want 2 (stderr = %s)", args, res.code, res.stderr)
		}
	}
}

func TestInspectWarnsWhenTheSurroundingsAreUnavailable(t *testing.T) {
	broken := &recordingSource{name: "broken", err: errors.New("permission denied")}
	path := saveEnvelope(t, timeline.Result{Events: neighbourhood()})
	res := runCLI(t, cliDeps{sources: []source.Source{broken}, now: testNow},
		"inspect", "evt_main", "--file", path)
	if res.code != 0 {
		t.Fatalf("a missing snapshot of surroundings must not hide the event: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "broken") {
		t.Errorf("the limitation must name the source:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, "Raw journal fields") {
		t.Errorf("the event itself must still be shown:\n%s", res.stdout)
	}
}

func TestSnapshotFailureDoesNotFailTheQuery(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(1)}
	save := func(timeline.Result) (string, error) { return "", errors.New("read-only cache") }
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow, save: save}, "timeline", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "could not save the query snapshot") {
		t.Errorf("stderr = %q", res.stderr)
	}
}
