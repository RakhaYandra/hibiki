package output

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func sourceStatusUnavailable(name, detail string) source.Status {
	return source.Status{Name: name, Detail: detail, Err: exit.SourceUnavailable("%s", detail)}
}

func TestRenderJSONIsValidAndVersioned(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderJSON(&buf, sampleResult()); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["schema_version"] != float64(1) {
		t.Errorf("schema_version = %v", m["schema_version"])
	}
	if _, ok := m["next_cursor"]; !ok {
		t.Error("next_cursor must always be present (null when there is none)")
	}
	if _, ok := m["events"].([]any); !ok {
		t.Error("events must be an array")
	}
	if _, ok := m["sources"].([]any); !ok {
		t.Error("sources must be an array")
	}
	if _, ok := m["covered_since"].(string); !ok {
		t.Errorf("covered_since = %v", m["covered_since"])
	}
	if _, ok := m["generated_at"].(string); !ok {
		t.Errorf("generated_at = %v", m["generated_at"])
	}
}

func TestRenderJSONEmptyResultIsValid(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderJSON(&buf, timeline.Result{}); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"events", "sources", "limitations"} {
		list, ok := m[key].([]any)
		if !ok {
			t.Fatalf("%s must be an array, got %v", key, m[key])
		}
		if len(list) != 0 {
			t.Errorf("%s = %v", key, list)
		}
	}
	if m["next_cursor"] != nil {
		t.Errorf("next_cursor = %v", m["next_cursor"])
	}
	if m["covered_since"] != nil || m["covered_until"] != nil {
		t.Errorf("an empty result proves no coverage: %v %v", m["covered_since"], m["covered_until"])
	}
}

func TestRenderJSONNeverLeaksAnErrorValue(t *testing.T) {
	res := sampleResult()
	res.Sources = append(res.Sources, sourceStatusUnavailable("broken", "permission denied"))
	var buf bytes.Buffer
	if err := RenderJSON(&buf, res); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"Err"`) {
		t.Errorf("a raw error value must never reach JSON: %s", buf.String())
	}
	var m struct {
		Sources []struct {
			Name      string `json:"name"`
			Available bool   `json:"available"`
			Error     string `json:"error"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 2 || m.Sources[1].Available || m.Sources[1].Error != "permission denied" {
		t.Errorf("sources = %+v", m.Sources)
	}
}

func TestRenderJSONIsByteStable(t *testing.T) {
	restore := nowFunc
	nowFunc = fixedNow
	t.Cleanup(func() { nowFunc = restore })

	var first, second bytes.Buffer
	if err := RenderJSON(&first, sampleResult()); err != nil {
		t.Fatal(err)
	}
	if err := RenderJSON(&second, sampleResult()); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Errorf("output is not byte stable:\n%s\n%s", first.String(), second.String())
	}
}

// The streamed document must stay the same document the Envelope type
// describes, field for field, so the two can never drift apart.
func TestRenderJSONStreamsTheSameDocumentAsTheEnvelope(t *testing.T) {
	restore := nowFunc
	nowFunc = fixedNow
	t.Cleanup(func() { nowFunc = restore })

	var streamed bytes.Buffer
	if err := RenderJSON(&streamed, sampleResult()); err != nil {
		t.Fatal(err)
	}
	whole, err := json.MarshalIndent(NewEnvelope(sampleResult()), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(streamed.Bytes(), &a); err != nil {
		t.Fatalf("streamed output is not valid JSON: %v\n%s", err, streamed.String())
	}
	if err := json.Unmarshal(whole, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("streamed envelope differs from the envelope:\n%s\n%s", streamed.String(), whole)
	}
}

// The envelope is a published document, so its bytes are pinned here. Decoding
// both documents into maps cannot see a reordered field, which is the one
// change a machine consumer notices first.
func TestRenderJSONDocumentBytesArePinned(t *testing.T) {
	restore := nowFunc
	nowFunc = fixedNow
	t.Cleanup(func() { nowFunc = restore })

	var buf bytes.Buffer
	if err := RenderJSON(&buf, goldenResult()); err != nil {
		t.Fatal(err)
	}
	const want = `{
  "schema_version": 1,
  "generated_at": "2026-10-16T08:30:00Z",
  "covered_since": "2026-10-16T08:15:30Z",
  "covered_until": "2026-10-16T08:15:30Z",
  "truncated": false,
  "next_cursor": null,
  "sources": [
    {
      "name": "systemd-journal",
      "available": true,
      "detail": "journal is readable"
    }
  ],
  "limitations": [],
  "events": [
    {"schema_version":1,"id":"evt_golden","timestamp":"2026-10-16T08:15:30Z","timestamp_unix_us":1792138530000000,"timestamp_monotonic_us":null,"time_basis":"realtime","source":{"type":"systemd-journal","boot_id":null,"cursor":null},"category":"service","severity":"info","message":"started","service":{"unit":"example.service","active_state":null,"sub_state":null,"result":null},"process":{"pid":null,"comm":null,"executable":null},"correlation_ids":[],"raw":{},"limitations":[]}
  ]
}
`
	if buf.String() != want {
		t.Errorf("the emitted document changed:\ngot:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func goldenResult() timeline.Result {
	ts := time.Date(2026, 10, 16, 8, 15, 30, 0, time.UTC)
	unit := "example.service"
	return timeline.Result{
		Events: []event.Event{{
			SchemaVersion:   event.SchemaVersion,
			ID:              "evt_golden",
			Timestamp:       ts,
			TimestampUnixUS: ts.UnixMicro(),
			TimeBasis:       event.TimeBasisRealtime,
			Source:          event.Source{Type: "systemd-journal"},
			Category:        event.CategoryService,
			Severity:        event.SeverityInfo,
			Message:         "started",
			Service:         event.Service{Unit: &unit},
			CorrelationIDs:  []string{},
			Raw:             map[string]string{},
			Limitations:     []string{},
		}},
		CoveredSince: ts,
		CoveredUntil: ts,
		Sources:      []source.Status{{Name: "systemd-journal", Available: true, Detail: "journal is readable"}},
		Limitations:  []string{},
	}
}

func TestRenderEventJSONIsASingleObject(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderEventJSON(&buf, sampleEvent()); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["id"] != "evt_7d91c2" {
		t.Errorf("id = %v", m["id"])
	}
	if !strings.HasSuffix(strings.TrimSpace(buf.String()), "}") {
		t.Errorf("a single object is expected: %s", buf.String())
	}
}
