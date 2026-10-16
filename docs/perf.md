# Performance notes

Measured on 2026-10-16, Arch Linux, AMD Ryzen 7 7840HS (16 threads), journal of
907 MB with roughly 26 days of retention, systemd 261, Go 1.27.2.

Every number here comes from a command that was actually run. Nothing is
extrapolated.

## Budget from PRD 14

| Metric              | Target                                | Measured                                  |
| ------------------- | ------------------------------------- | ----------------------------------------- |
| CLI startup         | P95 under 100 ms, excluding the query | 1.4 ms to 2.6 ms for `hibiki version`     |
| 30 minute query     | P95 under 2 s on a fixture machine    | 34 ms for 200 events (default limit)      |
| Memory, 10k events  | under 100 MB                          | 82 MB peak RSS, `--format json`           |
| Cancellation        | the process stops after cancellation  | under 2 s after SIGINT, covered by tests  |
| Pagination          | the whole journal is never loaded     | see the cursor section below              |

The memory figure is the peak RSS of the `hibiki` process, taken from
`resource.getrusage(RUSAGE_CHILDREN).ru_maxrss` of a wrapper that ran
`hibiki timeline --since 24h --limit 10000 --format json`. The peak follows Go's
garbage collector, so a run can land anywhere between roughly 76 MB and 90 MB
depending on when the collector fires; the streaming writer below is what keeps
it in that band instead of 270 MB.

## Benchmarks

```bash
go test ./internal/source/journal/ ./internal/timeline/ -run '^$' -bench . -benchmem
```

| Benchmark                        | Work per op                                  | ns/op      | B/op  | allocs/op |
| -------------------------------- | -------------------------------------------- | ---------- | ----- | --------- |
| `BenchmarkNormalize30mFixture`   | normalize 4135 records                       | 5,451,305  | 3.5 M | 62,036    |
| `BenchmarkDecodeNormalize10k`    | decode and normalize 10,000 JSONL records    | 68,365,951 | 33.8 M | 612,406  |
| `BenchmarkMergeTwoSources10k`    | merge two interleaved sources, 10,000 events | 4,636,866  | 14.6 M | 10,030    |

`BenchmarkDecodeNormalize10k` runs at 69 MB/s of input, which is the ceiling for
the read path: normalizing one record costs about 6.8 µs, of which most is the
per record allocation of the `raw` map.

For comparison, the spike measured `journalctl --output=json` on the same
machine returning the 30 minute window in 168 ms for 4135 records. Decoding and
normalizing those 4135 records here costs about 28 ms, so Hibiki's own work is
roughly a sixth of the time journalctl spends reading, and the external read
dominates the wall clock exactly as expected.

## Where the memory went

The first measurement of `--limit 10000 --format json` was 270 MB peak RSS,
almost three times the budget. A heap profile showed the events themselves at
34 MB (`VmHWM` 61 MB for the query alone), so the rest was duplication:

- `encoding/json` builds the whole document in one buffer before writing a
  single byte, so the rendered envelope was a second copy of every event,
  indentation included.
- The automatic session snapshot did the same thing again, and the snapshot is
  written on every query.

Both now stream: `output.RenderJSON` writes the envelope head, then one event at
a time, and the snapshot and `--save` write through the same function. A test
(`TestRenderJSONStreamsTheSameDocumentAsTheEnvelope`) compares the streamed
document against the one `json.MarshalIndent` produces from the `Envelope` type,
so the two cannot drift apart.

Nothing was dropped from the result to reach the budget, as PRD 14 requires.

## Limits that are deliberately not removed

`hibiki timeline --since 24h --limit 0 --format json` returns 96,399 events in
6.4 s and uses 658 MB peak RSS. Holding every event of a day is proportional to
data, not a leak: `--limit 0` means "no limit" and the user asked for it. The
default `--limit 200` keeps a normal query at tens of milliseconds and a few
megabytes, and a truncated result always says how to continue:

```
covered: 2026-10-16T09:28:40Z .. 2026-10-16T09:48:40Z
warning: showing the first 200 events; continue with --cursor <base64>
```

The cursor carries the sort key of the last returned event, so the journal is
never loaded to continue a query: the next page starts from that key.

## Reproducing

```bash
make build
python3 - <<'PY'
import json, resource, subprocess, time
t = time.perf_counter()
p = subprocess.run(["./bin/hibiki", "timeline", "--since", "24h", "--limit", "10000", "--format", "json"],
                   stdout=subprocess.DEVNULL)
print("exit=%d elapsed=%.2fs peak_rss=%.1fMB" % (
    p.returncode, time.perf_counter() - t,
    resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss / 1024))
PY
```

The numbers depend on the machine, the journal size and how much of it is still
on disk: the window above is bounded by what the local journal retained.
