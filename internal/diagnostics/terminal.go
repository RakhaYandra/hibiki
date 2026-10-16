package diagnostics

import (
	"errors"
	"fmt"
	"strings"
)

var ErrNotATerminal = errors.New("requires a terminal")

// CheckTerminal lives here rather than next to the TUI so the doctor check and
// the refusal to start cannot disagree about what a usable terminal is.
func CheckTerminal(termEnv string, isTTY bool) error {
	switch {
	case !isTTY:
		return fmt.Errorf("%w: stdout is not a terminal", ErrNotATerminal)
	case strings.EqualFold(strings.TrimSpace(termEnv), "dumb"):
		return fmt.Errorf("%w: TERM=dumb cannot address the cursor", ErrNotATerminal)
	}
	return nil
}
