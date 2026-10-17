package correlation

import (
	"reflect"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

const baseUS = int64(1_700_000_000_000_000)

func testEvent(id string, tsUS int64, category event.Category, severity event.Severity, unit string) event.Event {
	ev := event.Event{
		ID:              id,
		Timestamp:       time.UnixMicro(tsUS).UTC(),
		TimestampUnixUS: tsUS,
		Category:        category,
		Severity:        severity,
		CorrelationIDs:  []string{},
		Raw:             map[string]string{},
	}
	if unit != "" {
		ev.Service.Unit = &unit
	}
	return ev
}

func withBoot(ev event.Event, bootID string) event.Event {
	ev.Source.BootID = &bootID
	return ev
}

func restartEvents() []event.Event {
	failure := testEvent("e1", baseUS, event.CategoryService, event.SeverityError, "nginx.service")
	firstRestart := testEvent("e2", baseUS+10_000_000, event.CategoryService, event.SeverityInfo, "nginx.service")
	firstRestart.Raw["MESSAGE_ID"] = restartMessageID
	secondRestart := testEvent("e3", baseUS+25_000_000, event.CategoryService, event.SeverityInfo, "nginx.service")
	secondRestart.Raw["MESSAGE_ID"] = restartMessageID
	return []event.Event{failure, firstRestart, secondRestart}
}

func clusterEvents() []event.Event {
	return []event.Event{
		testEvent("e1", baseUS, event.CategoryService, event.SeverityError, "nginx.service"),
		testEvent("e2", baseUS+5_000_000, event.CategoryService, event.SeverityError, "nginx.service"),
		testEvent("e3", baseUS+10_000_000, event.CategoryService, event.SeverityError, "nginx.service"),
	}
}

func deterministicEvents() []event.Event {
	return []event.Event{
		testEvent("e1", baseUS, event.CategoryService, event.SeverityError, "nginx.service"),
		testEvent("e2", baseUS+5_000_000, event.CategoryService, event.SeverityError, "nginx.service"),
		testEvent("e3", baseUS+10_000_000, event.CategoryService, event.SeverityError, "nginx.service"),
		testEvent("L1", baseUS+3_600_000_000_000, event.CategoryService, event.SeverityError, "later.service"),
		testEvent("L2", baseUS+3_600_005_000_000, event.CategoryService, event.SeverityError, "later.service"),
		testEvent("nostamp", 0, event.CategoryService, event.SeverityError, "nginx.service"),
	}
}

func TestRun_RestartLoop(t *testing.T) {
	t.Run("three matching events within five minutes form one incident", func(t *testing.T) {
		got := Run(restartEvents(), Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		inc := got[0]
		if inc.Rule != "restart-loop" {
			t.Errorf("Rule = %q, want %q", inc.Rule, "restart-loop")
		}
		if inc.Confidence != ConfidenceHigh {
			t.Errorf("Confidence = %q, want %q", inc.Confidence, ConfidenceHigh)
		}
		const wantID = "inc_195c260735d6"
		if inc.ID != wantID {
			t.Errorf("ID = %q, want %q", inc.ID, wantID)
		}
		const wantReason = "nginx.service: 3 restart or failure events within 5m"
		if inc.Reason != wantReason {
			t.Errorf("Reason = %q, want %q", inc.Reason, wantReason)
		}
		if inc.Primary.ID != "e1" {
			t.Errorf("Primary.ID = %q, want %q", inc.Primary.ID, "e1")
		}
		if len(inc.Related) != 2 {
			t.Fatalf("len(Related) = %d, want 2", len(inc.Related))
		}
		if inc.Related[0].ID != "e2" || inc.Related[1].ID != "e3" {
			t.Errorf("Related ids = [%s, %s], want [e2, e3]", inc.Related[0].ID, inc.Related[1].ID)
		}
		wantStart := time.UnixMicro(baseUS).UTC()
		if !inc.WindowStart.Equal(wantStart) {
			t.Errorf("WindowStart = %v, want %v", inc.WindowStart, wantStart)
		}
		if d := inc.WindowEnd.Sub(inc.WindowStart); d != 5*time.Minute {
			t.Errorf("WindowEnd - WindowStart = %v, want 5m", d)
		}
		wantIDs := []string{wantID}
		if !reflect.DeepEqual(inc.Primary.CorrelationIDs, wantIDs) {
			t.Errorf("Primary.CorrelationIDs = %v, want %v", inc.Primary.CorrelationIDs, wantIDs)
		}
		for _, rel := range inc.Related {
			if !reflect.DeepEqual(rel.CorrelationIDs, wantIDs) {
				t.Errorf("Related %s CorrelationIDs = %v, want %v", rel.ID, rel.CorrelationIDs, wantIDs)
			}
		}
	})

	t.Run("span exactly equal to the window still matches", func(t *testing.T) {
		events := []event.Event{
			testEvent("e1", baseUS, event.CategoryService, event.SeverityError, "nginx.service"),
			testEvent("e2", baseUS+2_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e3", baseUS+4_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
		}
		events[1].Raw["MESSAGE_ID"] = restartMessageID
		events[2].Raw["MESSAGE_ID"] = restartMessageID
		got := Run(events, Options{Window: 4 * time.Second})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		if got[0].Rule != "restart-loop" {
			t.Errorf("Rule = %q, want %q", got[0].Rule, "restart-loop")
		}
		if len(got[0].Related) != 2 {
			t.Errorf("len(Related) = %d, want 2", len(got[0].Related))
		}
	})

	t.Run("one second override rejects events two seconds apart", func(t *testing.T) {
		events := []event.Event{
			testEvent("e1", baseUS, event.CategoryService, event.SeverityError, "nginx.service"),
			testEvent("e2", baseUS+2_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e3", baseUS+4_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
		}
		events[1].Raw["MESSAGE_ID"] = restartMessageID
		events[2].Raw["MESSAGE_ID"] = restartMessageID
		if got := Run(events, Options{}); len(got) != 1 {
			t.Fatalf("default window returned %d incidents, want 1", len(got))
		}
		got := Run(events, Options{Window: 1 * time.Second})
		if len(got) != 0 {
			t.Errorf("Run() with 1s window returned %d incidents, want 0", len(got))
		}
	})
}

func TestRun_FailedThenError(t *testing.T) {
	t.Run("failure then error five seconds later", func(t *testing.T) {
		events := []event.Event{
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
			testEvent("x1", baseUS+5_000_000, event.CategoryApplication, event.SeverityError, "api.service"),
		}
		got := Run(events, Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		inc := got[0]
		if inc.Rule != "failed-then-error" {
			t.Errorf("Rule = %q, want %q", inc.Rule, "failed-then-error")
		}
		if inc.Confidence != ConfidenceMedium {
			t.Errorf("Confidence = %q, want %q", inc.Confidence, ConfidenceMedium)
		}
		const wantID = "inc_961d07d61a02"
		if inc.ID != wantID {
			t.Errorf("ID = %q, want %q", inc.ID, wantID)
		}
		const wantReason = "api.service: failed, then 1 error event(s) from the same unit within 30s"
		if inc.Reason != wantReason {
			t.Errorf("Reason = %q, want %q", inc.Reason, wantReason)
		}
		if inc.Primary.ID != "f1" {
			t.Errorf("Primary.ID = %q, want %q", inc.Primary.ID, "f1")
		}
		if len(inc.Related) != 1 || inc.Related[0].ID != "x1" {
			t.Errorf("Related = %+v, want single event x1", inc.Related)
		}
		if d := inc.WindowEnd.Sub(inc.WindowStart); d != 30*time.Second {
			t.Errorf("WindowEnd - WindowStart = %v, want 30s", d)
		}
	})

	t.Run("error before failure yields no incident", func(t *testing.T) {
		events := []event.Event{
			testEvent("x0", baseUS-5_000_000, event.CategoryApplication, event.SeverityError, "api.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		got := Run(events, Options{})
		if len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("error sharing the failure timestamp is not related", func(t *testing.T) {
		events := []event.Event{
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
			testEvent("x1", baseUS, event.CategoryApplication, event.SeverityError, "api.service"),
		}
		got := Run(events, Options{})
		if len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("different unit yields no incident", func(t *testing.T) {
		events := []event.Event{
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
			testEvent("x1", baseUS+5_000_000, event.CategoryApplication, event.SeverityError, "worker.service"),
		}
		got := Run(events, Options{})
		if len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})
}

func TestRun_SkipsEventsWithoutTimestamp(t *testing.T) {
	t.Run("zero timestamp event cannot anchor a group", func(t *testing.T) {
		events := []event.Event{
			testEvent("z0", 0, event.CategoryService, event.SeverityError, "nginx.service"),
			testEvent("e1", 5_000_000, event.CategoryService, event.SeverityError, "nginx.service"),
			testEvent("e2", 6_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e3", 7_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
		}
		events[2].Raw["MESSAGE_ID"] = restartMessageID
		events[3].Raw["MESSAGE_ID"] = restartMessageID
		got := Run(events, Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		inc := got[0]
		if inc.Primary.ID != "e1" {
			t.Errorf("Primary.ID = %q, want %q", inc.Primary.ID, "e1")
		}
		if len(inc.Related) != 2 || inc.Related[0].ID != "e2" || inc.Related[1].ID != "e3" {
			t.Errorf("Related ids do not match [e2, e3]: %+v", inc.Related)
		}
		if inc.Primary.TimestampUnixUS == 0 {
			t.Errorf("Primary has no timestamp")
		}
		for _, rel := range inc.Related {
			if rel.TimestampUnixUS == 0 {
				t.Errorf("Related %s has no timestamp", rel.ID)
			}
		}
	})
}

func TestRun_DeterministicAndPure(t *testing.T) {
	ordered := deterministicEvents()
	shuffled := []event.Event{ordered[5], ordered[2], ordered[0], ordered[4], ordered[1], ordered[3]}
	wantOrdered := append([]event.Event(nil), ordered...)
	wantShuffled := append([]event.Event(nil), shuffled...)

	gotShuffled := Run(shuffled, Options{})
	gotOrdered := Run(ordered, Options{})
	gotAgain := Run(ordered, Options{})

	if !reflect.DeepEqual(gotShuffled, gotOrdered) {
		t.Errorf("shuffled input produced a different result than sorted input")
	}
	if !reflect.DeepEqual(gotOrdered, gotAgain) {
		t.Errorf("repeat run produced a different result")
	}
	if !reflect.DeepEqual(ordered, wantOrdered) {
		t.Errorf("Run mutated its input slice")
	}
	if !reflect.DeepEqual(shuffled, wantShuffled) {
		t.Errorf("Run mutated its input slice")
	}
	if len(gotOrdered) != 3 {
		t.Fatalf("Run() returned %d incidents, want 3", len(gotOrdered))
	}
	wantRules := []string{"failed-then-error", "restart-loop", "failed-then-error"}
	wantPrimaries := []string{"e1", "e1", "L1"}
	for i := range wantRules {
		if gotOrdered[i].Rule != wantRules[i] || gotOrdered[i].Primary.ID != wantPrimaries[i] {
			t.Errorf("incident %d = (%s, %s), want (%s, %s)",
				i, gotOrdered[i].Rule, gotOrdered[i].Primary.ID, wantRules[i], wantPrimaries[i])
		}
	}
}

func TestRun_EqualTimestampsBreakByID(t *testing.T) {
	events := []event.Event{
		testEvent("bb", baseUS, event.CategoryService, event.SeverityInfo, "nginx.service"),
		testEvent("cc", baseUS, event.CategoryService, event.SeverityInfo, "nginx.service"),
		testEvent("aa", baseUS, event.CategoryService, event.SeverityInfo, "nginx.service"),
	}
	for i := range events {
		events[i].Raw["MESSAGE_ID"] = restartMessageID
	}
	got := Run(events, Options{})
	if len(got) != 1 {
		t.Fatalf("Run() returned %d incidents, want 1", len(got))
	}
	inc := got[0]
	if inc.Primary.ID != "aa" {
		t.Errorf("Primary.ID = %q, want %q", inc.Primary.ID, "aa")
	}
	if len(inc.Related) != 2 || inc.Related[0].ID != "bb" || inc.Related[1].ID != "cc" {
		t.Errorf("Related ids are not [bb, cc] in order: %+v", inc.Related)
	}
}

func TestRun_EmptyInput(t *testing.T) {
	if got := Run(nil, Options{}); len(got) != 0 {
		t.Errorf("Run(nil) returned %d incidents, want 0", len(got))
	}
	if got := Run([]event.Event{}, Options{Window: time.Second}); len(got) != 0 {
		t.Errorf("Run(empty) returned %d incidents, want 0", len(got))
	}
}

func TestFor(t *testing.T) {
	events := clusterEvents()
	incs := Run(events, Options{})
	if len(incs) != 2 {
		t.Fatalf("Run() returned %d incidents, want 2", len(incs))
	}

	if got := For(events[0], incs); len(got) != 2 {
		t.Errorf("For(primary) returned %d incidents, want 2", len(got))
	}
	if got := For(events[1], incs); len(got) != 2 {
		t.Errorf("For(related) returned %d incidents, want 2", len(got))
	}
	if got := For(testEvent("other", baseUS, event.CategoryService, event.SeverityError, ""), incs); len(got) != 0 {
		t.Errorf("For(unrelated) returned %d incidents, want 0", len(got))
	}

	reversed := []Incident{incs[1], incs[0]}
	got := For(events[0], reversed)
	if len(got) != 2 {
		t.Fatalf("For(reversed) returned %d incidents, want 2", len(got))
	}
	if got[0].Rule != incs[1].Rule || got[1].Rule != incs[0].Rule {
		t.Errorf("For(reversed) rules = [%s, %s], want [%s, %s]",
			got[0].Rule, got[1].Rule, incs[1].Rule, incs[0].Rule)
	}
}

func TestApplyIDs(t *testing.T) {
	dst := deterministicEvents()
	dst = append(dst, testEvent("keep", baseUS+7_200_000_000_000, event.CategoryService, event.SeverityError, "keep.service"))
	dst[0].CorrelationIDs = []string{"stale"}
	dst[6].CorrelationIDs = []string{"untouched"}

	incs := Run(dst, Options{})
	if len(incs) != 3 {
		t.Fatalf("Run() returned %d incidents, want 3", len(incs))
	}
	ApplyIDs(dst, incs)

	wantEvidence := []string{"inc_195c260735d6", "inc_77f356e7feb7"}
	for i := 0; i < 3; i++ {
		if !reflect.DeepEqual(dst[i].CorrelationIDs, wantEvidence) {
			t.Errorf("dst[%d].CorrelationIDs = %v, want %v", i, dst[i].CorrelationIDs, wantEvidence)
		}
	}
	if !reflect.DeepEqual(dst[3].CorrelationIDs, []string{"inc_86505c019b56"}) {
		t.Errorf("dst[3].CorrelationIDs = %v, want [inc_86505c019b56]", dst[3].CorrelationIDs)
	}
	if !reflect.DeepEqual(dst[4].CorrelationIDs, []string{"inc_86505c019b56"}) {
		t.Errorf("dst[4].CorrelationIDs = %v, want [inc_86505c019b56]", dst[4].CorrelationIDs)
	}
	if !reflect.DeepEqual(dst[5].CorrelationIDs, []string{}) {
		t.Errorf("dst[5].CorrelationIDs = %v, want empty", dst[5].CorrelationIDs)
	}
	if !reflect.DeepEqual(dst[6].CorrelationIDs, []string{"untouched"}) {
		t.Errorf("dst[6].CorrelationIDs = %v, want [untouched]", dst[6].CorrelationIDs)
	}
}
