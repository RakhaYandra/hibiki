package event

import "testing"

func TestSeverityFromPriority(t *testing.T) {
	want := map[string]Severity{
		"0": SeverityCritical,
		"1": SeverityCritical,
		"2": SeverityCritical,
		"3": SeverityError,
		"4": SeverityWarning,
		"5": SeverityWarning,
		"6": SeverityInfo,
		"7": SeverityDebug,
	}
	for p, w := range want {
		if got := SeverityFromPriority(p); got != w {
			t.Errorf("priority %s = %q, want %q", p, got, w)
		}
	}
	for _, bad := range []string{"", "8", "-1", "err", "03", "3.0"} {
		if got := SeverityFromPriority(bad); got != SeverityUnknown {
			t.Errorf("priority %q = %q, want unknown", bad, got)
		}
	}
}
