package cli

import (
	"strings"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/exit"
)

func TestUnknownFlagExitsTwo(t *testing.T) {
	res := runCLI(t, cliDeps{}, "timeline", "--bogus")
	if res.code != exit.CodeUsage {
		t.Fatalf("exit = %d, want %d (stderr = %s)", res.code, exit.CodeUsage, res.stderr)
	}
	if !strings.Contains(res.stderr, "unknown flag") {
		t.Errorf("stderr must say what was wrong: %q", res.stderr)
	}
}

func TestStrayArgumentExitsTwo(t *testing.T) {
	res := runCLI(t, cliDeps{}, "timeline", "bogus-arg")
	if res.code != exit.CodeUsage {
		t.Fatalf("exit = %d, want %d (stderr = %s)", res.code, exit.CodeUsage, res.stderr)
	}
	if !strings.Contains(res.stderr, "unknown command") {
		t.Errorf("stderr must say what was wrong: %q", res.stderr)
	}
}

func TestUnknownSubcommandExitsTwo(t *testing.T) {
	res := runCLI(t, cliDeps{}, "bogus")
	if res.code != exit.CodeUsage {
		t.Fatalf("exit = %d, want %d (stderr = %s)", res.code, exit.CodeUsage, res.stderr)
	}
}

func TestInspectMissingArgumentExitsTwo(t *testing.T) {
	res := runCLI(t, cliDeps{}, "inspect")
	if res.code != exit.CodeUsage {
		t.Fatalf("exit = %d, want %d (stderr = %s)", res.code, exit.CodeUsage, res.stderr)
	}
	if !strings.Contains(res.stderr, "accepts 1 arg(s), received 0") {
		t.Errorf("stderr must say what was wrong: %q", res.stderr)
	}
}
