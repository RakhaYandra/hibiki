package integration

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTuiQuitKeyAfterTheListLoadedExitsZero(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script(1) is not on PATH")
	}

	out, in, done := startTuiSession(t)
	waitForOutput(t, out.Name(), "covered:", 20*time.Second)
	if _, err := in.Write([]byte{'q'}); err != nil {
		t.Fatal(err)
	}

	captured, code := finishTuiSession(t, out, in, done)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for a plain quit after loading (output: %q)", code, captured)
	}
	if !strings.Contains(captured, "\x1b[?1049l") {
		t.Errorf("output = %q, want the alternate screen released", captured)
	}
}
