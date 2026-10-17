package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/RakhaYandra/hibiki/internal/diagnostics"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/source/journal"
	"github.com/spf13/cobra"
)

var newDoctorDeps = func(cmd *cobra.Command) diagnostics.Deps {
	return diagnostics.Deps{
		Journalctl: func(ctx context.Context) (string, error) { return toolVersion(ctx, "journalctl") },
		Systemctl:  func(ctx context.Context) (string, error) { return toolVersion(ctx, "systemctl") },
		ReadProbe:  readOneRecord,
		IsTTY:      isTTY(cmd.OutOrStdout()),
		Term:       os.Getenv("TERM"),
	}
}

func newDoctorCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check what Hibiki can read on this machine",
		Long: "Run six read only checks and report what works, what does not, and what\n" +
			"the user can do about it. Hibiki never changes the machine itself.",
		Args: noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if format != "text" && format != "json" {
				return exit.Usage("--format must be text or json, got %q", format)
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			checks := diagnostics.Run(ctx, newDoctorDeps(c))
			if err := renderChecks(c.OutOrStdout(), checks, format); err != nil {
				return err
			}
			if diagnostics.SourceUnavailable(checks) {
				return exit.SourceUnavailable("Hibiki cannot read the journal on this machine; see the fixes above")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	return cmd
}

func renderChecks(w io.Writer, checks []diagnostics.Check, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Checks []diagnostics.Check `json:"checks"`
		}{Checks: checks})
	}
	out := bufio.NewWriter(w)
	for _, c := range checks {
		state := "ok"
		if !c.OK {
			state = "needs attention"
		}
		fmt.Fprintf(out, "%-14s %-16s %s\n", c.Name, state, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(out, "%-14s %-16s %s\n", "", "", "fix: "+c.Fix)
		}
	}
	return out.Flush()
}

func toolVersion(ctx context.Context, tool string) (string, error) {
	cmd := exec.CommandContext(ctx, tool, "--version")
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", exit.SourceUnavailable("%s is not installed or not on PATH", tool)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", exit.SourceUnavailable("%s --version exited with status %d: %s", tool, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", exit.SourceUnavailable("%s could not be run: %v", tool, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func readOneRecord(ctx context.Context) (map[string]string, error) {
	it, err := journal.New().Query(ctx, source.Query{Limit: 1})
	if err != nil {
		return nil, err
	}
	defer it.Close()
	ev, err := it.Next(ctx)
	if errors.Is(err, io.EOF) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return ev.Raw, nil
}
