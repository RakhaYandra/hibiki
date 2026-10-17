package cli

import (
	"strings"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func exitSourceUnavailable() error {
	return exit.SourceUnavailable("permission denied")
}

func TestSourcesShowsCoverageAndProcessLimitation(t *testing.T) {
	src := &recordingSource{name: "systemd-journal", evs: manyEvents(3)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "sources")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	for _, want := range []string{"systemd-journal", "available", "coverage", "read only", "process", "cannot be reconstructed"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("sources output lost %q:\n%s", want, res.stdout)
		}
	}
	if !strings.Contains(res.stdout, manyEvents(1)[0].Timestamp.UTC().Format("2006-01-02T15:04:05")) {
		t.Errorf("the reachable range must be stated:\n%s", res.stdout)
	}
}

func TestSourcesReportsUnavailableSourceWithReason(t *testing.T) {
	broken := &recordingSource{name: "systemd-journal", err: exitSourceUnavailable()}
	res := runCLI(t, cliDeps{sources: []source.Source{broken}, now: testNow}, "sources")
	if res.code != 0 {
		t.Fatalf("listing sources is never an error: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "unavailable") || !strings.Contains(res.stdout, "permission denied") {
		t.Errorf("stdout = %s", res.stdout)
	}
	if strings.Contains(res.stdout, "coverage") {
		t.Errorf("an unreadable source has no coverage to report:\n%s", res.stdout)
	}
}

func TestSourcesCheckExitsThreeWhenTheJournalIsUnreadable(t *testing.T) {
	broken := &recordingSource{name: "systemd-journal", err: exitSourceUnavailable()}
	res := runCLI(t, cliDeps{sources: []source.Source{broken}, now: testNow}, "sources", "--check")
	if res.code != exit.CodeSourceUnavailable {
		t.Fatalf("exit = %d, want %d (stderr = %s)", res.code, exit.CodeSourceUnavailable, res.stderr)
	}
	if !strings.Contains(res.stderr, "permission denied") {
		t.Errorf("stderr must carry the source detail: %q", res.stderr)
	}
}

func TestSourcesCheckPrintsTheListBeforeFailing(t *testing.T) {
	broken := &recordingSource{name: "systemd-journal", err: exitSourceUnavailable()}
	res := runCLI(t, cliDeps{sources: []source.Source{broken}, now: testNow}, "sources", "--check")
	for _, want := range []string{"journal", "process", "unavailable"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("the report is the point and must be printed first; stdout lost %q:\n%s", want, res.stdout)
		}
	}
}

func TestSourcesCheckExitsZeroOnAHealthyMachine(t *testing.T) {
	ok := &recordingSource{name: "systemd-journal", evs: manyEvents(3)}
	res := runCLI(t, cliDeps{sources: []source.Source{ok}, now: testNow}, "sources", "--check")
	if res.code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr = %s)", res.code, res.stderr)
	}
}
