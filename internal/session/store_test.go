package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/output"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func storedEvent() event.Event {
	unit := "example.service"
	ts := time.Date(2026, 10, 16, 9, 28, 29, 3719, time.UTC)
	return event.Event{
		SchemaVersion:   event.SchemaVersion,
		ID:              "evt_x",
		Timestamp:       ts,
		TimestampUnixUS: ts.UnixMicro(),
		TimeBasis:       event.TimeBasisRealtime,
		Source:          event.Source{Type: "systemd-journal"},
		Category:        event.CategoryService,
		Severity:        event.SeverityWarning,
		Message:         "Main process exited",
		Service:         event.Service{Unit: &unit},
		CorrelationIDs:  []string{},
		Raw:             map[string]string{"PRIORITY": "4", "_PID": "2401"},
		Limitations:     []string{},
	}
}

func sampleResult() timeline.Result {
	e := storedEvent()
	return timeline.Result{
		Events:       []event.Event{e},
		CoveredSince: e.Timestamp,
		CoveredUntil: e.Timestamp,
		Limitations:  []string{},
	}
}

func TestSaveWritesRestrictedFileAndLoadFindsEvent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hibiki")
	path, err := Save(dir, sampleResult())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the snapshot directory must stay private: %v %v", info.Mode().Perm(), err)
	}
	got, res, err := Load(dir, "evt_x")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "evt_x" || len(res.Events) == 0 {
		t.Fatalf("load returned %+v", got)
	}
	if got.Raw["PRIORITY"] != "4" || got.Service.Unit == nil || *got.Service.Unit != "example.service" {
		t.Fatalf("the event lost fields on the way through disk: %+v", got)
	}
	if res.CoveredSince.IsZero() {
		t.Error("coverage must survive the snapshot")
	}
}

func TestLoadUnknownIDExplainsItIsNotInTheLastQuery(t *testing.T) {
	dir := t.TempDir()
	if _, err := Save(dir, sampleResult()); err != nil {
		t.Fatal(err)
	}
	_, _, err := Load(dir, "evt_missing")
	if err == nil {
		t.Fatal("an unknown id must be an error")
	}
	if !strings.Contains(err.Error(), "not in the last query") {
		t.Errorf("the message must say what happened: %v", err)
	}
}

func TestLoadWithoutASnapshotExplainsWhatToDo(t *testing.T) {
	_, _, err := Load(t.TempDir(), "evt_x")
	if err == nil {
		t.Fatal("a missing snapshot must be an error")
	}
	if !strings.Contains(err.Error(), "no previous query") {
		t.Errorf("the message must say what to do: %v", err)
	}
}

func TestSaveIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Save(dir, sampleResult()); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(dir, sampleResult()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != fileName {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory holds %v, want only %s", names, fileName)
	}
}

func TestSaveReplacesAnUnreadableSnapshot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(dir, sampleResult()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(dir, "evt_x"); err != nil {
		t.Fatalf("a fresh save must repair a damaged snapshot: %v", err)
	}
}

func TestLoadReportsDamageInsteadOfPanicking(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(dir, "evt_x"); err == nil {
		t.Fatal("a damaged snapshot must be reported")
	}
}

func TestDefaultDirHonoursXDGCacheHome(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/opencode/cache")
	if got := DefaultDir(); got != filepath.Join("/tmp/opencode/cache", "hibiki") {
		t.Errorf("DefaultDir = %q", got)
	}
	t.Setenv("XDG_CACHE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory to fall back to")
	}
	if got := DefaultDir(); got != filepath.Join(home, ".cache", "hibiki") {
		t.Errorf("DefaultDir = %q", got)
	}
}

func TestFindInLoadsFromAnEnvelope(t *testing.T) {
	env := output.NewEnvelope(sampleResult())
	got, res, err := FindIn(env, "evt_x")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "evt_x" || len(res.Events) != 1 {
		t.Fatalf("got %+v", got)
	}
	if _, _, err := FindIn(env, "evt_other"); err == nil {
		t.Fatal("an unknown id must be reported")
	}
}
