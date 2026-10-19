package output

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/correlation"
	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

func incidentSampleResult() timeline.Result {
	first := sampleEvent()
	second := sampleEvent()
	second.ID = "evt_9ab301c"
	second.Timestamp = first.Timestamp.Add(4 * time.Second)
	second.TimestampUnixUS = first.TimestampUnixUS + 4_000_000
	second.Message = "example.service: Main process exited, code=exited, status=2/INVALIDARGUMENT"
	return timeline.Result{
		Events:       []event.Event{first, second},
		CoveredSince: first.Timestamp,
		CoveredUntil: second.Timestamp,
		Sources:      sampleResult().Sources,
		Limitations:  []string{},
	}
}

func incidentSample(t *testing.T) []correlation.Incident {
	t.Helper()
	incs := correlation.Run(incidentSampleResult().Events, correlation.Options{})
	if len(incs) == 0 {
		t.Fatal("the sample must fire a rule, otherwise this test proves nothing")
	}
	return incs
}

func topLevelKeys(t *testing.T, doc string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(doc))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("document is not JSON: %v\n%s", err, doc)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		t.Fatalf("document is not an object: %s", doc)
	}
	var keys []string
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("document ended early: %v\n%s", err, doc)
		}
		switch v := tok.(type) {
		case json.Delim:
			if v != '}' {
				t.Fatalf("unexpected %v inside the object", v)
			}
			return keys
		case string:
			keys = append(keys, v)
			if err := skipValue(dec); err != nil {
				t.Fatalf("cannot skip %q: %v", v, err)
			}
		}
	}
}

func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || (delim != '{' && delim != '[') {
		return nil
	}
	for depth := 1; depth > 0; {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return nil
}

func TestRenderIncidentsJSONHeadMatchesEnvelope(t *testing.T) {
	restore := nowFunc
	nowFunc = fixedNow
	t.Cleanup(func() { nowFunc = restore })

	res := incidentSampleResult()
	var envelopeDoc, incidentsDoc bytes.Buffer
	env := NewEnvelope(res)
	if err := RenderJSON(&envelopeDoc, res); err != nil {
		t.Fatal(err)
	}
	if err := RenderIncidentsJSON(&incidentsDoc, env, incidentSample(t)); err != nil {
		t.Fatal(err)
	}
	envelopeKeys := topLevelKeys(t, envelopeDoc.String())
	incidentKeys := topLevelKeys(t, incidentsDoc.String())
	if len(envelopeKeys) != len(incidentKeys) {
		t.Fatalf("field counts differ: %v vs %v", envelopeKeys, incidentKeys)
	}
	head := len(envelopeKeys) - 1
	for i := range head {
		if incidentKeys[i] != envelopeKeys[i] {
			t.Fatalf("field %d = %q, want %q (envelope order is the contract)", i, incidentKeys[i], envelopeKeys[i])
		}
	}
	if envelopeKeys[head] != "events" || incidentKeys[head] != "incidents" {
		t.Errorf("the trailing array differs: %v", incidentKeys)
	}
}

func TestRenderIncidentsJSONCarriesStampedEvidence(t *testing.T) {
	incs := incidentSample(t)
	res := incidentSampleResult()
	env := NewEnvelope(res)
	var buf bytes.Buffer
	if err := RenderIncidentsJSON(&buf, env, incs); err != nil {
		t.Fatal(err)
	}
	var view struct {
		Incidents []struct {
			ID          string `json:"id"`
			Rule        string `json:"rule"`
			Confidence  string `json:"confidence"`
			Reason      string `json:"reason"`
			WindowStart string `json:"window_start"`
			WindowEnd   string `json:"window_end"`
			Primary     struct {
				ID             string   `json:"id"`
				CorrelationIDs []string `json:"correlation_ids"`
			} `json:"primary"`
			Related []struct {
				ID             string   `json:"id"`
				CorrelationIDs []string `json:"correlation_ids"`
			} `json:"related"`
		} `json:"incidents"`
	}
	if err := json.Unmarshal(buf.Bytes(), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if len(view.Incidents) != len(incs) {
		t.Fatalf("incidents = %d, want %d", len(view.Incidents), len(incs))
	}
	for i, inc := range incs {
		got := view.Incidents[i]
		if got.ID != inc.ID {
			t.Errorf("incident %d id = %q, want %q", i, got.ID, inc.ID)
		}
		if got.Rule != inc.Rule {
			t.Errorf("incident %d rule = %q, want %q", i, got.Rule, inc.Rule)
		}
		if got.Confidence != string(inc.Confidence) {
			t.Errorf("incident %d confidence = %q, want %q", i, got.Confidence, inc.Confidence)
		}
		if got.Reason != inc.Reason {
			t.Errorf("incident %d reason = %q, want %q", i, got.Reason, inc.Reason)
		}
		if got.Primary.ID != inc.Primary.ID {
			t.Errorf("incident %d primary id = %q, want %q", i, got.Primary.ID, inc.Primary.ID)
		}
		start, err := time.Parse(time.RFC3339Nano, got.WindowStart)
		if err != nil {
			t.Fatalf("incident %d window_start %q is not RFC3339Nano: %v", i, got.WindowStart, err)
		}
		if !start.Equal(inc.WindowStart) {
			t.Errorf("incident %d window_start = %v, want %v", i, start, inc.WindowStart)
		}
		end, err := time.Parse(time.RFC3339Nano, got.WindowEnd)
		if err != nil {
			t.Fatalf("incident %d window_end %q is not RFC3339Nano: %v", i, got.WindowEnd, err)
		}
		if !end.Equal(inc.WindowEnd) {
			t.Errorf("incident %d window_end = %v, want %v", i, end, inc.WindowEnd)
		}
		if !containsID(inc.Primary.CorrelationIDs, inc.ID) {
			t.Errorf("primary %s of %s carries %v", inc.Primary.ID, inc.ID, inc.Primary.CorrelationIDs)
		}
		for _, rel := range inc.Related {
			if !containsID(rel.CorrelationIDs, inc.ID) {
				t.Errorf("related %s of %s carries %v", rel.ID, inc.ID, rel.CorrelationIDs)
			}
		}
	}
}

func TestRenderIncidentsJSONEmptyResultIsValid(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderIncidentsJSON(&buf, NewEnvelope(timeline.Result{}), []correlation.Incident{}); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	list, ok := m["incidents"].([]any)
	if !ok {
		t.Fatalf("incidents must be an array, got %v", m["incidents"])
	}
	if len(list) != 0 {
		t.Errorf("incidents = %v", list)
	}
	for _, key := range []string{"sources", "limitations"} {
		if got, ok := m[key].([]any); !ok || len(got) != 0 {
			t.Errorf("%s = %v", key, m[key])
		}
	}
	if m["next_cursor"] != nil {
		t.Errorf("next_cursor = %v", m["next_cursor"])
	}
}

func TestRenderIncidentsJSONNilRelatedIsAnArray(t *testing.T) {
	hand := []correlation.Incident{{
		ID:          "inc_000000000000",
		Rule:        "restart-loop",
		Confidence:  correlation.ConfidenceHigh,
		Reason:      "example.service: 3 restart or failure events within 5m",
		Primary:     sampleEvent(),
		WindowStart: sampleEvent().Timestamp,
		WindowEnd:   sampleEvent().Timestamp,
	}}
	var buf bytes.Buffer
	if err := RenderIncidentsJSON(&buf, NewEnvelope(sampleResult()), hand); err != nil {
		t.Fatal(err)
	}
	var view struct {
		Incidents []struct {
			Related []struct {
				ID string `json:"id"`
			} `json:"related"`
		} `json:"incidents"`
	}
	if err := json.Unmarshal(buf.Bytes(), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if len(view.Incidents) != 1 {
		t.Fatalf("incidents = %d", len(view.Incidents))
	}
	if view.Incidents[0].Related == nil {
		t.Errorf("a nil related list must render as an array, not null:\n%s", buf.String())
	}
}

func TestRenderIncidentsJSONKeepsTheEngineIncidentShape(t *testing.T) {
	incs := incidentSample(t)
	res := incidentSampleResult()
	engine, err := json.Marshal(incs[0])
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := RenderIncidentsJSON(&buf, NewEnvelope(res), incs); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Incidents []json.RawMessage `json:"incidents"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if len(doc.Incidents) == 0 {
		t.Fatalf("no incident rendered:\n%s", buf.String())
	}
	got := topLevelKeys(t, string(doc.Incidents[0]))
	want := topLevelKeys(t, string(engine))
	if !slices.Equal(got, want) {
		t.Errorf("rendered incident fields %v, engine fields %v: the projection must not add, drop or reorder one", got, want)
	}
}

func TestRenderIncidentsTextNamesRuleConfidenceReasonAndEvidence(t *testing.T) {
	incs := incidentSample(t)
	var buf bytes.Buffer
	if err := RenderIncidentsText(&buf, incidentSampleResult(), incs, TextOptions{Now: fixedNow()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Incidents (2)",
		incs[0].ID + " " + incs[0].Rule + " [" + string(incs[0].Confidence) + "]",
		"window inspected ",
		"reason " + incs[0].Reason,
		"primary ",
		"related ",
		"Sources",
		"covered: ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("incidents text lost %q:\n%s", want, out)
		}
	}
	lowered := strings.ToLower(out)
	for _, banned := range []string{"because", "caused by"} {
		if strings.Contains(lowered, banned) {
			t.Errorf("rendered text must not assert causality (%q):\n%s", banned, out)
		}
	}
}

func TestRenderIncidentsTextWithNoRuleMatchKeepsTheEmptyResultWording(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderIncidentsText(&buf, sampleResult(), nil, TextOptions{Now: fixedNow()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Incidents (0)", "No incidents found.", "Sources", "systemd-journal", "covered: "} {
		if !strings.Contains(out, want) {
			t.Errorf("an empty answer still reports what was read:\n%s", out)
		}
	}
	if strings.Contains(out, "no events matched") {
		t.Errorf("events were read, so this is an empty correlation result, not an empty query:\n%s", out)
	}
}

func TestRenderIncidentsTextWithNothingObservedDoesNotClaimAnEmptySearch(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderIncidentsText(&buf, timeline.Result{}, nil, TextOptions{Now: fixedNow()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Incidents (0)", "no events matched in the requested window", "covered: ", "nothing observed"} {
		if !strings.Contains(out, want) {
			t.Errorf("a query that read nothing must say so (%q):\n%s", want, out)
		}
	}
	if strings.Contains(out, "No incidents found.") {
		t.Errorf("no rule ran over zero events, so no correlation result was produced:\n%s", out)
	}
}

func TestRenderIncidentsTextColorizesBySeverityOnlyWhenAsked(t *testing.T) {
	incs := incidentSample(t)
	var plain, colored bytes.Buffer
	if err := RenderIncidentsText(&plain, incidentSampleResult(), incs, TextOptions{Now: fixedNow()}); err != nil {
		t.Fatal(err)
	}
	if err := RenderIncidentsText(&colored, incidentSampleResult(), incs, TextOptions{Now: fixedNow(), Color: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Errorf("color must stay off for a pipe: %q", plain.String())
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Errorf("color was requested: %q", colored.String())
	}
}

func TestRenderIncidentsTextNamesWhatCouldNotBeReadOrSkipped(t *testing.T) {
	res := incidentSampleResult()
	res.Sources = []source.Status{sourceStatusUnavailable("broken", "permission denied")}
	res.Limitations = []string{"malformed_records:1"}
	res.Truncated = true
	res.NextCursor = "CURSORVALUE"
	var buf bytes.Buffer
	if err := RenderIncidentsText(&buf, res, incidentSample(t), TextOptions{Now: fixedNow()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"broken", "permission denied", "Limitations", "malformed_records:1", "--cursor CURSORVALUE"} {
		if !strings.Contains(out, want) {
			t.Errorf("a partial answer must name what could not be read (%q):\n%s", want, out)
		}
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
