package diagnostics

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/exit"
)

func allGoodDeps() Deps {
	return Deps{
		Journalctl: func(context.Context) (string, error) { return "systemd 261\nextra line", nil },
		Systemctl:  func(context.Context) (string, error) { return "systemd 261", nil },
		ReadProbe: func(context.Context) (map[string]string, error) {
			return map[string]string{"_BOOT_ID": "30107f27d01e4e42b24846e4da81db1a", "MESSAGE": "hello"}, nil
		},
		IsTTY: true,
	}
}

func checkNamed(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %s in %+v", name, checks)
	return Check{}
}

func TestDoctorCoversSixChecksInOrder(t *testing.T) {
	checks := Run(context.Background(), allGoodDeps())
	want := []string{CheckJournalctl, CheckSystemd, CheckJournalRead, CheckJournalJSON, CheckBootMetadata, CheckTUIReady}
	if len(checks) != len(want) {
		t.Fatalf("checks = %d, want %d", len(checks), len(want))
	}
	for i, name := range want {
		if checks[i].Name != name {
			t.Errorf("check %d = %s, want %s", i, checks[i].Name, name)
		}
	}
	for _, c := range checks {
		if !c.OK {
			t.Errorf("check %s should pass on a healthy machine: %+v", c.Name, c)
		}
	}
}

func TestDoctorReportsMissingJournalctl(t *testing.T) {
	deps := allGoodDeps()
	deps.Journalctl = func(context.Context) (string, error) { return "", exec.ErrNotFound }
	checks := Run(context.Background(), deps)
	c := checkNamed(t, checks, CheckJournalctl)
	if c.OK {
		t.Fatalf("check = %+v", c)
	}
	if !strings.Contains(c.Fix, "systemd") {
		t.Errorf("fix must say what to install: %q", c.Fix)
	}
	if !SourceUnavailable(checks) {
		t.Error("a missing journalctl must map to the source unavailable exit code")
	}
}

func TestDoctorReportsPermissionDeniedWithBothFixes(t *testing.T) {
	deps := allGoodDeps()
	deps.ReadProbe = func(context.Context) (map[string]string, error) {
		return nil, exit.SourceUnavailable("journalctl cannot read the journal: Failed to open /var/log/journal: Permission denied")
	}
	checks := Run(context.Background(), deps)
	c := checkNamed(t, checks, CheckJournalRead)
	if c.OK {
		t.Fatalf("check = %+v", c)
	}
	for _, want := range []string{"systemd-journal", "ACL"} {
		if !strings.Contains(c.Fix, want) {
			t.Errorf("fix must name both access routes (%q missing): %q", want, c.Fix)
		}
	}
	if !strings.Contains(c.Detail, "Permission denied") {
		t.Errorf("the raw reason must survive: %q", c.Detail)
	}
	if checkNamed(t, checks, CheckJournalJSON).OK {
		t.Error("the JSON output cannot be verified while the journal is unreadable")
	}
	if !SourceUnavailable(checks) {
		t.Error("an unreadable journal must map to the source unavailable exit code")
	}
}

func TestDoctorReportsInvalidJSON(t *testing.T) {
	deps := allGoodDeps()
	deps.ReadProbe = func(context.Context) (map[string]string, error) {
		return nil, exit.BadData("malformed journal record on line 1: invalid character 'b'")
	}
	checks := Run(context.Background(), deps)
	if c := checkNamed(t, checks, CheckJournalJSON); c.OK {
		t.Fatalf("check = %+v", c)
	}
	if !checkNamed(t, checks, CheckJournalRead).OK {
		t.Error("the read itself succeeded, so the read check must pass")
	}
	if SourceUnavailable(checks) {
		t.Error("a decode problem is not a missing source")
	}
}

func TestDoctorReportsMissingBootMetadata(t *testing.T) {
	deps := allGoodDeps()
	deps.ReadProbe = func(context.Context) (map[string]string, error) {
		return map[string]string{"MESSAGE": "hello"}, nil
	}
	checks := Run(context.Background(), deps)
	c := checkNamed(t, checks, CheckBootMetadata)
	if c.OK || c.Fix == "" {
		t.Fatalf("check = %+v", c)
	}
	if SourceUnavailable(checks) {
		t.Error("missing boot metadata is a data finding, not a missing source")
	}
}

func TestDoctorReportsNonTerminalWithoutFailingTheSource(t *testing.T) {
	deps := allGoodDeps()
	deps.IsTTY = false
	checks := Run(context.Background(), deps)
	if c := checkNamed(t, checks, CheckTUIReady); c.OK {
		t.Fatalf("check = %+v", c)
	}
	if SourceUnavailable(checks) {
		t.Error("presentation must not fail the machine check")
	}
}

func TestDoctorNeverSuggestsChangingAnythingByItself(t *testing.T) {
	deps := allGoodDeps()
	deps.ReadProbe = func(context.Context) (map[string]string, error) {
		return nil, errors.New("permission denied")
	}
	for _, c := range Run(context.Background(), deps) {
		if strings.Contains(c.Fix, "sudo systemctl") || strings.Contains(c.Fix, "chmod 777") {
			t.Errorf("a fix must never instruct Hibiki to change the system: %q", c.Fix)
		}
	}
}

func TestTUIReadyNamesADumbTerminal(t *testing.T) {
	deps := allGoodDeps()
	deps.Term = "dumb"
	check := checkNamed(t, Run(context.Background(), deps), CheckTUIReady)
	if check.OK {
		t.Fatalf("check = %+v, want a dumb terminal refused", check)
	}
	if !strings.Contains(check.Detail, "dumb") {
		t.Errorf("detail = %q, want it to name TERM", check.Detail)
	}
	if !strings.Contains(check.Fix, "cursor addressing") || !strings.Contains(check.Fix, "timeline") {
		t.Errorf("fix = %q, want a terminal with cursor addressing or the CLI commands", check.Fix)
	}
}

func TestTUIReadyAcceptsAPlainTerminal(t *testing.T) {
	deps := allGoodDeps()
	deps.Term = "xterm-256color"
	check := checkNamed(t, Run(context.Background(), deps), CheckTUIReady)
	if !check.OK {
		t.Fatalf("check = %+v, want OK on a plain terminal", check)
	}
	if !strings.Contains(check.Detail, "xterm-256color") {
		t.Errorf("detail = %q, want it to name the terminal", check.Detail)
	}
}
