package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/RakhaYandra/hibiki/internal/exit"
)

const maxSpanSeconds = int64(10 * 365 * 24 * 60 * 60)

var spanPart = regexp.MustCompile(`^([0-9]+)([dhms])`)

var spanUnitSeconds = map[byte]int64{'d': 86400, 'h': 3600, 'm': 60, 's': 1}

var spanUnitRank = map[byte]int{'s': 0, 'm': 1, 'h': 2, 'd': 3}

// One time syntax only: a plain span or an RFC 3339 stamp. journalctl never
// sees the span because "--since 30m" fails to parse (spike 2), and "2d" is
// outside time.ParseDuration.
func ParseSince(s string, now time.Time) (time.Time, error) {
	if d, ok := parseSpan(s); ok {
		return now.Add(-d).UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, exit.Usage("invalid time %q: pass a span (30m, 2h, 1h30m, 2d) or an RFC 3339 timestamp", s)
	}
	return t.UTC(), nil
}

// A span for until counts back from now, matching journalctl's "-30min".
func ParseUntil(s string, now time.Time) (time.Time, error) {
	return ParseSince(s, now)
}

func ValidSpan(s string) bool {
	_, ok := parseSpan(s)
	return ok
}

// journalctl reads "@<seconds>.<microseconds>" as UTC, so a local timezone
// never leaks into the query window.
func FormatJournalArg(t time.Time) string {
	u := t.UTC()
	return fmt.Sprintf("@%d.%06d", u.Unix(), u.Nanosecond()/1000)
}

func parseSpan(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	prevRank := 4
	total := int64(0)
	rest := s
	for rest != "" {
		m := spanPart.FindStringSubmatch(rest)
		if m == nil {
			return 0, false
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || int64(n) > maxSpanSeconds {
			return 0, false
		}
		unit := m[2][0]
		if spanUnitRank[unit] >= prevRank {
			return 0, false
		}
		prevRank = spanUnitRank[unit]
		total += int64(n) * spanUnitSeconds[unit]
		if total <= 0 || total > maxSpanSeconds {
			return 0, false
		}
		rest = rest[len(m[0]):]
	}
	return time.Duration(total) * time.Second, true
}
