package exit

import (
	"errors"
	"fmt"
	"testing"
)

func TestFromErrorMapsCodes(t *testing.T) {
	cases := []struct {
		in   error
		want int
	}{
		{nil, 0},
		{Usage("bad flag"), 2},
		{SourceUnavailable("no journal"), 3},
		{BadData("malformed"), 4},
		{errors.New("boom"), 1},
		{fmt.Errorf("wrap: %w", Usage("x")), 2},
	}
	for _, c := range cases {
		if got := FromError(c.in); got != c.want {
			t.Errorf("FromError(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestErrorKeepsMessageAndUnwraps(t *testing.T) {
	inner := errors.New("permission denied")
	err := BadData("decode line %d: %w", 7, inner)
	if err.Error() != "decode line 7: permission denied" {
		t.Errorf("message = %q", err.Error())
	}
	if !errors.Is(err, inner) {
		t.Error("wrapped error must stay reachable")
	}
}
