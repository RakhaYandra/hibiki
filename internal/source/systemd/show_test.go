package systemd

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func stubRun(t *testing.T, script string, capture *[]string) {
	t.Helper()
	prev := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if capture != nil {
			*capture = append([]string{name}, args...)
		}
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
	t.Cleanup(func() { runCommand = prev })
}

func TestShowParsesKeyValue(t *testing.T) {
	stubRun(t, "printf 'Id=sshd.service\nActiveState=active\nNRestarts=2148\n'", nil)
	got, err := Show(context.Background(), "sshd.service", false)
	if err != nil {
		t.Fatal(err)
	}
	if got["NRestarts"] != "2148" || got["Id"] != "sshd.service" {
		t.Errorf("properties = %v", got)
	}
}

func TestShowUsesArgArrayAndUserScope(t *testing.T) {
	var argv []string
	stubRun(t, "echo Id=x.service", &argv)
	if _, err := Show(context.Background(), "x.service", true); err != nil {
		t.Fatal(err)
	}
	if len(argv) == 0 || argv[0] != "systemctl" {
		t.Fatalf("argv = %v", argv)
	}
	joined := strings.Join(argv, " ")
	for _, want := range []string{"show x.service", "--user", "-p ActiveState", "-p LoadState"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q must contain %q", joined, want)
		}
	}
	for _, a := range argv {
		if strings.ContainsAny(a, " \t\n\"'|;&$<>") {
			t.Errorf("argv element %q looks like a shell string", a)
		}
	}
}

func TestUnitStateMissingUnitIsNotAnError(t *testing.T) {
	stubRun(t, "echo LoadState=not-found; exit 0", nil)
	state, exists, err := UnitState(context.Background(), "typo.service", false)
	if err != nil {
		t.Fatalf("a missing unit is not an error: %v", err)
	}
	if exists || state != "not-found" {
		t.Fatalf("state=%q exists=%v", state, exists)
	}
}

func TestUnitStatePresentUnit(t *testing.T) {
	stubRun(t, "echo LoadState=loaded", nil)
	state, exists, err := UnitState(context.Background(), "sshd.service", true)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || state != "loaded" {
		t.Fatalf("state=%q exists=%v", state, exists)
	}
}

func TestShowMissingSystemctlIsSourceUnavailable(t *testing.T) {
	prev := runCommand
	runCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/nonexistent/systemctl-xyz")
	}
	t.Cleanup(func() { runCommand = prev })
	if _, err := Show(context.Background(), "x.service", false); err == nil {
		t.Fatal("a missing systemctl must surface as an error")
	} else if !strings.Contains(err.Error(), "systemctl") {
		t.Errorf("error must name the tool: %v", err)
	}
}

func TestMonotonicNS2US(t *testing.T) {
	if got := MonotonicNS2US(4862738194); got != 4862738 {
		t.Errorf("got %d, want 4862738", got)
	}
	if got := MonotonicNS2US(0); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}
