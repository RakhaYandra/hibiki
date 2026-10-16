package timeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
)

// Two sources sharing n records, interleaved in time, which is the shape a real
// machine has once a second adapter lands (PRD 14).
func interleaved(n int) (*fakeSource, *fakeSource) {
	const base = int64(1_792_000_000_000_000)
	even := make([]event.Event, 0, n)
	odd := make([]event.Event, 0, n)
	for i := 0; i < n; i++ {
		e := ev(base+int64(i)*1000, fmt.Sprintf("evt_%d", i))
		if i%2 == 0 {
			even = append(even, e)
		} else {
			odd = append(odd, e)
		}
	}
	return newFakeSource("even", even...), newFakeSource("odd", odd...)
}

func BenchmarkMergeTwoSources10k(b *testing.B) {
	a, other := interleaved(10000)
	filter := Filter{Since: ev(0, "x").Timestamp, Until: ev(int64(9e15), "x").Timestamp}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := Run(context.Background(), []source.Source{a, other}, filter, 0, "")
		if err != nil {
			b.Fatal(err)
		}
		if len(res.Events) != 10000 {
			b.Fatalf("events = %d, want 10000", len(res.Events))
		}
	}
}
