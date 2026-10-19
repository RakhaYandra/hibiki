package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func crashLoop(t *testing.T) {
	t.Helper()
	t.Setenv("HIBIKI_JOURNAL_FILE", filepath.Join("..", "..", "test", "fixtures", "regression", "crash-loop.jsonl"))
}

type incidentView struct {
	ID          string        `json:"id"`
	Rule        string        `json:"rule"`
	Confidence  string        `json:"confidence"`
	Reason      string        `json:"reason"`
	Primary     event.Event   `json:"primary"`
	Related     []event.Event `json:"related"`
	WindowStart time.Time     `json:"window_start"`
	WindowEnd   time.Time     `json:"window_end"`
}

type incidentsView struct {
	SchemaVersion int            `json:"schema_version"`
	Incidents     []incidentView `json:"incidents"`
	Limitations   []string       `json:"limitations"`
	Sources       []struct {
		Name      string `json:"name"`
		Available bool   `json:"available"`
	} `json:"sources"`
}

func TestIncidentsJSONHasIncidentsKey(t *testing.T) {
	crashLoop(t)
	res := runCLI(t, cliDeps{now: testNow}, "incidents", "--since", "24h", "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var view incidentsView
	if err := json.Unmarshal([]byte(res.stdout), &view); err != nil {
		t.Fatalf("stdout is not a Hibiki incidents document: %v\n%s", err, res.stdout)
	}
	if view.SchemaVersion != event.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", view.SchemaVersion, event.SchemaVersion)
	}
	if len(view.Incidents) != 3 {
		t.Fatalf("incidents = %d, want the three the crash-loop fixture fires:\n%s", len(view.Incidents), res.stdout)
	}
	want := []string{"restart-loop", "failed-then-error", "pid-errors"}
	for _, inc := range view.Incidents {
		if inc.Rule == "dependency-error" {
			t.Errorf("one unit cannot be its own dependency:\n%s", res.stdout)
		}
		if !slices.Contains(want, inc.Rule) {
			t.Errorf("unexpected rule %q", inc.Rule)
		}
		if inc.ID == "" || inc.Reason == "" || inc.Confidence == "" {
			t.Errorf("incident %+v lost its identity or its reason", inc)
		}
		if inc.WindowStart.IsZero() || inc.WindowEnd.IsZero() {
			t.Errorf("incident %s lost its inspected window", inc.ID)
		}
		if !slices.Contains(inc.Primary.CorrelationIDs, inc.ID) {
			t.Errorf("primary %s must carry the incident id it belongs to, got %v", inc.Primary.ID, inc.Primary.CorrelationIDs)
		}
		if len(inc.Related) == 0 {
			t.Errorf("incident %s must show its evidence", inc.ID)
		}
	}
}

type savedIncident struct {
	ID         string `json:"id"`
	Rule       string `json:"rule"`
	Confidence string `json:"confidence"`
	Primary    struct {
		ID string `json:"id"`
	} `json:"primary"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

type savedEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	GeneratedAt   string          `json:"generated_at"`
	Incidents     []savedIncident `json:"incidents"`
}

// --save must write the same document the command prints: the incidents
// envelope, one generated_at on stdout and in the file.
func TestIncidentsSaveWritesTheSameDocument(t *testing.T) {
	crashLoop(t)
	path := filepath.Join(t.TempDir(), "hibiki-saved.json")
	res := runCLI(t, cliDeps{now: testNow}, "incidents", "--since", "24h", "--format", "json", "--save", path)
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var stdout incidentsView
	if err := json.Unmarshal([]byte(res.stdout), &stdout); err != nil {
		t.Fatalf("stdout is not a Hibiki incidents document: %v\n%s", err, res.stdout)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved savedEnvelope
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("saved file is not a Hibiki incidents document: %v\n%s", err, string(data))
	}
	if saved.SchemaVersion != event.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", saved.SchemaVersion, event.SchemaVersion)
	}
	if len(saved.Incidents) != len(stdout.Incidents) {
		t.Fatalf("saved incidents = %d, want %d:\n%s", len(saved.Incidents), len(stdout.Incidents), res.stdout)
	}
	want := []string{"restart-loop", "failed-then-error", "pid-errors"}
	for i, inc := range saved.Incidents {
		wantInc := stdout.Incidents[i]
		if inc.ID != wantInc.ID {
			t.Errorf("saved incident %d id = %q, want %q", i, inc.ID, wantInc.ID)
		}
		if inc.Rule != wantInc.Rule {
			t.Errorf("saved incident %d rule = %q, want %q", i, inc.Rule, wantInc.Rule)
		}
		if inc.Confidence != wantInc.Confidence {
			t.Errorf("saved incident %d confidence = %q, want %q", i, inc.Confidence, wantInc.Confidence)
		}
		if inc.Primary.ID != wantInc.Primary.ID {
			t.Errorf("saved incident %d primary id = %q, want %q", i, inc.Primary.ID, wantInc.Primary.ID)
		}
		if !slices.Contains(want, inc.Rule) {
			t.Errorf("unexpected rule %q", inc.Rule)
		}
		if inc.WindowStart.IsZero() || inc.WindowEnd.IsZero() {
			t.Errorf("saved incident %s lost its inspected window", inc.ID)
		}
	}
	if saved.GeneratedAt != stdoutGeneratedAt(t, res.stdout) {
		t.Errorf("saved generated_at = %q, want the one stdout printed %q", saved.GeneratedAt, stdoutGeneratedAt(t, res.stdout))
	}
}

func stdoutGeneratedAt(t *testing.T, stdout string) string {
	t.Helper()
	var head struct {
		GeneratedAt string `json:"generated_at"`
	}
	if err := json.Unmarshal([]byte(stdout), &head); err != nil {
		t.Fatalf("stdout is not a Hibiki document: %v\n%s", err, stdout)
	}
	return head.GeneratedAt
}

func TestIncidentsTextListsReasons(t *testing.T) {
	crashLoop(t)
	res := runCLI(t, cliDeps{now: testNow}, "incidents", "--since", "24h")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	for _, want := range []string{"Incidents (3)", "restart-loop [high]", "window ", "reason ", "primary ", "related ", "Sources"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("incidents text lost %q:\n%s", want, res.stdout)
		}
	}
	lowered := strings.ToLower(res.stdout)
	for _, banned := range []string{"because", "caused by"} {
		if strings.Contains(lowered, banned) {
			t.Errorf("rendered output must not assert causality (%q):\n%s", banned, res.stdout)
		}
	}
}

func TestIncidentsWindowFlagOverrides(t *testing.T) {
	crashLoop(t)
	res := runCLI(t, cliDeps{now: testNow}, "incidents", "--since", "24h", "--window", "1ms")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "No incidents found.") {
		t.Errorf("every span in the fixture is wider than 1ms:\n%s", res.stdout)
	}
}

func TestIncidentsWindowFlagNegative(t *testing.T) {
	res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test", evs: manyEvents(3)}}, now: testNow},
		"incidents", "--since", "1h", "--window", "-1m")
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr = %s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "--window") {
		t.Errorf("stderr must name the flag: %q", res.stderr)
	}
}

func TestIncidentsEmptyReadable(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(3)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "incidents", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("a readable query without a matching rule is not an error: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "No incidents found.") {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestIncidentsNothingObservedIsNotAnEmptyResult(t *testing.T) {
	src := &recordingSource{name: "test"}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "incidents", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("a readable source with no records is not an error: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "no events matched in the requested window") {
		t.Errorf("no rule ran, so the wording must match the timeline list:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, "No incidents found.") {
		t.Errorf("the query never produced a correlation result:\n%s", res.stdout)
	}
}

func TestIncidentsJSONOnAnEmptyResultIsAnArray(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(3)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "incidents", "--since", "1h", "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var view incidentsView
	if err := json.Unmarshal([]byte(res.stdout), &view); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, res.stdout)
	}
	if view.Incidents == nil {
		t.Error("incidents must be an array, never null")
	}
	if view.Limitations == nil || view.Sources == nil {
		t.Errorf("the envelope head must survive: %+v", view)
	}
}

func TestIncidentsUnreadableSourceIsAnError(t *testing.T) {
	broken := &recordingSource{name: "broken", err: exit.SourceUnavailable("no journal here")}
	res := runCLI(t, cliDeps{sources: []source.Source{broken}, now: testNow}, "incidents", "--since", "1h")
	if res.code != 3 {
		t.Fatalf("exit = %d, want 3 for an unreadable source (stderr = %s)", res.code, res.stderr)
	}
	if strings.Contains(res.stdout, "No incidents found.") {
		t.Errorf("an unreadable journal is not an answer:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "no journal") {
		t.Errorf("stderr = %q", res.stderr)
	}
}
