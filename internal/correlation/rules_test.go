package correlation

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

func TestIncidentID(t *testing.T) {
	restart := restartEvents()
	got := incidentID("restart-loop", restart[0], restart[1:])
	if got != "inc_195c260735d6" {
		t.Errorf("incidentID(restart-loop) = %q, want %q", got, "inc_195c260735d6")
	}

	cluster := clusterEvents()
	got = incidentID("failed-then-error", cluster[0], cluster[1:])
	if got != "inc_77f356e7feb7" {
		t.Errorf("incidentID(failed-then-error) = %q, want %q", got, "inc_77f356e7feb7")
	}

	got = incidentID("failed-then-error", cluster[0], cluster[1:2])
	if got == "inc_77f356e7feb7" {
		t.Errorf("a smaller related set must produce a different id")
	}

	reversed := incidentID("restart-loop", restart[0], []event.Event{restart[2], restart[1]})
	if reversed != "inc_195c260735d6" {
		t.Errorf("incidentID with reversed related order = %q, want %q", reversed, "inc_195c260735d6")
	}
}

func TestRun_RestartMessageIDCriterion(t *testing.T) {
	t.Run("info priority restart events match", func(t *testing.T) {
		events := []event.Event{
			testEvent("e1", baseUS, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e2", baseUS+1_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e3", baseUS+2_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
		}
		for i := range events {
			events[i].Raw["MESSAGE_ID"] = restartMessageID
		}
		got := Run(events, Options{})
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

	t.Run("info events without the message id do not match", func(t *testing.T) {
		events := []event.Event{
			testEvent("e1", baseUS, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e2", baseUS+1_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e3", baseUS+2_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})
}

func TestRun_BootGuard(t *testing.T) {
	t.Run("differing boot ids split the group", func(t *testing.T) {
		events := restartEvents()
		events[2] = withBoot(events[2], "boot-b")
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("identical boot ids keep the group", func(t *testing.T) {
		events := restartEvents()
		for i := range events {
			events[i] = withBoot(events[i], "boot-a")
		}
		if got := Run(events, Options{}); len(got) != 1 {
			t.Errorf("Run() returned %d incidents, want 1", len(got))
		}
	})

	t.Run("nil and non-nil boot ids never group together", func(t *testing.T) {
		events := restartEvents()
		events[0] = withBoot(events[0], "boot-a")
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("nil boot id never joins an empty string boot id", func(t *testing.T) {
		events := []event.Event{
			testEvent("e1", baseUS, event.CategoryService, event.SeverityError, "nginx.service"),
			withBoot(testEvent("e2", baseUS+5_000_000, event.CategoryService, event.SeverityError, "nginx.service"), ""),
			withBoot(testEvent("e3", baseUS+45_000_000, event.CategoryService, event.SeverityError, "nginx.service"), ""),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})
}

func incidentIDs(incs []Incident, rule string) []string {
	ids := []string{}
	for _, inc := range incs {
		if inc.Rule == rule {
			ids = append(ids, inc.ID)
		}
	}
	return ids
}

func withPID(ev event.Event, pid int) event.Event {
	ev.Process.PID = &pid
	return ev
}

func withMono(ev event.Event, monoUS int64) event.Event {
	ev.TimestampMonotonicUS = &monoUS
	return ev
}

// The regression fixtures stay in test/ and this package may import only
// internal/event, so crash-loop.jsonl is rebuilt here from the journal fields
// the rules read.
func crashLoopEvents() []event.Event {
	records := []struct {
		cursor    string
		realtime  int64
		monotonic int64
		priority  string
		messageID string
	}{
		{"s=cl;i=1", 1792142920253720, 4936375933000, "6", "5eb03494b6584870a536b337290809b3"},
		{"s=cl;i=2", 1792142920255120, 4936375934400, "6", "39f53479d3a045ac8e11786248231fbf"},
		{"s=cl;i=3", 1792142920277720, 4936375957000, "3", "98e322203f7a4ed290d09fe03c09fe15"},
		{"s=cl;i=4", 1792142920279720, 4936375959000, "3", "d9b373ed55a64feb8242e02dbe79a49c"},
		{"s=cl;i=5", 1792142921280720, 4936376960000, "6", "7d4958e842da4a758f6c1cdc7b36dcc5"},
		{"s=cl;i=6", 1792142921281720, 4936376961000, "6", "9d1aaa27d60140bd96365438aad20286"},
	}
	boot := "30107f27-d01e-4e42-b248-46e4da81db1a"
	pid := 1
	comm := "systemd"
	out := make([]event.Event, 0, len(records))
	for _, rec := range records {
		ev := testEvent(
			event.IDFor(event.Source{Type: "systemd-journal", BootID: &boot, Cursor: &rec.cursor}),
			rec.realtime,
			event.CategoryService,
			event.SeverityFromPriority(rec.priority),
			"omarchy-speaker-loudness.service",
		)
		ev.Source = event.Source{Type: "systemd-journal", Host: "host", BootID: &boot, Cursor: &rec.cursor}
		ev.Process = event.Process{PID: &pid, Comm: &comm}
		ev.Raw["MESSAGE_ID"] = rec.messageID
		ev.Raw["PRIORITY"] = rec.priority
		out = append(out, withMono(ev, rec.monotonic))
	}
	return out
}

// Every group yields two dependency-error incidents over one primary, so the
// incidents tie on timestamp, rule and primary id and only the id separates
// them (ADR-007 determinism contract).
func tiedIncidentEvents() []event.Event {
	events := []event.Event{}
	for g := 0; g < 12; g++ {
		boot := fmt.Sprintf("boot-%02d", g)
		base := baseUS + int64(g)*1_000_000
		tag := fmt.Sprintf("%02d", g)
		pid := 100 + g
		events = append(events,
			withBoot(testEvent("d"+tag+"-a", base, event.CategoryApplication, event.SeverityError, "db.service"), boot),
			withBoot(testEvent("d"+tag+"-b", base+1_000, event.CategoryApplication, event.SeverityError, "cache.service"), boot),
			withBoot(testEvent("f"+tag, base+2_000, event.CategoryService, event.SeverityError, "api.service"), boot),
			withBoot(withPID(testEvent("p"+tag+"-1", base+3_000, event.CategoryApplication, event.SeverityError, "worker.service"), pid), boot),
			withBoot(withPID(testEvent("p"+tag+"-2", base+4_000, event.CategoryApplication, event.SeverityError, "helper.service"), pid), boot),
		)
	}
	return events
}

func TestRun_DeterministicAcrossNewRules(t *testing.T) {
	ordered := tiedIncidentEvents()
	shuffled := []event.Event{}
	for i := len(ordered) - 1; i >= 0; i-- {
		shuffled = append(shuffled, ordered[i])
	}
	wantOrdered := append([]event.Event(nil), ordered...)
	wantShuffled := append([]event.Event(nil), shuffled...)

	gotOrdered := Run(ordered, Options{})
	if n := len(incidentIDs(gotOrdered, "dependency-error")); n != 24 {
		t.Fatalf("fixture produced %d dependency-error incidents, want 24", n)
	}
	if n := len(incidentIDs(gotOrdered, "pid-errors")); n != 12 {
		t.Fatalf("fixture produced %d pid-errors incidents, want 12", n)
	}

	gotShuffled := Run(shuffled, Options{})
	if !reflect.DeepEqual(gotShuffled, gotOrdered) {
		t.Errorf("shuffled input produced a different result than sorted input")
	}
	for i := 0; i < 300; i++ {
		if again := Run(ordered, Options{}); !reflect.DeepEqual(again, gotOrdered) {
			t.Fatalf("run %d produced a different incident order", i)
		}
	}
	if !reflect.DeepEqual(ordered, wantOrdered) {
		t.Errorf("Run mutated its input slice")
	}
	if !reflect.DeepEqual(shuffled, wantShuffled) {
		t.Errorf("Run mutated its input slice")
	}

	for i := 1; i < len(gotOrdered); i++ {
		prev, cur := gotOrdered[i-1], gotOrdered[i]
		if cur.Primary.TimestampUnixUS == prev.Primary.TimestampUnixUS &&
			cur.Rule == prev.Rule &&
			cur.Primary.ID == prev.Primary.ID &&
			cur.ID <= prev.ID {
			t.Errorf("incidents %d and %d tie on timestamp, rule and primary id but are not in id order: %s then %s", i-1, i, prev.ID, cur.ID)
		}
	}
}

func TestRun_PIDErrors(t *testing.T) {
	t.Run("two errors on one pid within thirty seconds", func(t *testing.T) {
		events := []event.Event{
			withPID(testEvent("p1", baseUS, event.CategoryService, event.SeverityError, "api.service"), 4242),
			withPID(testEvent("p2", baseUS+10_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242),
		}
		got := Run(events, Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		inc := got[0]
		if inc.Rule != "pid-errors" {
			t.Errorf("Rule = %q, want %q", inc.Rule, "pid-errors")
		}
		if inc.Confidence != ConfidenceLow {
			t.Errorf("Confidence = %q, want %q", inc.Confidence, ConfidenceLow)
		}
		const wantID = "inc_c76f67e7ae3b"
		if inc.ID != wantID {
			t.Errorf("ID = %q, want %q", inc.ID, wantID)
		}
		const wantReason = "pid 4242: 2 error events within 30s"
		if inc.Reason != wantReason {
			t.Errorf("Reason = %q, want %q", inc.Reason, wantReason)
		}
		if inc.Primary.ID != "p1" {
			t.Errorf("Primary.ID = %q, want %q", inc.Primary.ID, "p1")
		}
		if len(inc.Related) != 1 || inc.Related[0].ID != "p2" {
			t.Errorf("Related = %+v, want single event p2", inc.Related)
		}
		if d := inc.WindowEnd.Sub(inc.WindowStart); d != 30*time.Second {
			t.Errorf("WindowEnd - WindowStart = %v, want 30s", d)
		}
	})

	t.Run("one cluster per pid span and the next span starts fresh", func(t *testing.T) {
		events := []event.Event{
			withPID(testEvent("p1", baseUS, event.CategoryService, event.SeverityError, "api.service"), 4242),
			withPID(testEvent("p2", baseUS+5_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242),
			withPID(testEvent("p3", baseUS+10_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242),
			withPID(testEvent("p4", baseUS+40_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242),
			withPID(testEvent("p5", baseUS+41_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242),
		}
		got := Run(events, Options{})
		if len(got) != 2 {
			t.Fatalf("Run() returned %d incidents, want 2", len(got))
		}
		if got[0].ID != "inc_4135d933c613" || got[0].Primary.ID != "p1" {
			t.Errorf("first incident = %s with primary %s, want inc_4135d933c613 with primary p1", got[0].ID, got[0].Primary.ID)
		}
		if got[0].Rule != "pid-errors" {
			t.Errorf("Rule = %q, want %q", got[0].Rule, "pid-errors")
		}
		if len(got[0].Related) != 2 || got[0].Related[0].ID != "p2" || got[0].Related[1].ID != "p3" {
			t.Errorf("Related ids = %+v, want [p2, p3]", got[0].Related)
		}
		if got[1].ID != "inc_3470d3944270" || got[1].Primary.ID != "p4" {
			t.Errorf("second incident = %s with primary %s, want inc_3470d3944270 with primary p4", got[1].ID, got[1].Primary.ID)
		}
		if len(got[1].Related) != 1 || got[1].Related[0].ID != "p5" {
			t.Errorf("Related ids = %+v, want [p5]", got[1].Related)
		}
		if got[0].Reason != "pid 4242: 3 error events within 30s" {
			t.Errorf("first Reason = %q, want %q", got[0].Reason, "pid 4242: 3 error events within 30s")
		}
		if got[1].Reason != "pid 4242: 2 error events within 30s" {
			t.Errorf("second Reason = %q, want %q", got[1].Reason, "pid 4242: 2 error events within 30s")
		}
	})

	t.Run("same pid on two different boots yields nothing", func(t *testing.T) {
		events := []event.Event{
			withBoot(withPID(testEvent("p1", baseUS, event.CategoryService, event.SeverityError, "api.service"), 4242), "boot-a"),
			withBoot(withPID(testEvent("p2", baseUS+1_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242), "boot-b"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("nil pid yields nothing", func(t *testing.T) {
		events := []event.Event{
			testEvent("p1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
			testEvent("p2", baseUS+1_000_000, event.CategoryApplication, event.SeverityError, "worker.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("info severity yields nothing", func(t *testing.T) {
		events := []event.Event{
			withPID(testEvent("p1", baseUS, event.CategoryService, event.SeverityInfo, "api.service"), 4242),
			withPID(testEvent("p2", baseUS+1_000_000, event.CategoryService, event.SeverityInfo, "api.service"), 4242),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("span beyond thirty seconds yields nothing", func(t *testing.T) {
		events := []event.Event{
			withPID(testEvent("p1", baseUS, event.CategoryService, event.SeverityError, "api.service"), 4242),
			withPID(testEvent("p2", baseUS+30_000_001, event.CategoryApplication, event.SeverityError, "worker.service"), 4242),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("span exactly thirty seconds still matches", func(t *testing.T) {
		events := []event.Event{
			withPID(testEvent("p1", baseUS, event.CategoryService, event.SeverityError, "api.service"), 4242),
			withPID(testEvent("p2", baseUS+30_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242),
		}
		got := Run(events, Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		if got[0].Rule != "pid-errors" {
			t.Errorf("Rule = %q, want %q", got[0].Rule, "pid-errors")
		}
	})

	t.Run("close monotonic values do not substitute for close realtime values", func(t *testing.T) {
		events := []event.Event{
			withMono(withPID(testEvent("p1", baseUS, event.CategoryService, event.SeverityError, "api.service"), 4242), 900_000_000),
			withMono(withPID(testEvent("p2", baseUS+40_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 4242), 900_000_001),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})
}

func TestRun_DependencyError(t *testing.T) {
	t.Run("other unit error ten seconds before the failure", func(t *testing.T) {
		events := []event.Event{
			testEvent("d1", baseUS-10_000_000, event.CategoryApplication, event.SeverityError, "db.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		got := Run(events, Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		inc := got[0]
		if inc.Rule != "dependency-error" {
			t.Errorf("Rule = %q, want %q", inc.Rule, "dependency-error")
		}
		if inc.Confidence != ConfidenceLow {
			t.Errorf("Confidence = %q, want %q", inc.Confidence, ConfidenceLow)
		}
		const wantID = "inc_d2f94ededacf"
		if inc.ID != wantID {
			t.Errorf("ID = %q, want %q", inc.ID, wantID)
		}
		const wantReason = "db.service error before api.service failed, within 30s"
		if inc.Reason != wantReason {
			t.Errorf("Reason = %q, want %q", inc.Reason, wantReason)
		}
		if inc.Primary.ID != "f1" {
			t.Errorf("Primary.ID = %q, want %q", inc.Primary.ID, "f1")
		}
		if len(inc.Related) != 1 || inc.Related[0].ID != "d1" {
			t.Errorf("Related = %+v, want single event d1", inc.Related)
		}
		wantStart := time.UnixMicro(baseUS - 30_000_000).UTC()
		if !inc.WindowStart.Equal(wantStart) {
			t.Errorf("WindowStart = %v, want %v", inc.WindowStart, wantStart)
		}
		wantEnd := time.UnixMicro(baseUS).UTC()
		if !inc.WindowEnd.Equal(wantEnd) {
			t.Errorf("WindowEnd = %v, want %v", inc.WindowEnd, wantEnd)
		}
	})

	t.Run("two other units produce one incident each", func(t *testing.T) {
		events := []event.Event{
			testEvent("d1", baseUS-10_000_000, event.CategoryApplication, event.SeverityError, "db.service"),
			testEvent("d2", baseUS-5_000_000, event.CategoryApplication, event.SeverityError, "cache.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		got := Run(events, Options{})
		if len(got) != 2 {
			t.Fatalf("Run() returned %d incidents, want 2", len(got))
		}
		wantIDs := []string{"inc_0cd20a9379da", "inc_d2f94ededacf"}
		wantReasons := []string{
			"cache.service error before api.service failed, within 30s",
			"db.service error before api.service failed, within 30s",
		}
		wantStart := time.UnixMicro(baseUS - 30_000_000).UTC()
		wantEnd := time.UnixMicro(baseUS).UTC()
		for i, inc := range got {
			if inc.ID != wantIDs[i] {
				t.Errorf("incident %d ID = %q, want %q", i, inc.ID, wantIDs[i])
			}
			if inc.Reason != wantReasons[i] {
				t.Errorf("incident %d Reason = %q, want %q", i, inc.Reason, wantReasons[i])
			}
			if inc.Primary.ID != "f1" {
				t.Errorf("incident %d Primary.ID = %q, want %q", i, inc.Primary.ID, "f1")
			}
			if !inc.WindowStart.Equal(wantStart) {
				t.Errorf("incident %d WindowStart = %v, want %v", i, inc.WindowStart, wantStart)
			}
			if !inc.WindowEnd.Equal(wantEnd) {
				t.Errorf("incident %d WindowEnd = %v, want %v", i, inc.WindowEnd, wantEnd)
			}
		}
	})

	t.Run("same unit error yields nothing", func(t *testing.T) {
		events := []event.Event{
			testEvent("x1", baseUS-10_000_000, event.CategoryApplication, event.SeverityError, "api.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("other unit error after the failure yields nothing", func(t *testing.T) {
		events := []event.Event{
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
			testEvent("d1", baseUS+10_000_000, event.CategoryApplication, event.SeverityError, "db.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("a same timestamp failure of another unit is still a primary", func(t *testing.T) {
		events := []event.Event{
			testEvent("d1", baseUS-10_000_000, event.CategoryApplication, event.SeverityError, "db.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
			testEvent("f2", baseUS, event.CategoryService, event.SeverityError, "web.service"),
		}
		got := Run(events, Options{})
		if len(got) != 2 {
			t.Fatalf("Run() returned %d incidents, want 2: %+v", len(got), got)
		}
		want := []struct {
			id      string
			reason  string
			primary string
		}{
			{"inc_d2f94ededacf", "db.service error before api.service failed, within 30s", "f1"},
			{"inc_7007d70129f3", "db.service error before web.service failed, within 30s", "f2"},
		}
		for i, w := range want {
			if got[i].ID != w.id {
				t.Errorf("incident %d ID = %q, want %q", i, got[i].ID, w.id)
			}
			if got[i].Reason != w.reason {
				t.Errorf("incident %d Reason = %q, want %q", i, got[i].Reason, w.reason)
			}
			if got[i].Primary.ID != w.primary {
				t.Errorf("incident %d Primary.ID = %q, want %q", i, got[i].Primary.ID, w.primary)
			}
			if len(got[i].Related) != 1 || got[i].Related[0].ID != "d1" {
				t.Errorf("incident %d Related = %+v, want single event d1", i, got[i].Related)
			}
		}
	})

	t.Run("only the earliest failure of one unit inside one window is a primary", func(t *testing.T) {
		events := []event.Event{
			testEvent("d1", baseUS-20_000_000, event.CategoryApplication, event.SeverityError, "db.service"),
			testEvent("d2", baseUS-15_000_000, event.CategoryApplication, event.SeverityError, "cache.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
			testEvent("f2", baseUS+10_000_000, event.CategoryService, event.SeverityError, "api.service"),
		}
		got := Run(events, Options{})
		wantIDs := []string{"inc_0cd20a9379da", "inc_d2f94ededacf"}
		gotIDs := incidentIDs(got, "dependency-error")
		if !reflect.DeepEqual(gotIDs, wantIDs) {
			t.Errorf("dependency-error ids in order = %v, want %v", gotIDs, wantIDs)
		}
		for _, inc := range got {
			if inc.Rule != "dependency-error" {
				continue
			}
			if inc.Primary.ID != "f1" {
				t.Errorf("Primary.ID = %q, want %q (f2 must not become a primary)", inc.Primary.ID, "f1")
			}
		}
	})

	t.Run("same timestamp is not before", func(t *testing.T) {
		events := []event.Event{
			testEvent("d1", baseUS, event.CategoryApplication, event.SeverityError, "db.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("nil related unit yields nothing", func(t *testing.T) {
		events := []event.Event{
			testEvent("x1", baseUS-10_000_000, event.CategoryApplication, event.SeverityError, ""),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("different boots yield nothing", func(t *testing.T) {
		events := []event.Event{
			withBoot(testEvent("d1", baseUS-10_000_000, event.CategoryApplication, event.SeverityError, "db.service"), "boot-a"),
			withBoot(testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"), "boot-b"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("more than thirty seconds before yields nothing", func(t *testing.T) {
		events := []event.Event{
			testEvent("d1", baseUS-30_000_001, event.CategoryApplication, event.SeverityError, "db.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("exactly thirty seconds before still matches", func(t *testing.T) {
		events := []event.Event{
			testEvent("d1", baseUS-30_000_000, event.CategoryApplication, event.SeverityError, "db.service"),
			testEvent("f1", baseUS, event.CategoryService, event.SeverityError, "api.service"),
		}
		got := Run(events, Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		if got[0].Rule != "dependency-error" {
			t.Errorf("Rule = %q, want %q", got[0].Rule, "dependency-error")
		}
	})
}

func TestRun_NoCrossBootMatches(t *testing.T) {
	t.Run("same unit and pid in two boots with close realtime yields nothing", func(t *testing.T) {
		events := []event.Event{
			withMono(withBoot(withPID(testEvent("b1", baseUS, event.CategoryService, event.SeverityError, "app.service"), 1), "boot-a"), 900_000_000),
			withMono(withBoot(withPID(testEvent("b2", baseUS+4_000_000, event.CategoryService, event.SeverityError, "app.service"), 1), "boot-b"), 5_000_000),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0: %+v", len(got), got)
		}
	})

	t.Run("two units in two boots yields nothing", func(t *testing.T) {
		events := []event.Event{
			withBoot(testEvent("b1", baseUS, event.CategoryApplication, event.SeverityError, "db.service"), "boot-a"),
			withBoot(testEvent("b2", baseUS+4_000_000, event.CategoryService, event.SeverityError, "api.service"), "boot-b"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0: %+v", len(got), got)
		}
	})

	t.Run("nil boot id never joins a non-nil boot id on one pid", func(t *testing.T) {
		events := []event.Event{
			withPID(testEvent("b1", baseUS, event.CategoryService, event.SeverityError, "app.service"), 1),
			withBoot(withPID(testEvent("b2", baseUS+1_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 1), "boot-a"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0: %+v", len(got), got)
		}
	})

	t.Run("nil boot id never joins an empty string boot id on one pid", func(t *testing.T) {
		events := []event.Event{
			withPID(testEvent("b1", baseUS, event.CategoryService, event.SeverityError, "app.service"), 1),
			withBoot(withPID(testEvent("b2", baseUS+1_000_000, event.CategoryApplication, event.SeverityError, "worker.service"), 1), ""),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0: %+v", len(got), got)
		}
	})

	t.Run("identical boot ids on the same events do match", func(t *testing.T) {
		events := []event.Event{
			withBoot(withPID(testEvent("b1", baseUS, event.CategoryService, event.SeverityError, "app.service"), 1), "boot-a"),
			withBoot(withPID(testEvent("b2", baseUS+4_000_000, event.CategoryService, event.SeverityError, "app.service"), 1), "boot-a"),
		}
		got := Run(events, Options{})
		if len(got) != 2 {
			t.Fatalf("Run() returned %d incidents, want 2", len(got))
		}
		wantRules := []string{"failed-then-error", "pid-errors"}
		for i, inc := range got {
			if inc.Rule != wantRules[i] {
				t.Errorf("incident %d Rule = %q, want %q", i, inc.Rule, wantRules[i])
			}
		}
	})
}

func TestRun_FalsePositivesOnCrashLoop(t *testing.T) {
	got := Run(crashLoopEvents(), Options{})
	if len(got) != 3 {
		t.Fatalf("Run() returned %d incidents, want 3: %+v", len(got), got)
	}

	want := []struct {
		rule       string
		confidence Confidence
		id         string
		reason     string
		primary    string
		related    []string
		window     time.Duration
	}{
		{
			"restart-loop", ConfidenceHigh, "inc_171ce20e78a2",
			"omarchy-speaker-loudness.service: 3 restart or failure events within 5m",
			"evt_7bfa3df5c761", []string{"evt_7bfa3ff5c762", "evt_7bfa40f5c762"}, 5 * time.Minute,
		},
		{
			"failed-then-error", ConfidenceMedium, "inc_775419612656",
			"omarchy-speaker-loudness.service: failed, then 1 error event(s) from the same unit within 30s",
			"evt_7bfa3ff5c762", []string{"evt_7bfa40f5c762"}, 30 * time.Second,
		},
		{
			// Two systemd errors share pid 1. The criteria are weak on purpose
			// and the low confidence plus the shown evidence tell the user to judge.
			"pid-errors", ConfidenceLow, "inc_5a685bcf67f9",
			"pid 1: 2 error events within 30s",
			"evt_7bfa3ff5c762", []string{"evt_7bfa40f5c762"}, 30 * time.Second,
		},
	}

	for i, w := range want {
		inc := got[i]
		if inc.Rule != w.rule {
			t.Errorf("incident %d Rule = %q, want %q", i, inc.Rule, w.rule)
		}
		if inc.Confidence != w.confidence {
			t.Errorf("incident %d Confidence = %q, want %q", i, inc.Confidence, w.confidence)
		}
		if inc.ID != w.id {
			t.Errorf("incident %d ID = %q, want %q", i, inc.ID, w.id)
		}
		if inc.Reason != w.reason {
			t.Errorf("incident %d Reason = %q, want %q", i, inc.Reason, w.reason)
		}
		if inc.Primary.ID != w.primary {
			t.Errorf("incident %d Primary.ID = %q, want %q", i, inc.Primary.ID, w.primary)
		}
		gotRelated := []string{}
		for _, rel := range inc.Related {
			gotRelated = append(gotRelated, rel.ID)
		}
		if !reflect.DeepEqual(gotRelated, w.related) {
			t.Errorf("incident %d Related = %v, want %v", i, gotRelated, w.related)
		}
		if d := inc.WindowEnd.Sub(inc.WindowStart); d != w.window {
			t.Errorf("incident %d window = %v, want %v", i, d, w.window)
		}
	}

	for _, inc := range got {
		if inc.Rule == "dependency-error" {
			t.Errorf("dependency-error fired on a single unit: %q", inc.Reason)
		}
	}
}
