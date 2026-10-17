package tui

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func TestQuitCancelsInFlightWorkAndLeavesNoGoroutine(t *testing.T) {
	fake := newFakeSource()
	fake.block = true
	baseline := runtime.NumGoroutine()

	m := New(Deps{Sources: []source.Source{fake}, PageSize: 50}, Options{Limit: 200})
	cmd := m.startQuery(m.filter, m.limit, "")
	if cmd == nil {
		t.Fatal("cmd = nil, want the query to start")
	}

	msgs := make(chan tea.Msg, 1)
	go func() { msgs <- cmd() }()

	_, quitCmd := press(t, m, "q")
	if quitCmd == nil {
		t.Fatal("cmd = nil, want the program to quit")
	}

	select {
	case <-fake.Cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the query was never cancelled: quitting has to stop the work in flight")
	}
	select {
	case <-msgs:
	case <-time.After(2 * time.Second):
		t.Fatal("the query goroutine never returned: something is holding it open")
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > baseline {
		t.Fatalf("goroutines = %d, want back to the baseline of %d", got, baseline)
	}
}

func TestRunReturnsInterruptedWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, Deps{Sources: []source.Source{newFakeSource()}}, Options{}, strings.NewReader(""), io.Discard)
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err = %v, want ErrInterrupted", err)
	}
	if errors.Is(err, diagnostics.ErrNotATerminal) {
		t.Fatal("the terminal guard must not answer for a context a signal already cancelled")
	}
}

func TestCtrlCQuitsAsAnInterruptWhileQDoesNot(t *testing.T) {
	afterCtrlC, _ := press(t, New(Deps{Sources: []source.Source{newFakeSource()}}, Options{Limit: 200}), "ctrl+c")
	if !afterCtrlC.interrupted {
		t.Fatal("ctrl+c must be recorded as an interrupt, so the exit code can be 130")
	}

	afterQuit, _ := press(t, New(Deps{Sources: []source.Source{newFakeSource()}}, Options{Limit: 200}), "q")
	if afterQuit.interrupted {
		t.Fatal("q is a plain quit and must not report an interrupt")
	}
}
