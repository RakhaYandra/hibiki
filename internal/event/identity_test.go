package event

import (
	"strings"
	"testing"
)

func TestIDForIsStableAndSourceScoped(t *testing.T) {
	cur := "s=abc;i=1;b=30107f27"
	boot := "30107f27-d01e-4e42-b248-46e4da81db1a"
	a := Source{Type: "systemd-journal", BootID: &boot, Cursor: &cur}
	if IDFor(a) != IDFor(a) {
		t.Fatal("id must be stable for unchanged input")
	}
	b := a
	other := "s=abc;i=2;b=30107f27"
	b.Cursor = &other
	if IDFor(a) == IDFor(b) {
		t.Fatal("different cursors must produce different ids")
	}
	c := a
	c.Type = "other"
	if IDFor(a) == IDFor(c) {
		t.Fatal("source type must take part in the id")
	}
	if !strings.HasPrefix(IDFor(a), "evt_") {
		t.Fatal("id must carry the evt_ prefix")
	}
}

func TestIDForWithoutCursorIsDeterministic(t *testing.T) {
	s := Source{Type: "systemd-journal"}
	if IDFor(s) != IDFor(s) {
		t.Fatal("fallback must be deterministic")
	}
	boot := "b1"
	withBoot := Source{Type: "systemd-journal", BootID: &boot}
	if IDFor(s) == IDFor(withBoot) {
		t.Fatal("fallback must differ when the boot id differs")
	}
	if IDForFallback("k", "a", "b") == IDForFallback("k", "a", "c") {
		t.Fatal("IDForFallback must include every part")
	}
}

func TestNormalizeBootIDAcceptsEveryFormTheUserHas(t *testing.T) {
	const want = "30107f27-d01e-4e42-b248-46e4da81db1a"
	cases := []string{
		"30107f27-d01e-4e42-b248-46e4da81db1a",
		"30107f27d01e4e42b24846e4da81db1a",
		"30107F27-D01E-4E42-B248-46E4DA81DB1A",
		"  30107f27d01e4e42b24846e4da81db1a  ",
	}
	for _, in := range cases {
		got, err := NormalizeBootID(in)
		if err != nil {
			t.Errorf("NormalizeBootID(%q) failed: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeBootID(%q) = %q, want %q", in, got, want)
		}
	}

	for _, bad := range []string{"", "0", "0x30107f27d01e4e42b24846e4da81db1a", "30107f27d01e4e42b24846e4da81db1", "zzzzzzzzd01e4e42b24846e4da81db1a"} {
		if _, err := NormalizeBootID(bad); err == nil {
			t.Errorf("NormalizeBootID(%q) must fail", bad)
		}
	}
}
