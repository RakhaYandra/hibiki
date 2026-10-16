package diagnostics

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckTerminalRefusesWhenStdoutIsNotATerminal(t *testing.T) {
	err := CheckTerminal("xterm-256color", false)
	if !errors.Is(err, ErrNotATerminal) {
		t.Fatalf("err = %v, want ErrNotATerminal", err)
	}
	if !strings.Contains(err.Error(), "requires a terminal") {
		t.Errorf("the message must say what is wrong: %v", err)
	}
}

func TestCheckTerminalRefusesADumbTerminal(t *testing.T) {
	err := CheckTerminal("dumb", true)
	if !errors.Is(err, ErrNotATerminal) {
		t.Fatalf("err = %v, want ErrNotATerminal", err)
	}
	if !strings.Contains(err.Error(), "TERM=dumb") {
		t.Errorf("the reason must survive: %v", err)
	}
}

func TestCheckTerminalAcceptsAPlainTerminal(t *testing.T) {
	if err := CheckTerminal("xterm-256color", true); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestCheckTerminalAcceptsAnEmptyTermValueOnATTY(t *testing.T) {
	if err := CheckTerminal("  ", true); err != nil {
		t.Fatalf("err = %v, want nil for an unset TERM on a terminal", err)
	}
}
