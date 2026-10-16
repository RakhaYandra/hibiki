package journal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
)

// Shaped like a user unit record from the real journal: one boot id, a unit,
// a priority and an ordinary message (spike 8).
func benchRecord(i int) map[string]string {
	us := int64(1_792_000_000_000_000) + int64(i)*1000
	return map[string]string{
		"__REALTIME_TIMESTAMP":  strconv.FormatInt(us, 10),
		"__MONOTONIC_TIMESTAMP": strconv.FormatInt(int64(i)*1000+497_042_663_000, 10),
		"__CURSOR":              fmt.Sprintf("s=bench;i=%d", i),
		"_BOOT_ID":              "30107f27d01e4e42b24846e4da81db1a",
		"_HOSTNAME":             "bench",
		"_TRANSPORT":            "journal",
		"_PID":                  "2401",
		"_COMM":                 "python3",
		"_EXE":                  "/usr/bin/python3",
		"_SYSTEMD_UNIT":         "user@1000.service",
		"_SYSTEMD_USER_UNIT":    "omarchy-speaker-loudness.service",
		"MESSAGE_ID":            "d9b3738c0d9c4a2f9d0a5f1c1d0e1a2b",
		"PRIORITY":              "6",
		"MESSAGE":               "Starting Omarchy speaker loudness compensation...",
	}
}

func benchRecords(n int) []map[string]string {
	records := make([]map[string]string, 0, n)
	for i := 0; i < n; i++ {
		records = append(records, benchRecord(i))
	}
	return records
}

// 4135 records is what the real journal held in the 30 minute window measured
// by the spike, so the numbers are comparable to that run.
func BenchmarkNormalize30mFixture(b *testing.B) {
	records := benchRecords(4135)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, fields := range records {
			if _, err := Normalize(fields); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkDecodeNormalize10k(b *testing.B) {
	records := benchRecords(10000)
	var buf bytes.Buffer
	for _, fields := range records {
		line, err := json.Marshal(fields)
		if err != nil {
			b.Fatal(err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	data := buf.Bytes()

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		err := Decode(bytes.NewReader(data), func(fields map[string]string) error {
			if _, err := Normalize(fields); err != nil {
				return err
			}
			count++
			return nil
		}, nil)
		if err != nil {
			b.Fatal(err)
		}
		if count != 10000 {
			b.Fatalf("decoded %d records", count)
		}
	}
}
