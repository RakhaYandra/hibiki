package journal

import (
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

func hasLimitation(e event.Event, want string) bool {
	for _, l := range e.Limitations {
		if l == want {
			return true
		}
	}
	return false
}

func TestNormalizeTimestampsAndSeverity(t *testing.T) {
	f := map[string]string{
		"__REALTIME_TIMESTAMP": "1792142671274265", "__MONOTONIC_TIMESTAMP": "4698621544",
		"_BOOT_ID": "30107f27d01e4e42b24846e4da81db1a", "__CURSOR": "s=abc;i=1;b=30107f27",
		"_TRANSPORT": "journal", "MESSAGE_ID": "7d4958e842da4a758f6c1cdc7b36dcc5",
		"_SYSTEMD_UNIT": "user@1000.service", "_SYSTEMD_USER_UNIT": "init.scope",
		"SYSLOG_IDENTIFIER": "systemd", "PRIORITY": "6", "_PID": "1", "_COMM": "systemd",
		"MESSAGE": "Starting Example...",
	}
	e, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	if e.TimestampUnixUS != 1792142671274265 {
		t.Errorf("realtime: %d", e.TimestampUnixUS)
	}
	if e.Timestamp.UTC() != time.UnixMicro(1792142671274265).UTC() {
		t.Error("timestamp must be UTC")
	}
	if e.TimestampMonotonicUS == nil || *e.TimestampMonotonicUS != 4698621544 {
		t.Error("monotonic must stay in microseconds as delivered")
	}
	if e.Source.BootID == nil || *e.Source.BootID != "30107f27-d01e-4e42-b248-46e4da81db1a" {
		t.Errorf("boot_id must be a dashed uuid: %v", e.Source.BootID)
	}
	if e.TimeBasis != event.TimeBasisRealtime {
		t.Errorf("time_basis = %q", e.TimeBasis)
	}
	if e.Category != event.CategoryService {
		t.Errorf("category = %q", e.Category)
	}
	if e.Severity != event.SeverityInfo {
		t.Errorf("severity = %q", e.Severity)
	}
	if e.Process.PID == nil || *e.Process.PID != 1 {
		t.Errorf("pid = %v", e.Process.PID)
	}
	if e.Service.ActiveState != nil || e.Service.Result != nil {
		t.Error("a journal record must never carry a systemd snapshot state")
	}
	if !strings.HasPrefix(e.ID, "evt_") {
		t.Errorf("id = %q", e.ID)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("normalized event must validate: %v", err)
	}
}

func TestNormalizeUnitPrefersUserUnit(t *testing.T) {
	f := map[string]string{"__REALTIME_TIMESTAMP": "1792142671274265", "_TRANSPORT": "journal",
		"_SYSTEMD_UNIT": "user@1000.service", "_SYSTEMD_USER_UNIT": "omarchy-speaker-loudness.service"}
	e, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	if e.Service.Unit == nil || *e.Service.Unit != "omarchy-speaker-loudness.service" {
		t.Fatalf("unit = %v", e.Service.Unit)
	}
	if e.Raw["_SYSTEMD_UNIT"] != "user@1000.service" || e.Raw["_SYSTEMD_USER_UNIT"] != "omarchy-speaker-loudness.service" {
		t.Error("raw must stay intact")
	}
	if hasLimitation(e, event.LimitationUnitUnresolved) {
		t.Error("a resolved user unit must not be flagged")
	}
}

func TestNormalizeMarksUnresolvedUnit(t *testing.T) {
	f := map[string]string{"__REALTIME_TIMESTAMP": "1792142671274265", "_TRANSPORT": "journal",
		"_SYSTEMD_UNIT": "user@1000.service"}
	e, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	if e.Service.Unit != nil {
		t.Fatalf("unit = %v, want nil", *e.Service.Unit)
	}
	if !hasLimitation(e, event.LimitationUnitUnresolved) {
		t.Errorf("limitations = %v", e.Limitations)
	}
}

func TestNormalizeKeepsRawForEveryInputField(t *testing.T) {
	f := map[string]string{
		"__REALTIME_TIMESTAMP": "1792142671274265", "__MONOTONIC_TIMESTAMP": "4698621544",
		"__CURSOR": "s=1;i=2", "_BOOT_ID": "30107f27d01e4e42b24846e4da81db1a",
		"_TRANSPORT": "stdout", "_PID": "2401", "_COMM": "nginx", "_EXE": "/usr/bin/nginx",
		"_HOSTNAME": "legion5", "PRIORITY": "3", "MESSAGE": "boom", "UNKNOWN_KEY": "kept",
	}
	e, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range f {
		if got, ok := e.Raw[k]; !ok || got != v {
			t.Errorf("raw[%q] = %q (present=%v), want %q", k, got, ok, v)
		}
	}
	if e.Source.Host != "legion5" {
		t.Errorf("host = %q", e.Source.Host)
	}
	if e.Process.Executable == nil || *e.Process.Executable != "/usr/bin/nginx" {
		t.Errorf("executable = %v", e.Process.Executable)
	}
}

func TestNormalizeSourceRealtimeSanity(t *testing.T) {
	base := map[string]string{"__REALTIME_TIMESTAMP": "1792142671274265", "_TRANSPORT": "kernel"}
	nanoseconds := map[string]string{}
	for k, v := range base {
		nanoseconds[k] = v
	}
	nanoseconds["_SOURCE_REALTIME_TIMESTAMP"] = "1792142671274265789"
	e, err := Normalize(nanoseconds)
	if err != nil {
		t.Fatal(err)
	}
	if !hasLimitation(e, event.LimitationSourceRealtimeImplausible) {
		t.Errorf("a nanosecond value must be flagged: %v", e.Limitations)
	}
	if e.TimestampUnixUS != 1792142671274265 {
		t.Error("an implausible value must never drive ordering")
	}

	good := map[string]string{}
	for k, v := range base {
		good[k] = v
	}
	good["_SOURCE_REALTIME_TIMESTAMP"] = "1792142671274265"
	e2, err := Normalize(good)
	if err != nil {
		t.Fatal(err)
	}
	if hasLimitation(e2, event.LimitationSourceRealtimeImplausible) {
		t.Error("a plausible value must not be flagged")
	}
}

func TestNormalizeUsesSourceRealtimeWhenRealtimeMissing(t *testing.T) {
	e, err := Normalize(map[string]string{
		"_SOURCE_REALTIME_TIMESTAMP": "1792142671274265",
		"_TRANSPORT":                 "stdout",
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.TimeBasis != event.TimeBasisRealtimeApprox || e.TimestampUnixUS != 1792142671274265 {
		t.Fatalf("basis = %q, us = %d", e.TimeBasis, e.TimestampUnixUS)
	}
}

func TestNormalizeMissingTimestampIsNotInvented(t *testing.T) {
	e, err := Normalize(map[string]string{"MESSAGE": "x", "_TRANSPORT": "kernel"})
	if err != nil {
		t.Fatal(err)
	}
	if e.TimestampUnixUS != 0 || !e.Timestamp.IsZero() {
		t.Error("a missing timestamp must never be invented")
	}
	if e.TimeBasis != event.TimeBasisUnknown {
		t.Errorf("time_basis = %q", e.TimeBasis)
	}
	if e.Severity != event.SeverityUnknown {
		t.Errorf("without PRIORITY severity must be unknown, not info, got %q", e.Severity)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("a timeless event must still validate: %v", err)
	}
}

func TestNormalizeRejectsUnparseableRealtime(t *testing.T) {
	if _, err := Normalize(map[string]string{"__REALTIME_TIMESTAMP": "not-a-number"}); err == nil {
		t.Fatal("garbage realtime must be rejected")
	}
}

func TestNormalizePropagatesDecodeLimitations(t *testing.T) {
	f := map[string]string{
		"MESSAGE": "hello", "__REALTIME_TIMESTAMP": "1792142671274265",
		sentinelLimitationsKey: "message_non_utf8",
	}
	e, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	if !hasLimitation(e, event.LimitationMessageNonUTF8) {
		t.Error("a decode limitation must be carried over")
	}
	if _, ok := e.Raw[sentinelLimitationsKey]; ok {
		t.Error("the sentinel key must not reach raw")
	}
}

func TestNormalizeStableIDFromCursor(t *testing.T) {
	f := map[string]string{"__REALTIME_TIMESTAMP": "1792142671274265", "__CURSOR": "s=1;i=2"}
	a, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Errorf("ids differ: %q vs %q", a.ID, b.ID)
	}
	f["__CURSOR"] = "s=1;i=3"
	c, err := Normalize(f)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == c.ID {
		t.Error("a different cursor must produce a different id")
	}
}

func TestNormalizeWithoutCursorFlagsFallbackIdentity(t *testing.T) {
	e, err := Normalize(map[string]string{"__REALTIME_TIMESTAMP": "1792142671274265", "MESSAGE": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasLimitation(e, event.LimitationIdentityFallback) {
		t.Errorf("limitations = %v", e.Limitations)
	}
}
