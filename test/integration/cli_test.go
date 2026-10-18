package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

const (
	repoRoot = "../.."
	// Fixtures carry fixed timestamps, so the window is absolute: a relative
	// window would make these tests fail the moment the date changes.
	windowSince = "2026-10-01T00:00:00Z"
	windowUntil = "2026-11-01T00:00:00Z"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hibiki-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binPath = filepath.Join(dir, "hibiki")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/hibiki")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(repoRoot, "test", "fixtures", "regression", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return path
}

// envWith builds a clean environment: every key it sets is dropped from the
// parent first, so an inherited HIBIKI_JOURNAL_FILE never changes a result.
func envWith(pairs map[string]string, drop ...string) []string {
	dropped := map[string]bool{}
	for _, d := range drop {
		dropped[d] = true
	}
	for k := range pairs {
		dropped[k] = true
	}
	out := make([]string, 0, len(os.Environ())+len(pairs))
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 && dropped[kv[:i]] {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range pairs {
		out = append(out, k+"="+v)
	}
	return out
}

func runCLI(t *testing.T, env []string, args ...string) cliResult {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = repoRoot
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := cliResult{stdout: stdout.String(), stderr: stderr.String()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		res.code = exitErr.ExitCode()
	default:
		t.Fatalf("cannot run %s: %v", binPath, err)
	}
	return res
}

type envelope struct {
	SchemaVersion int           `json:"schema_version"`
	Events        []event.Event `json:"events"`
	Truncated     bool          `json:"truncated"`
	NextCursor    *string       `json:"next_cursor"`
	Limitations   []string      `json:"limitations"`
}

func decodeEnvelope(t *testing.T, stdout string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not a Hibiki envelope: %v\n%s", err, stdout)
	}
	return env
}

func TestCLIEndToEndWithFixtureSource(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}),
		"timeline", "--since", windowSince, "--until", windowUntil, "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	env := decodeEnvelope(t, res.stdout)
	if env.SchemaVersion != event.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", env.SchemaVersion, event.SchemaVersion)
	}
	if len(env.Events) != 6 {
		t.Fatalf("events = %d, want the whole fixture", len(env.Events))
	}
	for i, ev := range env.Events {
		if err := ev.Validate(); err != nil {
			t.Fatalf("event %d does not validate: %v", i, err)
		}
		if ev.Source.Cursor == nil || ev.Source.BootID == nil {
			t.Fatalf("event %d lost its provenance: %+v", i, ev.Source)
		}
		if i > 0 && ev.TimestampUnixUS < env.Events[i-1].TimestampUnixUS {
			t.Fatalf("events must be chronological, %d is not", i)
		}
	}
	if !strings.Contains(env.Events[0].Message, "restart counter is at 2148") {
		t.Errorf("the crash loop must be visible as recorded: %q", env.Events[0].Message)
	}
}

func TestCLIReadsTheSameFixtureAsText(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}),
		"events", "--since", windowSince, "--until", windowUntil, "--severity", "error")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	lines := strings.Split(strings.TrimSpace(res.stdout), "\n")
	// Two error records, one line each plus the coverage line.
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3:\n%s", len(lines), res.stdout)
	}
	for _, line := range lines[:2] {
		if !strings.Contains(line, " | ERROR") {
			t.Errorf("line = %q", line)
		}
	}
	if !strings.Contains(lines[2], "covered:") {
		t.Errorf("the coverage line must close the list: %q", lines[2])
	}
}

func TestCLIPreservesMultilineMessages(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "multiline.jsonl")}),
		"events", "--since", windowSince, "--until", windowUntil, "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	env := decodeEnvelope(t, res.stdout)
	if len(env.Events) != 2 {
		t.Fatalf("events = %d", len(env.Events))
	}
	if got := strings.Count(env.Events[0].Message, "\n"); got != 3 {
		t.Errorf("json must keep every line of the message, newlines = %d (%q)", got, env.Events[0].Message)
	}

	text := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "multiline.jsonl")}),
		"events", "--since", windowSince, "--until", windowUntil)
	if got := strings.Count(strings.TrimSpace(text.stdout), "\n"); got != 2 {
		t.Errorf("text mode must print one line per event, lines = %d:\n%s", got+1, text.stdout)
	}
}

func TestCLIKeepsUnknownFieldsOfAOneHundredSixtyOneFieldRecord(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "many-fields.jsonl")}),
		"events", "--since", windowSince, "--until", windowUntil, "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	env := decodeEnvelope(t, res.stdout)
	if len(env.Events) != 1 {
		t.Fatalf("events = %d", len(env.Events))
	}
	if got := len(env.Events[0].Raw); got != 161 {
		t.Errorf("raw fields = %d, want every one of them", got)
	}
	if env.Events[0].Raw["EXTRA_FIELD_000"] != "value 0" {
		t.Errorf("an unknown field must survive verbatim: %q", env.Events[0].Raw["EXTRA_FIELD_000"])
	}
}

func TestCLIWithoutJournalctlFailsClearly(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"PATH": ""}, "HIBIKI_JOURNAL_FILE"),
		"timeline", "--since", "30m")
	if res.code != 3 {
		t.Fatalf("exit = %d, want 3 (stdout = %s, stderr = %s)", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "journalctl") {
		t.Errorf("stderr must name the missing tool: %q", res.stderr)
	}
	if strings.Contains(res.stderr, "panic") || strings.Contains(res.stderr, "goroutine") {
		t.Errorf("a missing tool is not a crash: %q", res.stderr)
	}
}

func TestCLIMalformedFixtureExitsZeroWithLimitation(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "malformed.jsonl")}),
		"events", "--since", windowSince, "--until", windowUntil, "--format", "json")
	if res.code != 0 {
		t.Fatalf("one damaged line must not fail the query: exit = %d, stderr = %s", res.code, res.stderr)
	}
	env := decodeEnvelope(t, res.stdout)
	if len(env.Events) != 1 {
		t.Fatalf("the record before the damage must still arrive: %d events", len(env.Events))
	}
	if !contains(env.Limitations, "malformed_records:1") {
		t.Errorf("the user must be told what was skipped: %v", env.Limitations)
	}
}

func TestCLIAllMalformedFixtureExitsFour(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "all-malformed.jsonl")}),
		"events", "--since", windowSince, "--until", windowUntil)
	if res.code != 4 {
		t.Fatalf("exit = %d, want 4 (stdout = %s, stderr = %s)", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "malformed") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestCLIEmptyWindowIsSuccess(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}),
		"events", "--since", "2020-01-01T00:00:00Z", "--until", "2020-01-02T00:00:00Z")
	if res.code != 0 {
		t.Fatalf("an empty window is not an error: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "no events matched") {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestCLICrossBootEventsAreNotOrderedByMonotonic(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "cross-boot.jsonl")}),
		"timeline", "--since", windowSince, "--until", windowUntil, "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	env := decodeEnvelope(t, res.stdout)
	if len(env.Events) != 2 {
		t.Fatalf("events = %d", len(env.Events))
	}
	first, second := env.Events[0], env.Events[1]
	if !strings.Contains(first.Message, "boot A") || !strings.Contains(second.Message, "boot B") {
		t.Fatalf("the order must follow realtime, not monotonic: %q then %q", first.Message, second.Message)
	}
	if first.TimestampMonotonicUS == nil || second.TimestampMonotonicUS == nil {
		t.Fatalf("the fixture keeps both monotonic stamps: %+v", env.Events)
	}
	if *first.TimestampMonotonicUS <= *second.TimestampMonotonicUS {
		t.Fatalf("monotonic must not be comparable across boots: %d then %d", *first.TimestampMonotonicUS, *second.TimestampMonotonicUS)
	}
	if first.TimestampUnixUS >= second.TimestampUnixUS {
		t.Error("realtime must still ascend")
	}
}

func TestCLIBootFilterAcceptsEitherBootIDForm(t *testing.T) {
	// The fixture holds boot A (all a) and boot B (all b), the second one
	// earlier in monotonic time but later in realtime.
	forms := []string{
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA",
	}
	for _, boot := range forms {
		res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "cross-boot.jsonl")}),
			"timeline", "--since", windowSince, "--until", windowUntil, "--boot", boot, "--format", "json")
		if res.code != 0 {
			t.Fatalf("--boot %s: exit = %d, stderr = %s", boot, res.code, res.stderr)
		}
		env := decodeEnvelope(t, res.stdout)
		if len(env.Events) != 1 {
			t.Fatalf("--boot %s returned %d events, want only boot A", boot, len(env.Events))
		}
		if !strings.Contains(env.Events[0].Message, "boot A") {
			t.Errorf("--boot %s returned %q", boot, env.Events[0].Message)
		}
	}
}

func TestCLIRejectsAValueThatIsNotABootID(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "cross-boot.jsonl")}),
		"timeline", "--since", windowSince, "--until", windowUntil, "--boot", "0")
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2 (stdout = %s, stderr = %s)", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "boot") {
		t.Errorf("stderr must name the flag: %q", res.stderr)
	}
}

func TestCLITuiHelpListsTheFilterFlags(t *testing.T) {
	res := runCLI(t, envWith(nil), "tui", "--help")
	if res.code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr = %s)", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "--since") {
		t.Errorf("the tui must offer the same window flag: %q", res.stdout)
	}
}

func TestCLITuiRefusesAPipedStdout(t *testing.T) {
	res := runCLI(t, envWith(nil), "tui", "--since", windowSince)
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2 (stdout = %s, stderr = %s)", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "requires a terminal") {
		t.Errorf("stderr must say why: %q", res.stderr)
	}
}

func TestCLISIGINTCancelsWithinTwoSeconds(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "journalctl")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binPath, "timeline", "--since", "30m")
	cmd.Dir = repoRoot
	cmd.Env = envWith(map[string]string{"PATH": dir}, "HIBIKI_JOURNAL_FILE")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	// Give the query time to reach the read before interrupting it.
	time.Sleep(300 * time.Millisecond)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("an interrupt must stop the query at once, took %s", elapsed)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("an interrupted query must not report success: err = %v", err)
	}
	if strings.Contains(out.String(), "panic") {
		t.Errorf("output = %q", out.String())
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if strings.Contains(v, want) {
			return true
		}
	}
	return false
}

func TestTuiSIGINTRestoresTheTerminalAndExits130(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script(1) is not on PATH")
	}

	out, in, done := startTuiSession(t)
	waitForOutput(t, out.Name(), "HIBIKI", 15*time.Second)
	if _, err := in.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}

	captured, code := finishTuiSession(t, out, in, done)
	if code != 130 {
		t.Fatalf("exit = %d, want 130 (output: %q)", code, captured)
	}
	if !strings.Contains(captured, "\x1b[?1049l") {
		t.Errorf("output = %q, want the alternate screen released so the shell is usable", captured)
	}
}

func TestTuiQuitKeyExitsZeroUnderAPty(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script(1) is not on PATH")
	}

	out, in, done := startTuiSession(t)
	waitForOutput(t, out.Name(), "HIBIKI", 15*time.Second)
	if _, err := in.Write([]byte{'q'}); err != nil {
		t.Fatal(err)
	}

	captured, code := finishTuiSession(t, out, in, done)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for a plain quit (output: %q)", code, captured)
	}
}

func startTuiSession(t *testing.T) (*os.File, *os.File, *exec.Cmd) {
	t.Helper()
	out, err := os.Create(filepath.Join(t.TempDir(), "tui.out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close() })
	in, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { in.Close() })

	session := fmt.Sprintf("%s tui --since %s --until %s", binPath, windowSince, windowUntil)
	cmd := exec.Command("script", "-qec", session, os.DevNull)
	cmd.Dir = repoRoot
	cmd.Env = envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")})
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return out, writer, cmd
}

func finishTuiSession(t *testing.T, out, in *os.File, cmd *exec.Cmd) (string, int) {
	t.Helper()
	err := cmd.Wait()
	_ = in.Close()
	captured, readErr := os.ReadFile(out.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(captured), exitErr.ExitCode()
	}
	if err != nil {
		t.Fatalf("err = %v, want a reported exit status (output: %q)", err, captured)
	}
	return string(captured), 0
}

func waitForOutput(t *testing.T, path, marker string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), marker) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never showed %q within %s", path, marker, timeout)
}

func TestCLIIncidentsGroupsTheCrashLoopFixture(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}),
		"incidents", "--since", windowSince, "--until", windowUntil, "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var view struct {
		SchemaVersion int `json:"schema_version"`
		Incidents     []struct {
			ID         string `json:"id"`
			Rule       string `json:"rule"`
			Confidence string `json:"confidence"`
			Reason     string `json:"reason"`
			Primary    struct {
				ID string `json:"id"`
			} `json:"primary"`
			Related []struct {
				ID string `json:"id"`
			} `json:"related"`
		} `json:"incidents"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &view); err != nil {
		t.Fatalf("stdout is not a Hibiki incidents document: %v\n%s", err, res.stdout)
	}
	if view.SchemaVersion != event.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", view.SchemaVersion, event.SchemaVersion)
	}
	if len(view.Incidents) != 3 {
		t.Fatalf("incidents = %d, want the three the crash-loop fixture fires:\n%s", len(view.Incidents), res.stdout)
	}
	rules := map[string]string{}
	for _, inc := range view.Incidents {
		rules[inc.Rule] = inc.Confidence
		if inc.ID == "" || inc.Reason == "" || inc.Primary.ID == "" || len(inc.Related) == 0 {
			t.Errorf("incident %+v lost its id, reason or evidence", inc)
		}
	}
	for rule, confidence := range map[string]string{
		"restart-loop":      "high",
		"failed-then-error": "medium",
		"pid-errors":        "low",
	} {
		if got := rules[rule]; got != confidence {
			t.Errorf("rule %s = %q, want confidence %q", rule, got, confidence)
		}
	}
	if _, ok := rules["dependency-error"]; ok {
		t.Errorf("one unit is not its own dependency:\n%s", res.stdout)
	}
}

func TestCLIIncidentsTextNamesEveryRule(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}),
		"incidents", "--since", windowSince, "--until", windowUntil)
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	for _, want := range []string{"Incidents (3)", "restart-loop [high]", "failed-then-error [medium]", "pid-errors [low]", "reason ", "Sources"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("incidents text lost %q:\n%s", want, res.stdout)
		}
	}
}

func TestCLIIncidentsWindowFlagCanSilenceEveryRule(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}),
		"incidents", "--since", windowSince, "--until", windowUntil, "--window", "1ms")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "No incidents found.") {
		t.Errorf("every span in the fixture is wider than 1ms:\n%s", res.stdout)
	}
}

func TestCLIIncidentsWithAnEmptyWindowDoesNotClaimAnEmptySearch(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}),
		"incidents", "--since", "2020-01-01T00:00:00Z", "--until", "2020-01-02T00:00:00Z")
	if res.code != 0 {
		t.Fatalf("an empty window is not an error: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "no events matched in the requested window") {
		t.Errorf("no rule ran over zero events:\n%s", res.stdout)
	}
	if strings.Contains(res.stdout, "No incidents found.") {
		t.Errorf("a query that read nothing never produced a correlation result:\n%s", res.stdout)
	}
}

func TestCLIIncidentsUnreadableSourceFailsClearly(t *testing.T) {
	missing, err := filepath.Abs(filepath.Join(repoRoot, "test", "fixtures", "regression", "missing-journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": missing}),
		"incidents", "--since", windowSince, "--until", windowUntil)
	if res.code != 3 {
		t.Fatalf("exit = %d, want 3 (stdout = %s, stderr = %s)", res.code, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, "No incidents found.") {
		t.Errorf("an unreadable journal is not an answer:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "HIBIKI_JOURNAL_FILE") {
		t.Errorf("stderr must name the unusable source: %q", res.stderr)
	}
}

func TestCLISourcesCheckExitsZeroOnAReadableJournal(t *testing.T) {
	res := runCLI(t, envWith(map[string]string{"HIBIKI_JOURNAL_FILE": fixture(t, "crash-loop.jsonl")}), "sources", "--check")
	if res.code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr = %s)", res.code, res.stderr)
	}
	for _, want := range []string{"journal", "process", "available"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("the report must still list every source; stdout lost %q:\n%s", want, res.stdout)
		}
	}
}
