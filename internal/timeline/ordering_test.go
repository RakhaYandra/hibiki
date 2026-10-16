package timeline

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func TestRunMergesSourcesChronologically(t *testing.T) {
	a := newFakeSource("a", ev(100, "a1"), ev(300, "a3"))
	b := newFakeSource("b", ev(200, "b2"), ev(400, "b4"))
	res, err := Run(context.Background(), []source.Source{a, b}, Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(res.Events), []string{"a1", "b2", "a3", "b4"}; !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	assertClosed(t, a, b)
}

func TestOrderTieBreakUsesMonotonicOnlyWithinSameBoot(t *testing.T) {
	m1, m2, m3 := int64(100), int64(200), int64(50)
	bootA, bootB := "boot-a", "boot-b"
	x := ev(1000, "x")
	x.TimestampMonotonicUS, x.Source.BootID = &m1, &bootA
	y := ev(1000, "y")
	y.TimestampMonotonicUS, y.Source.BootID = &m2, &bootA
	z := ev(1000, "z")
	z.TimestampMonotonicUS, z.Source.BootID = &m3, &bootB
	if !orderLess(x, y) {
		t.Error("a smaller monotonic value must come first inside one boot")
	}
	if orderLess(z, x) {
		t.Error("monotonic values must never be compared across boots")
	}
	w := ev(1000, "w")
	x2 := ev(1000, "x2")
	if !orderLess(w, x2) {
		t.Error("equal timestamps without monotonic data must still order by id")
	}
}

func TestOrderIsTotalAndDeterministic(t *testing.T) {
	boots := []string{"boot-a", "boot-b"}
	evs := make([]event.Event, 0, 500)
	for i := 0; i < 500; i++ {
		us := int64(1000 + i)
		if i < 40 {
			us = 1000
		}
		e := ev(us, fmt.Sprintf("id-%04d", i))
		boot := boots[i%2]
		e.Source.BootID = &boot
		mono := int64(i)
		e.TimestampMonotonicUS = &mono
		evs = append(evs, e)
	}

	sorted := make([]event.Event, len(evs))
	perm := rand.New(rand.NewSource(7)).Perm(len(evs))
	for i, p := range perm {
		sorted[i] = evs[p]
	}
	slices.SortFunc(sorted, func(a, b event.Event) int {
		if orderLess(a, b) {
			return -1
		}
		if orderLess(b, a) {
			return 1
		}
		return 0
	})
	again := slices.Clone(evs)
	slices.SortFunc(again, func(a, b event.Event) int {
		if orderLess(a, b) {
			return -1
		}
		if orderLess(b, a) {
			return 1
		}
		return 0
	})
	if !slices.Equal(ids(sorted), ids(again)) {
		t.Fatalf("ordering is not deterministic:\n%v\n%v", ids(sorted), ids(again))
	}
	for i := 1; i < len(sorted); i++ {
		if !orderLess(sorted[i-1], sorted[i]) && sorted[i-1].ID != sorted[i].ID {
			t.Fatalf("not sorted at %d: %v then %v", i, sorted[i-1].ID, sorted[i].ID)
		}
	}
}

func TestRunRecordsUnavailableSourceWithoutFailing(t *testing.T) {
	bad := newFailingSource("broken", exit.SourceUnavailable("permission denied"))
	res, err := Run(context.Background(), []source.Source{bad, newFakeSource("a", ev(1, "a1"))}, Filter{}, 0, "")
	if err != nil {
		t.Fatalf("a broken source must not fail the query: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events = %d", len(res.Events))
	}
	if len(res.Sources) != 2 || res.Sources[0].Available {
		t.Fatalf("sources = %+v", res.Sources)
	}
	if len(res.Limitations) == 0 {
		t.Error("an unavailable source must be recorded as a limitation")
	}
}

func TestRunWithNoSourcesReturnsEmptySlice(t *testing.T) {
	res, err := Run(context.Background(), nil, Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Events == nil || res.Sources == nil || res.Limitations == nil {
		t.Fatalf("result slices must be empty, not nil: %+v", res)
	}
}
