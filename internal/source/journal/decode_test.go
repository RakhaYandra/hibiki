package journal

import (
	"errors"
	"strings"
	"testing"

	"github.com/RakhaYandra/hibiki/internal/exit"
)

func TestDecodeKeepsUnknownFieldsAndStreams(t *testing.T) {
	var got []map[string]string
	err := Decode(strings.NewReader(strings.Join([]string{
		`{"MESSAGE":"a","_BOOT_ID":"b","WEIRD_FIELD":"keep-me"}`,
		`{"MESSAGE":"b","_BOOT_ID":"b"}`,
	}, "\n")), func(f map[string]string) error { got = append(got, f); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0]["WEIRD_FIELD"] != "keep-me" {
		t.Error("unknown fields must survive decoding")
	}
}

func TestDecodeStopsAtCallbackError(t *testing.T) {
	seen := 0
	stop := errors.New("stop")
	err := Decode(strings.NewReader(`{"MESSAGE":"a"}`+"\n"+`{"MESSAGE":"b"}`), func(map[string]string) error {
		seen++
		return stop
	})
	if !errors.Is(err, stop) || seen != 1 {
		t.Fatalf("err = %v, seen = %d", err, seen)
	}
}

func TestDecodeReportsMalformedLineWithNumber(t *testing.T) {
	err := Decode(strings.NewReader("{\"MESSAGE\":\"ok\"}\n{\"MESSAGE\":bro\n"), func(map[string]string) error { return nil })
	var e *exit.Error
	if !errors.As(err, &e) || e.Code != exit.CodeBadData {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("the message must name the line number: %v", err)
	}
}

func TestDecodeHandlesByteArrayMessage(t *testing.T) {
	var got map[string]string
	err := Decode(strings.NewReader(`{"MESSAGE":[104,101,108,108,111]}`), func(f map[string]string) error { got = f; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got["MESSAGE"] != "hello" {
		t.Errorf("MESSAGE = %q", got["MESSAGE"])
	}
	if got[sentinelLimitationsKey] != "message_non_utf8" {
		t.Errorf("a lossy decode must be flagged, got %q", got[sentinelLimitationsKey])
	}
}

func TestDecodeSkipsBlankLines(t *testing.T) {
	var got []map[string]string
	err := Decode(strings.NewReader("\n{\"MESSAGE\":\"a\"}\n\n"), func(f map[string]string) error {
		got = append(got, f)
		return nil
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("err = %v, len = %d", err, len(got))
	}
}

func TestDecodeRejectsOversizedRecord(t *testing.T) {
	big := `{"MESSAGE":"` + strings.Repeat("x", 2*1024*1024) + `"}`
	err := Decode(strings.NewReader(big), func(map[string]string) error { return nil })
	var e *exit.Error
	if !errors.As(err, &e) || e.Code != exit.CodeBadData {
		t.Fatalf("an oversized record must be bad data: %v", err)
	}
}
