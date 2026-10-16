package systemd

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/RakhaYandra/hibiki/internal/exit"
)

const (
	commandTimeout = 5 * time.Second
	waitDelay      = 2 * time.Second
)

var unitProperties = []string{
	"Id", "ActiveState", "SubState", "Result",
	"ExecMainPID", "ExecMainCode", "ExecMainStatus", "NRestarts", "LoadState",
}

var runCommand = exec.CommandContext

func Show(ctx context.Context, unit string, userScope bool) (map[string]string, error) {
	args := []string{"show", unit}
	for _, p := range unitProperties {
		args = append(args, "-p", p)
	}
	return runShow(ctx, args, userScope)
}

// A missing unit is reported by systemctl as LoadState=not-found with exit
// status 0, so existence comes from the property and never from the exit code
// (spike 3).
func UnitState(ctx context.Context, unit string, userScope bool) (string, bool, error) {
	props, err := runShow(ctx, []string{"show", unit, "-p", "LoadState"}, userScope)
	if err != nil {
		return "", false, err
	}
	state := props["LoadState"]
	if state == "" {
		return "", false, exit.SourceUnavailable("systemctl show %s returned no LoadState", unit)
	}
	return state, state != "not-found", nil
}

// systemd reports *TimestampMonotonic properties in nanoseconds while the
// journal reports __MONOTONIC_TIMESTAMP in microseconds (spike 4).
func MonotonicNS2US(ns int64) int64 { return ns / 1000 }

func runShow(ctx context.Context, args []string, userScope bool) (map[string]string, error) {
	if userScope {
		args = append(args, "--user")
	}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := runCommand(ctx, "systemctl", args...)
	cmd.WaitDelay = waitDelay
	out, err := cmd.Output()
	if err != nil {
		return nil, classify("systemctl", err)
	}
	return parseProperties(string(out)), nil
}

func parseProperties(out string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		props[k] = v
	}
	return props
}

func classify(tool string, err error) error {
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
			return exit.SourceUnavailable("%s failed: %s", tool, msg)
		}
		return exit.SourceUnavailable("%s exited with status %d", tool, exitErr.ExitCode())
	case errors.Is(err, exec.ErrNotFound):
		return exit.SourceUnavailable("%s is not installed or not on PATH", tool)
	default:
		return exit.SourceUnavailable("%s could not be run: %v", tool, err)
	}
}
