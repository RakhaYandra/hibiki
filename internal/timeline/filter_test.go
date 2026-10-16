package timeline

import (
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

func TestFilterCombinations(t *testing.T) {
	base := ev(1000, "base")
	cases := []struct {
		name string
		f    Filter
		want bool
	}{
		{"empty filter", Filter{}, true},
		{"category match", Filter{Categories: []event.Category{event.CategorySystem}}, true},
		{"category miss", Filter{Categories: []event.Category{event.CategoryService}}, false},
		{"category list is OR", Filter{Categories: []event.Category{event.CategoryService, event.CategorySystem}}, true},
		{"severity multi OR", Filter{Severities: []event.Severity{event.SeverityError, event.SeverityInfo}}, true},
		{"severity miss", Filter{Severities: []event.Severity{event.SeverityError}}, false},
		{"pid match", Filter{PIDs: []int{42}}, true},
		{"pid miss", Filter{PIDs: []int{7}}, false},
		{"unit match", Filter{Units: []string{"a.service"}}, true},
		{"unit miss", Filter{Units: []string{"b.service"}}, false},
		{"keyword case-insensitive", Filter{Keyword: "CONNECTION"}, true},
		{"keyword miss", Filter{Keyword: "timeout"}, false},
		{"combined AND", Filter{Categories: []event.Category{event.CategorySystem}, PIDs: []int{7}}, false},
		{"boot match", Filter{BootID: "boot-a"}, false},
	}
	for _, c := range cases {
		if got := Match(c.f, base); got != c.want {
			t.Errorf("%s: match = %v, want %v", c.name, got, c.want)
		}
	}

	boot := "boot-a"
	withBoot := base
	withBoot.Source.BootID = &boot
	if !Match(Filter{BootID: "boot-a"}, withBoot) {
		t.Error("the boot filter must match the event's own boot id")
	}
	if Match(Filter{BootID: "boot-b"}, withBoot) {
		t.Error("the boot filter must reject a different boot")
	}
	if Match(Filter{BootID: "boot-a"}, base) {
		t.Error("a boot filter must not match an event without a boot id")
	}
}

func TestFilterBoundariesAreInclusive(t *testing.T) {
	e := ev(1000, "x")
	b := time.UnixMicro(1000).UTC()
	if !Match(Filter{Since: b, Until: b}, e) {
		t.Error("since and until must include their own instant")
	}
	if !Match(Filter{Since: b}, e) || Match(Filter{Since: time.UnixMicro(1001).UTC()}, e) {
		t.Error("since must be inclusive and directional")
	}
	if !Match(Filter{Until: b}, e) || Match(Filter{Until: time.UnixMicro(999).UTC()}, e) {
		t.Error("until must be inclusive and directional")
	}
}

func TestFilterRejectsTimelessEventWhenTimeIsBounded(t *testing.T) {
	timeless := ev(0, "no-time")
	timeless.TimeBasis = event.TimeBasisUnknown
	if Match(Filter{Since: time.UnixMicro(1).UTC()}, timeless) {
		t.Error("an event without a timestamp must never pass a bounded window")
	}
	if Match(Filter{Until: time.UnixMicro(1).UTC()}, timeless) {
		t.Error("an event without a timestamp must never pass a bounded window")
	}
	if !Match(Filter{}, timeless) {
		t.Error("without a time bound the event is still evidence")
	}
}

func TestFilterKeywordOnlySearchesMessage(t *testing.T) {
	e := ev(1000, "x")
	e.Raw["CUSTOM"] = "needle"
	if Match(Filter{Keyword: "needle"}, e) {
		t.Error("the keyword must not reach raw fields")
	}
	e.Service.Unit = strPtr("needle.service")
	if Match(Filter{Keyword: "needle"}, e) {
		t.Error("the keyword must not reach the unit name")
	}
}
