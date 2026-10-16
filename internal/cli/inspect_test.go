package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/session"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func neighbourhood() []event.Event {
	main := testEvent(at(-5*time.Minute), "evt_main", event.CategoryService, event.SeverityWarning)
	main.Raw = map[string]string{"PRIORITY": "4", "_SYSTEMD_USER_UNIT": "example.service"}
	before := testEvent(at(-5*time.Minute-time.Second), "evt_before", event.CategoryService, event.SeverityInfo)
	after := testEvent(at(-5*time.Minute+time.Second), "evt_after", event.CategoryApplication, event.SeverityError)
	return []event.Event{before, main, after}
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
