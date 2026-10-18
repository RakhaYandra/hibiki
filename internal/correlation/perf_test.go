package correlation

import (
	"fmt"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

const (
	perfEvents   = 10_000
	perfUnits    = 200
	perfPerUnit  = perfEvents / perfUnits
	perfBootSpan = 40
	perfBootGap  = int64(3_600_000_000)
	perfUnitStep = int64(2_000_000)
	perfSlotStep = int64(10_000_000)
	perfBudget   = 500 * time.Millisecond
)

// Emitted slot by slot rather than unit by unit, so Run receives an input that
// is not already in the order its own comparator wants.
func perfFixture() []event.Event {
	evs := make([]event.Event, 0, perfEvents)
	for slot := 0; slot < perfPerUnit; slot++ {
		for unit := 0; unit < perfUnits; unit++ {
			evs = append(evs, perfSlot(unit, slot))
		}
	}
	return evs
}

// The 2s stagger between units is what puts one unit's failure inside another
// unit's 30s window, so dependency-error fires at all. Slot 4 sits one step
// past the 30s window anchored at slot 0, so it joins dependency-error and
// nothing else. The other 45 slots are ordinary application warnings that no
// rule selects.
func perfSlot(unit, slot int) event.Event {
	boot := unit / perfBootSpan
	base := baseUS + int64(boot)*perfBootGap + int64(unit%perfBootSpan)*perfUnitStep
	ev := testEvent(fmt.Sprintf("ev-%03d-%02d", unit, slot), base+int64(slot)*perfSlotStep,
		event.CategoryApplication, event.SeverityWarning, fmt.Sprintf("unit-%03d.service", unit))
	pid := 1000 + unit
	ev.Process.PID = &pid
	bootID := fmt.Sprintf("%032x", boot)
	ev.Source.BootID = &bootID
	switch slot {
	case 0, 3:
		ev.Category, ev.Severity = event.CategoryService, event.SeverityError
	case 1, 2:
		ev.Category, ev.Severity = event.CategoryService, event.SeverityInfo
		ev.Raw["MESSAGE_ID"] = restartMessageID
	case 4:
		ev.Severity = event.SeverityError
	}
	return ev
}

func TestRunWithinBudget(t *testing.T) {
	events := perfFixture()
	if len(events) != perfEvents {
		t.Fatalf("events = %d, want the fixture loaded with %d", len(events), perfEvents)
	}

	incs := Run(events, Options{})
	if len(incs) == 0 {
		t.Fatalf("Run produced no incidents from the fixture, so the budget would time an empty run")
	}
	fired := map[string]bool{}
	for _, inc := range incs {
		fired[inc.Rule] = true
	}
	for _, r := range rules {
		if !fired[r.id] {
			t.Errorf("rule %q never fired on the fixture, so the budget would time a partial run", r.id)
		}
	}

	start := time.Now()
	Run(events, Options{})
	elapsed := time.Since(start)
	t.Logf("Run over %d events and %d units produced %d incidents in %s, budget %s",
		len(events), perfUnits, len(incs), elapsed, perfBudget)
	if elapsed >= perfBudget {
		t.Errorf("Run over %d events took %s, budget %s", len(events), elapsed, perfBudget)
	}
}

func BenchmarkRun10k(b *testing.B) {
	events := perfFixture()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(Run(events, Options{})) == 0 {
			b.Fatal("Run produced no incidents")
		}
	}
}
