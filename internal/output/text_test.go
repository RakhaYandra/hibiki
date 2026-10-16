package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func renderText(t *testing.T, res timeline.Result, opts TextOptions) string {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderText(&buf, res, opts); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRenderTextOneLinePerEvent(t *testing.T) {
	out := renderText(t, sampleResult(), TextOptions{Now: fixedNow()})
	for _, want := range []string{"ERROR", "service", "example.service", "Failed to start", "08:15:30.123"} {
		if !strings.Contains(out, want) {
			t.Errorf("the line lost %q: %s", want, out)
		}
	}
	if lines := strings.Count(out, "\n"); lines != 2 {
		t.Errorf("expected one event line plus a coverage line, got:\n%s", out)
	}
}

func TestRenderTextShowsTruncationAndLimitations(t *testing.T) {
	res := sampleResult()
	res.Truncated = true
	res.NextCursor = "CURSORVALUE"
	res.Limitations = []string{"source_unavailable:broken"}
	out := renderText(t, res, TextOptions{Now: fixedNow()})
	if !strings.Contains(out, "--cursor CURSORVALUE") {
		t.Errorf("truncation must tell the user how to continue: %s", out)
	}
	if !strings.Contains(out, "warning: source_unavailable:broken") {
		t.Errorf("every limitation must be printed: %s", out)
	}
}

func TestRenderTextEmptyResultSaysSoWithRange(t *testing.T) {
	out := renderText(t, timeline.Result{}, TextOptions{Now: fixedNow()})
	if !strings.Contains(out, "no events matched") {
		t.Errorf("got %s", out)
	}
	if !strings.Contains(out, "covered:") {
		t.Errorf("the covered range must be stated: %s", out)
	}
	if !strings.Contains(out, "nothing observed") {
		t.Errorf("an empty result proves nothing: %s", out)
	}
}

func TestRenderTextNamesUnavailableSource(t *testing.T) {
	res := sampleResult()
	res.Sources = []source.Status{sourceStatusUnavailable("broken", "permission denied")}
	out := renderText(t, res, TextOptions{Now: fixedNow()})
	if !strings.Contains(out, "broken") || !strings.Contains(out, "permission denied") {
		t.Errorf("an empty timeline must be distinguishable from an unreadable source: %s", out)
	}
}

func TestRenderTextNoColorWhenDisabled(t *testing.T) {
	out := renderText(t, sampleResult(), TextOptions{Now: fixedNow()})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("color must stay off when it is not requested: %q", out)
	}
	colored := renderText(t, sampleResult(), TextOptions{Now: fixedNow(), Color: true})
	if !strings.Contains(colored, "\x1b[") {
		t.Errorf("color was requested: %q", colored)
	}
}

func TestRenderTextUnitMissingShowsDash(t *testing.T) {
	res := sampleResult()
	res.Events[0].Service.Unit = nil
	out := renderText(t, res, TextOptions{Now: fixedNow()})
	if !strings.Contains(out, " | - | ") {
		t.Errorf("a missing unit must be visible, not blank: %s", out)
	}
}

func TestRenderTextMultilineMessageUsesFirstLine(t *testing.T) {
	res := sampleResult()
	res.Events[0].Message = "first line\nsecond line"
	out := renderText(t, res, TextOptions{Now: fixedNow()})
	if strings.Contains(out, "second line") {
		t.Errorf("the list line must stay one line: %s", out)
	}
	if !strings.Contains(out, "first line") {
		t.Errorf("got %s", out)
	}
}

func TestRenderTextTimelessEventDoesNotInventTime(t *testing.T) {
	res := sampleResult()
	res.Events[0].Timestamp = time.Time{}
	res.Events[0].TimestampUnixUS = 0
	res.Events[0].TimeBasis = event.TimeBasisUnknown
	out := renderText(t, res, TextOptions{Now: fixedNow()})
	if strings.Contains(out, "0001-01-01") {
		t.Errorf("a timeless event must not print a fabricated date: %s", out)
	}
	if !strings.Contains(out, "- | ERROR") {
		t.Errorf("got %s", out)
	}
}

func TestRenderTextUsesTheLocationOfNow(t *testing.T) {
	opts := TextOptions{Now: time.Date(2026, 10, 16, 15, 30, 0, 0, time.FixedZone("WIB", 7*3600))}
	out := renderText(t, sampleResult(), opts)
	if !strings.Contains(out, "15:15:30.123") {
		t.Errorf("times must be shown in the viewer's location: %s", out)
	}
}
