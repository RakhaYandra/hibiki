package tui

import (
	"sort"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/source"
)

const (
	perfEvents  = 10_000
	perfFrames  = 200
	perfBudget  = 100 * time.Millisecond
	perfColumns = 120
	perfLines   = 40
)

func perfModel(t testing.TB, search string) Model {
	t.Helper()
	m := New(Deps{Sources: []source.Source{newFakeSource()}}, Options{Limit: perfEvents})
	m.width, m.height = perfColumns, perfLines
	m.applyQueryDone(queryDoneMsg{token: m.queryToken, res: readableResult(manyFakeEvents(perfEvents))})
	m.search = search
	if len(m.events) != perfEvents {
		t.Fatalf("events = %d, want the model loaded with %d", len(m.events), perfEvents)
	}
	return m
}

func p95(durations []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)*95/100]
}

func TestNavigationStaysUnderTheBudget(t *testing.T) {
	m := perfModel(t, "")

	down := make([]time.Duration, 0, perfFrames)
	for i := 0; i < perfFrames; i++ {
		start := time.Now()
		updated, _ := m.Update(keyMsg("down"))
		m = updated.(Model)
		_ = m.View()
		down = append(down, time.Since(start))
	}
	if got := p95(down); got >= perfBudget {
		t.Errorf("moving down with %d events: p95 = %s, budget %s", perfEvents, got, perfBudget)
	}

	// A search term with no match is the worst case for `n`: every press scans
	// the whole list looking for a hit that does not exist.
	m = perfModel(t, "no-such-string-anywhere")
	jumps := make([]time.Duration, 0, perfFrames)
	for i := 0; i < perfFrames; i++ {
		start := time.Now()
		updated, _ := m.Update(keyMsg("n"))
		m = updated.(Model)
		_ = m.View()
		jumps = append(jumps, time.Since(start))
	}
	if got := p95(jumps); got >= perfBudget {
		t.Errorf("jumping with %d events and no match: p95 = %s, budget %s", perfEvents, got, perfBudget)
	}
}

func BenchmarkViewWith10kEvents(b *testing.B) {
	m := perfModel(b, "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkUpdateNavigationWith10kEvents(b *testing.B) {
	m := perfModel(b, "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updated, _ := m.Update(keyMsg("down"))
		m = updated.(Model)
		_ = m.View()
	}
}
