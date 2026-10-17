package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/spf13/cobra"
)

func healthyDoctor(cmd *cobra.Command) diagnostics.Deps {
	return diagnostics.Deps{
		Journalctl: func(context.Context) (string, error) { return "systemd 261", nil },
		Systemctl:  func(context.Context) (string, error) { return "systemd 261", nil },
		ReadProbe: func(context.Context) (map[string]string, error) {
			return map[string]string{"_BOOT_ID": "30107f27d01e4e42b24846e4da81db1a"}, nil
		},
		IsTTY: false,
	}
}

func unreadableJournal(cmd *cobra.Command) diagnostics.Deps {
	deps := healthyDoctor(cmd)
	deps.ReadProbe = func(context.Context) (map[string]string, error) {
		return nil, exit.SourceUnavailable("journalctl cannot read the journal: Permission denied")
	}
	return deps
}

func TestDoctorTextListsEveryCheckAndFix(t *testing.T) {
	res := runCLI(t, cliDeps{doctorDeps: healthyDoctor}, "doctor")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	for _, want := range []string{"journalctl", "systemd", "journal-read", "journal-json", "boot-metadata", "tui-ready", "ok"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("doctor output lost %q:\n%s", want, res.stdout)
		}
	}
	if !strings.Contains(res.stdout, "needs attention") || !strings.Contains(res.stdout, "fix:") {
		t.Errorf("a failing check must explain itself:\n%s", res.stdout)
	}
}

func TestDoctorJSONHasSixChecks(t *testing.T) {
	res := runCLI(t, cliDeps{doctorDeps: healthyDoctor}, "doctor", "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var payload struct {
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, res.stdout)
	}
	if len(payload.Checks) != 6 {
		t.Fatalf("checks = %d, want 6", len(payload.Checks))
	}
	if payload.Checks[0].Name != "journalctl" || !payload.Checks[0].OK {
		t.Errorf("first check = %+v", payload.Checks[0])
	}
}

func TestDoctorExitCodeIsThreeWhenSourceUnavailable(t *testing.T) {
	res := runCLI(t, cliDeps{doctorDeps: unreadableJournal}, "doctor")
	if res.code != 3 {
		t.Fatalf("exit = %d, want 3 (stdout = %s, stderr = %s)", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "systemd-journal") && !strings.Contains(res.stdout, "systemd-journal group") {
		t.Errorf("the fix must name the group:\n%s", res.stdout)
	}
}

func TestDoctorRejectsBadFormat(t *testing.T) {
	res := runCLI(t, cliDeps{doctorDeps: healthyDoctor}, "doctor", "--format", "yaml")
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2 (stderr = %s)", res.code, res.stderr)
	}
}

func TestDoctorTUIReadyAppearsInTheJSONReport(t *testing.T) {
	res := runCLI(t, cliDeps{doctorDeps: healthyDoctor}, "doctor", "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var payload struct {
		Checks []struct {
			Name string `json:"name"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &payload); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, res.stdout)
	}
	for _, c := range payload.Checks {
		if c.Name == diagnostics.CheckTUIReady {
			return
		}
	}
	t.Fatalf("no %s check in the JSON contract:\n%s", diagnostics.CheckTUIReady, res.stdout)
}
