package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/tui"
	"github.com/spf13/cobra"
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

func TestTuiCommandPrintsInterruptAndExits130(t *testing.T) {
	root := &cobra.Command{Use: "hibiki", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(&cobra.Command{
		Use:  "tui",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return tui.ErrInterrupted },
	})
	var stderr bytes.Buffer
	root.SetOut(io.Discard)
	root.SetErr(&stderr)
	root.SetArgs([]string{"tui"})

	if code := run(root); code != interruptedCode {
		t.Fatalf("code = %d, want %d", code, interruptedCode)
	}
	if got := strings.TrimSpace(stderr.String()); got != "hibiki: interrupted" {
		t.Fatalf("message = %q, want %q", got, "hibiki: interrupted")
	}
}
