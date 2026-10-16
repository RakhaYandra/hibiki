package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/tui"
)

func TestCommandCodeMapsRefusalToUsage(t *testing.T) {
	if got := commandCode(diagnostics.ErrNotATerminal); got != exit.CodeUsage {
		t.Fatalf("code = %d, want %d", got, exit.CodeUsage)
	}
}

func TestCommandCodeMapsInterruptTo130(t *testing.T) {
	if got := commandCode(tui.ErrInterrupted); got != 130 {
		t.Fatalf("code = %d, want 130", got)
	}
}

func TestCommandCodeMapsAnythingElseToInternal(t *testing.T) {
	if got := commandCode(errors.New("boom")); got != exit.CodeInternal {
		t.Errorf("code = %d, want %d", got, exit.CodeInternal)
	}
	if got := commandCode(nil); got != exit.CodeOK {
		t.Errorf("code = %d, want %d", got, exit.CodeOK)
	}
}

func TestCommandCodeSeesThroughTheWrappedSentinel(t *testing.T) {
	err := fmt.Errorf("%w: TERM=dumb", diagnostics.ErrNotATerminal)
	if got := commandCode(err); got != exit.CodeUsage {
		t.Fatalf("code = %d, want %d: the wrapping must survive the CLI boundary", got, exit.CodeUsage)
	}
}

func TestTuiCommandRefusesAPipedStdout(t *testing.T) {
	res := runCLI(t, cliDeps{sources: []source.Source{}, now: testNow}, "tui")
	if res.code != exit.CodeUsage {
		t.Fatalf("code = %d, want %d (stderr %q)", res.code, exit.CodeUsage, res.stderr)
	}
	if !strings.Contains(res.stderr, "requires a terminal") {
		t.Errorf("stderr = %q, want the reason to say what is missing", res.stderr)
	}
}
