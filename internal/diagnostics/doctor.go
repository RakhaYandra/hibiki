package diagnostics

import (
	"context"
	"errors"
	"strings"

	"github.com/RakhaYandra/hibiki/internal/exit"
)

const (
	CheckJournalctl   = "journalctl"
	CheckSystemd      = "systemd"
	CheckJournalRead  = "journal-read"
	CheckJournalJSON  = "journal-json"
	CheckBootMetadata = "boot-metadata"
	CheckTUIReady     = "tui-ready"
)

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

type Deps struct {
	Journalctl func(ctx context.Context) (string, error)
	Systemctl  func(ctx context.Context) (string, error)
	ReadProbe  func(ctx context.Context) (map[string]string, error)
	IsTTY      bool
}

// Six fixed checks, in a fixed order, so a report is comparable between runs
// and machines (PRD 9.6). A failing check explains itself and says what the
// user can do; Hibiki never changes the system itself.
func Run(ctx context.Context, deps Deps) []Check {
	checks := make([]Check, 0, 6)

	checks = append(checks, toolCheck(ctx, CheckJournalctl, deps.Journalctl,
		"install systemd (journalctl ships with it) and make sure journalctl is on PATH"))
	checks = append(checks, toolCheck(ctx, CheckSystemd, deps.Systemctl,
		"install systemd (systemctl ships with it) and make sure systemctl is on PATH"))

	record, probeErr := deps.ReadProbe(ctx)
	read, decode := probeOutcome(probeErr)
	checks = append(checks, read, decode)

	switch {
	case probeErr != nil && read.OK:
		checks = append(checks, Check{Name: CheckBootMetadata, Detail: "cannot verify boot metadata until the journal is readable"})
	case probeErr != nil:
		checks = append(checks, Check{Name: CheckBootMetadata, Detail: "cannot verify boot metadata until the probe succeeds"})
	case record["_BOOT_ID"] != "":
		checks = append(checks, Check{Name: CheckBootMetadata, OK: true, Detail: "_BOOT_ID " + record["_BOOT_ID"]})
	default:
		checks = append(checks, Check{
			Name:   CheckBootMetadata,
			Detail: "the newest record carries no _BOOT_ID",
			Fix:    "boot scoped queries need _BOOT_ID; use --since instead of --boot on this journal",
		})
	}

	if deps.IsTTY {
		checks = append(checks, Check{Name: CheckTUIReady, OK: true, Detail: "stdout is a terminal"})
	} else {
		checks = append(checks, Check{
			Name:   CheckTUIReady,
			Detail: "stdout is not a terminal, so colored output stays off",
			Fix:    "run Hibiki in a terminal for color; this check only affects presentation",
		})
	}
	return checks
}

func toolCheck(ctx context.Context, name string, probe func(context.Context) (string, error), fix string) Check {
	if probe == nil {
		return Check{Name: name, Detail: "not probed", Fix: fix}
	}
	version, err := probe(ctx)
	if err != nil {
		return Check{Name: name, Detail: err.Error(), Fix: fix}
	}
	return Check{Name: name, OK: true, Detail: firstLine(version)}
}

// The read check and the decode check share one probe, so a decode failure must
// not be reported as an unreadable journal.
func probeOutcome(probeErr error) (read, decode Check) {
	read = Check{Name: CheckJournalRead, OK: true, Detail: "read one journal record"}
	decode = Check{Name: CheckJournalJSON, OK: true, Detail: "the record decodes as JSON"}
	switch {
	case probeErr == nil:
	case isBadData(probeErr):
		decode = Check{
			Name:   CheckJournalJSON,
			Detail: probeErr.Error(),
			Fix:    "the journal output is not valid JSON Lines; report it together with `journalctl --output=json --no-pager -n 1`",
		}
	default:
		read = Check{Name: CheckJournalRead, Detail: probeErr.Error(), Fix: journalAccessFix(probeErr)}
		decode = Check{Name: CheckJournalJSON, Detail: "cannot verify the JSON output while the journal is unreadable"}
	}
	return read, decode
}

func journalAccessFix(err error) string {
	if strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		return "read access has two routes: add your user to the systemd-journal group (or adm), or grant a read ACL such as " +
			"`setfacl -m g:wheel:r-x /var/log/journal`. Hibiki never changes permissions itself."
	}
	return "check that journalctl can read the journal as this user, and that /var/log/journal exists"
}

// Any source level failure makes the whole tool useless on this machine, so it
// maps to the source unavailable exit code (PRD 9.5).
func SourceUnavailable(checks []Check) bool {
	for _, c := range checks {
		if !c.OK && (c.Name == CheckJournalctl || c.Name == CheckJournalRead) {
			return true
		}
	}
	return false
}

func isBadData(err error) bool {
	var e *exit.Error
	return errors.As(err, &e) && e.Code == exit.CodeBadData
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
