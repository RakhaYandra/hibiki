package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
)

const (
	initialRecordBuf = 64 * 1024
	maxRecordBytes   = 1024 * 1024

	// Records are handed to the caller as string maps, so a lossy decode has to
	// travel inside the map. Normalize takes this key out of raw.
	sentinelLimitationsKey = "__hibiki_limitations"
)

// Decode streams JSON Lines records one at a time. The callback runs per
// record, so a large journal is never buffered in memory. A malformed record is
// reported to onMalformed and skipped rather than stopping the stream: one
// damaged line must not hide the rest of the journal (PRD 14). The caller
// decides what a broken line means when nothing else decoded.
func Decode(r io.Reader, fn func(map[string]string) error, onMalformed func(line int, err error)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, initialRecordBuf), maxRecordBytes)
	line := 0
	for sc.Scan() {
		line++
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		fields, err := decodeRecord(raw)
		if err != nil {
			if onMalformed != nil {
				onMalformed(line, err)
			}
			continue
		}
		if err := fn(fields); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return exit.BadData("journal record on line %d exceeds %d bytes", line+1, maxRecordBytes)
		}
		return err
	}
	return nil
}

func decodeRecord(raw []byte) (map[string]string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	fields := make(map[string]string, len(obj))
	lossy := false
	for k, v := range obj {
		value, wasLossy := decodeValue(v)
		if wasLossy {
			lossy = true
		}
		fields[k] = value
	}
	if lossy {
		fields[sentinelLimitationsKey] = event.LimitationMessageNonUTF8
	}
	return fields, nil
}

// journald emits every field as a JSON string except a non-UTF8 MESSAGE, which
// arrives as an array of byte values (spike 8 and the journald JSON format).
func decodeValue(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, false
	}
	var bytesArr []int
	if err := json.Unmarshal(raw, &bytesArr); err == nil {
		buf := make([]byte, 0, len(bytesArr))
		for _, b := range bytesArr {
			if b < 0 || b > 255 {
				return string(raw), false
			}
			buf = append(buf, byte(b))
		}
		return string(buf), true
	}
	return string(raw), false
}
