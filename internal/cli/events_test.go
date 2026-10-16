package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func eventFixture() []event.Event {
	return []event.Event{
		testEvent(at(-30*time.Minute), "info-1", event.CategorySystem, event.SeverityInfo),
		testEvent(at(-20*time.Minute), "warn-1", event.CategoryService, event.SeverityWarning),
		testEvent(at(-10*time.Minute), "err-1", event.CategoryService, event.SeverityError),
	}
}

func TestEventsSeverityCSVParsesMultiple(t *testing.T) {
	src := &recordingSource{name: "test", evs: eventFixture()}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "events", "--since", "1h", "--severity", "warning,error")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if strings.Contains(res.stdout, "info-1") {
		t.Errorf("info must be filtered out:\n%s", res.stdout)
	}
	for _, want := range []string{"warn-1", "err-1"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("%s missing:\n%s", want, res.stdout)
		}
	}
}

func TestEventsRejectsUnknownEnumValues(t *testing.T) {
	for _, args := range [][]string{
		{"events", "--severity", "warning,bogus"},
		{"events", "--category", "banana"},
		{"events", "--type", "banana"},
	} {
		res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test"}}}, args...)
		if res.code != 2 {
			t.Errorf("%v: exit = %d, want 2 (stderr = %s)", args, res.code, res.stderr)
		}
		if !strings.Contains(res.stderr, "bogus") && !strings.Contains(res.stderr, "banana") {
			t.Errorf("%v: the message must name the bad value: %s", args, res.stderr)
		}
	}
}

func TestEventsTypeIsAnAliasForCategory(t *testing.T) {
	src := &recordingSource{name: "test", evs: eventFixture()}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "events", "--since", "1h", "--type", "service")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if strings.Contains(res.stdout, "info-1") {
		t.Errorf("--type service must filter to service events:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, "warn-1") {
		t.Errorf("service events must stay:\n%s", res.stdout)
	}

	agree := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "events", "--since", "1h", "--type", "service", "--category", "service")
	if agree.code != 0 {
		t.Fatalf("matching --type and --category must be accepted: %s", agree.stderr)
	}
	disagree := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "events", "--since", "1h", "--type", "service", "--category", "system")
	if disagree.code != 2 {
		t.Fatalf("exit = %d, want 2 when --type and --category disagree (stderr = %s)", disagree.code, disagree.stderr)
	}
}

func TestEventsLimitDefaultsTo200(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(250)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "events", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if got := strings.Count(res.stdout, "message evt-"); got != 200 {
		t.Fatalf("rendered events = %d, want 200", got)
	}
	if !strings.Contains(res.stdout, "--cursor") {
		t.Errorf("a truncated result must show how to continue:\n%s", res.stdout)
	}
}

func TestEventsPidMustBePositive(t *testing.T) {
	for _, args := range [][]string{
		{"events", "--pid", "0"},
		{"events", "--pid", "-1"},
	} {
		res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test"}}}, args...)
		if res.code != 2 {
			t.Errorf("%v: exit = %d, want 2 (stderr = %s)", args, res.code, res.stderr)
		}
	}
}

func TestEventsUnsetPidIsNotAnError(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(1)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow}, "events", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
}

func TestEmptyResultExitsZero(t *testing.T) {
	res := runCLI(t, cliDeps{sources: []source.Source{&recordingSource{name: "test"}}, now: testNow}, "events", "--since", "1h")
	if res.code != 0 {
		t.Fatalf("an empty timeline is not an error: exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "no events matched") {
		t.Errorf("stdout = %s", res.stdout)
	}
}

func TestUnknownUnitWarnsButExitsZero(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(1)}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow, unitState: unitStateNotFound},
		"events", "--since", "1h", "--service", "typo.service")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "unit not found: typo.service") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestKnownUnitDoesNotWarn(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(1)}
	known := func(context.Context, string, bool) (string, bool, error) { return "loaded", true, nil }
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow, unitState: known},
		"events", "--since", "1h", "--service", "a.service")
	if res.code != 0 || strings.Contains(res.stderr, "unit not found") {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
}

func TestKnownOnlyInUserScopeDoesNotWarn(t *testing.T) {
	src := &recordingSource{name: "test", evs: manyEvents(1)}
	userOnly := func(_ context.Context, _ string, userScope bool) (string, bool, error) {
		if userScope {
			return "loaded", true, nil
		}
		return "not-found", false, nil
	}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow, unitState: userOnly},
		"events", "--since", "1h", "--service", "omarchy-speaker-loudness.service")
	if res.code != 0 || strings.Contains(res.stderr, "unit not found") {
		t.Fatalf("a user unit must not be reported as unknown: exit = %d, stderr = %q", res.code, res.stderr)
	}
}

func TestJSONFormatKeepsStdoutCleanAndWarningsOnStderr(t *testing.T) {
	unresolved := manyEvents(1)
	unresolved[0].Service.Unit = strPtr("typo.service")
	src := &recordingSource{name: "test", evs: unresolved}
	res := runCLI(t, cliDeps{sources: []source.Source{src}, now: testNow, unitState: unitStateNotFound},
		"events", "--since", "1h", "--service", "typo.service", "--format", "json")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}
	var envelope struct {
		NextCursor *string `json:"next_cursor"`
		Events     []struct {
			ID string `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &envelope); err != nil {
		t.Fatalf("stdout must be pure JSON: %v\n%s", err, res.stdout)
	}
	if len(envelope.Events) != 1 {
		t.Fatalf("events = %d", len(envelope.Events))
	}
	if !strings.Contains(res.stderr, "unit not found: typo.service") {
		t.Errorf("the warning must still be visible on stderr: %q", res.stderr)
	}
}

func TestJSONFormatReportsUnavailableSourceInsideTheEnvelope(t *testing.T) {
	broken := &recordingSource{name: "broken", err: exit.SourceUnavailable("permission denied")}
	res := runCLI(t, cliDeps{sources: []source.Source{broken, &recordingSource{name: "ok", evs: manyEvents(1)}}, now: testNow},
		"events", "--since", "1h", "--format", "json")
	if res.code != 0 {
		t.Fatalf("one broken source must not fail the query: exit = %d, stderr = %s", res.code, res.stderr)
	}
	var envelope struct {
		Sources []struct {
			Name      string `json:"name"`
			Available bool   `json:"available"`
			Error     string `json:"error"`
		} `json:"sources"`
		Limitations []string `json:"limitations"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &envelope); err != nil {
		t.Fatalf("stdout must be pure JSON: %v", err)
	}
	if len(envelope.Sources) != 2 || envelope.Sources[0].Available {
		t.Fatalf("sources = %+v", envelope.Sources)
	}
	if envelope.Sources[0].Error != "permission denied" {
		t.Errorf("the reason must survive: %+v", envelope.Sources[0])
	}
	if len(envelope.Limitations) == 0 {
		t.Error("a machine consumer must see that a source was missing")
	}
}
