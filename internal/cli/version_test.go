package cli

import (
	"bytes"
	"strings"
	"testing"
)

func executeForTest(args ...string) (string, error) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	out, err := executeForTest("version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.HasPrefix(out, "hibiki ") {
		t.Fatalf("output = %q", out)
	}
}
