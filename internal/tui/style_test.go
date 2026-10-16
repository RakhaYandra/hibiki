package tui

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func coloredStyles() *styles {
	st := newStyles(io.Discard)
	st.renderer.SetColorProfile(termenv.TrueColor)
	return st
}

func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func TestHighlightMatchesMarksEveryOccurrence(t *testing.T) {
	st := coloredStyles()
	text := "error and ERROR again"
	got := st.highlightMatches(text, "error")

	if n := strings.Count(got, "\x1b["); n < 4 {
		t.Fatalf("marks = %d, want both occurrences wrapped in an escape pair: %q", n, got)
	}
	if plain := stripANSI(got); plain != text {
		t.Fatalf("plain = %q, want the text unchanged: highlighting must not edit what it shows", plain)
	}
}

func TestHighlightMatchesLeavesOtherTextAlone(t *testing.T) {
	st := coloredStyles()
	got := st.highlightMatches("nothing to see", "error")
	if got != "nothing to see" {
		t.Fatalf("got = %q, want the text untouched when nothing matches", got)
	}
}

func TestStyledRowColoursTheSeverityCellWithoutMovingColumns(t *testing.T) {
	m := New(Deps{}, Options{})
	m.styles = coloredStyles()
	ev := manyFakeEvents(1)[0]

	styled := m.rowStyled(ev, false)
	plain := m.rowText(ev)
	if styled == plain {
		t.Fatal("styled row = plain row, want the severity cell coloured")
	}
	if stripANSI(styled) != plain {
		t.Fatalf("styled = %q, want the same text with colour added", plain)
	}
}
