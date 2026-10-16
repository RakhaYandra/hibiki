package timeparse

import (
	"testing"
	"time"
)

func TestParseSinceAcceptsSpansAndRFC3339(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"2h", 2 * time.Hour},
		{"90s", 90 * time.Second},
		{"1h30m", 90 * time.Minute},
		{"2d", 48 * time.Hour},
		{"1d2h3m4s", 24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second},
	}
	for _, c := range cases {
		got, err := ParseSince(c.in, now)
		if err != nil {
			t.Fatalf("ParseSince(%q): %v", c.in, err)
		}
		if now.Sub(got) != c.want {
			t.Errorf("ParseSince(%q) = %s, want now-%s", c.in, got, c.want)
		}
	}
	if got, err := ParseSince("2026-10-16T08:00:00Z", now); err != nil || !got.Equal(time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("rfc3339: %v %v", got, err)
	}
	if got, err := ParseSince("2026-10-16T15:00:00+07:00", now); err != nil || !got.Equal(time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("offset must land in UTC: %v %v", got, err)
	}
	if got, err := ParseSince("30m", now); err != nil || got.Location() != time.UTC {
		t.Fatalf("span result must be UTC: %v %v", got, err)
	}
}

func TestParseSinceRejectsGarbage(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	for _, bad := range []string{"", "30", "m30", "-5m", "30m ago", "yesterday", "30x", "1h30", "30m1h", "1h1h", "0m", "99999999999999999999d"} {
		if _, err := ParseSince(bad, now); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestParseUntilUsesTheSameGrammar(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 30, 0, 0, time.UTC)
	got, err := ParseUntil("15m", now)
	if err != nil || now.Sub(got) != 15*time.Minute {
		t.Fatalf("span until: %v %v", got, err)
	}
	got, err = ParseUntil("2026-10-16T08:00:00Z", now)
	if err != nil || !got.Equal(time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("absolute until: %v %v", got, err)
	}
}

func TestValidSpan(t *testing.T) {
	for _, ok := range []string{"30m", "2h", "2d", "1h30m"} {
		if !ValidSpan(ok) {
			t.Errorf("ValidSpan(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "30", "2026-10-16T08:00:00Z", "30m1h"} {
		if ValidSpan(bad) {
			t.Errorf("ValidSpan(%q) = true", bad)
		}
	}
}

func TestFormatJournalArg(t *testing.T) {
	t0 := time.Unix(1792138530, 123456000).UTC()
	if got := FormatJournalArg(t0); got != "@1792138530.123456" {
		t.Errorf("got %q", got)
	}
	t1 := time.Date(2026, 10, 16, 15, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	if got := FormatJournalArg(t1); got != "@1792137600.000000" {
		t.Errorf("non-UTC input must still format as UTC: got %q", got)
	}
}
