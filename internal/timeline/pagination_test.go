package timeline

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
)

func TestRunLimitSetsTruncatedAndCursor(t *testing.T) {
	fixture := []event.Event{ev(100, "a1"), ev(200, "a2"), ev(300, "a3")}
	first, err := Run(context.Background(), []source.Source{newFakeSource("a", fixture...)}, Filter{}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 2 || !first.Truncated || first.NextCursor == "" {
		t.Fatalf("%+v", first)
	}
	second, err := Run(context.Background(), []source.Source{newFakeSource("a", fixture...)}, Filter{}, 2, first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Events) != 1 || second.Events[0].ID != "a3" || second.Truncated {
		t.Fatalf("%+v", second)
	}
}

func TestRunLimitExactDoesNotReportTruncated(t *testing.T) {
	fixture := []event.Event{ev(100, "a1"), ev(200, "a2"), ev(300, "a3")}
	res, err := Run(context.Background(), []source.Source{newFakeSource("a", fixture...)}, Filter{}, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 3 || res.Truncated || res.NextCursor != "" {
		t.Fatalf("%+v", res)
	}
}

func TestRunLimitZeroIsUnlimitedAndFlagged(t *testing.T) {
	fixture := []event.Event{ev(100, "a1"), ev(200, "a2"), ev(300, "a3")}
	res, err := Run(context.Background(), []source.Source{newFakeSource("a", fixture...)}, Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 3 || res.Truncated {
		t.Fatalf("%+v", res)
	}
	if !slices.Contains(res.Limitations, "result_not_limited") {
		t.Errorf("an unbounded result must say so: %v", res.Limitations)
	}
}

func TestRunLimitCountsMatchingEventsOnly(t *testing.T) {
	service := ev(150, "s1")
	service.Category = event.CategoryService
	service.Message = "different"
	fixture := []event.Event{ev(100, "a1"), service, ev(200, "a2"), ev(300, "a3")}
	res, err := Run(context.Background(), []source.Source{newFakeSource("a", fixture...)}, Filter{Categories: []event.Category{event.CategorySystem}}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(res.Events), []string{"a1", "a2"}; !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if !res.Truncated {
		t.Error("a3 still matches, so the result is truncated")
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "!!!", "AAAA", "eyJ0IjoxfQ", "bnVsbA"} {
		if _, err := DecodeCursor(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestCursorRoundTripKeepsBootAndID(t *testing.T) {
	boot := "boot-a"
	in := ev(300, "a3")
	in.Source.BootID = &boot
	back, err := DecodeCursor(EncodeCursor(in))
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != in.ID || back.TimestampUnixUS != in.TimestampUnixUS {
		t.Fatalf("round trip lost data: %+v", back)
	}
	if back.Source.BootID == nil || *back.Source.BootID != boot {
		t.Fatalf("boot id lost: %+v", back.Source)
	}
}

func TestRunEmptyResultHasEmptySliceNotNil(t *testing.T) {
	res, err := Run(context.Background(), []source.Source{newFakeSource("a")}, Filter{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Events == nil {
		t.Error("Events must be [] not nil so the JSON shape is stable")
	}
	if !res.CoveredSince.IsZero() || !res.CoveredUntil.IsZero() {
		t.Errorf("an empty result proves no coverage: %v %v", res.CoveredSince, res.CoveredUntil)
	}
}

func TestRunCoverageComesFromEventsNotFromFilter(t *testing.T) {
	f := Filter{Since: time.UnixMicro(10).UTC(), Until: time.UnixMicro(100000).UTC()}
	res, err := Run(context.Background(), []source.Source{newFakeSource("a", ev(100, "a1"), ev(300, "a2"))}, f, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.CoveredSince.Equal(time.UnixMicro(100).UTC()) || !res.CoveredUntil.Equal(time.UnixMicro(300).UTC()) {
		t.Fatalf("coverage must come from the events: %v %v", res.CoveredSince, res.CoveredUntil)
	}
}

func TestRunStopsEarlyWithoutLeakingIterators(t *testing.T) {
	a := newFakeSource("a", ev(1, "a1"), ev(2, "a2"), ev(3, "a3"))
	b := newFakeSource("b", ev(4, "b1"))
	res, err := Run(context.Background(), []source.Source{a, b}, Filter{}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %d", len(res.Events))
	}
	assertClosed(t, a, b)
}
