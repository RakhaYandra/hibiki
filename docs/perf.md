# Performance notes

Measured on 2026-10-16, Arch Linux, AMD Ryzen 7 7840HS (16 threads), journal of
907 MB with roughly 26 days of retention, systemd 261, Go 1.27.2. The correlation
figures below were measured on 2026-10-18 on the same machine, when the journal
held 1008 MB.

Every number here comes from a command that was actually run. Nothing is
extrapolated.

## Budget from PRD 14

| Metric              | Target                                | Measured                                  |
| ------------------- | ------------------------------------- | ----------------------------------------- |
| CLI startup         | P95 under 100 ms, excluding the query | 1.4 ms to 2.6 ms for `hibiki version`     |
| 30 minute query     | P95 under 2 s on a fixture machine    | 34 ms for 200 events (default limit)      |
| Memory, 10k events  | under 100 MB                          | 82 MB peak RSS, `--format json`           |
| Cancellation        | the process stops after cancellation  | under 2 s after SIGINT, covered by tests  |
| TUI navigation      | input response under 100 ms, loaded   | 36 us per frame at 10,000 events, p95 asserted |
| Correlation pass    | under 500 ms over 10,000 events      | 14.9 ms for 4,520 incidents, 97 ms under `-race` |
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

## TUI navigation

```bash
go test ./internal/tui/ -run TestNavigationStaysUnderTheBudget -v
go test ./internal/tui/ -bench 'BenchmarkView|BenchmarkUpdateNavigation' -benchmem -count=3
```

| Benchmark                             | ns/op   | B/op  | allocs/op |
| ------------------------------------- | ------- | ----- | --------- |
| `BenchmarkViewWith10kEvents`          | 36,363  | 22,840 | 562      |
| `BenchmarkUpdateNavigationWith10kEvents` | 37,766 | 24,321 | 568    |

`TestNavigationStaysUnderTheBudget` loads 10,000 events into the model and
renders 200 frames at 120x40 for two inputs: moving down, and `n` pressed while
the search term matches nothing, which is the worst case for `n` because every
press scans the whole list. The 95th percentile of those frame durations must
stay under 100 ms (PRD 14), and it does: one frame costs about 36 us, more than
three orders of magnitude below the budget. Frame cost stays flat as the list
grows because only the visible window is rendered: `listRows` walks from
`offset` to `offset + rows`, never all 10,000 events.

## Correlation

```bash
go test ./internal/correlation/ -run '^$' -bench BenchmarkRun10k -benchtime 100x -benchmem
go test ./internal/correlation/ -run '^$' -bench BenchmarkRun10k -benchtime 100x -benchmem -race
go test ./internal/correlation/ -run TestRunWithinBudget -v
```

| Run                            | ns/op      | B/op       | allocs/op |
| ------------------------------ | ---------- | ---------- | --------- |
| `BenchmarkRun10k`              | 14,734,190 | 32,695,180 | 64,264    |
| `BenchmarkRun10k` with `-race` | 97,348,469 | 33,221,676 | 70,036    |

`TestRunWithinBudget` builds the same 10,000 synthetic events over 200 units and
40 time slots, refuses to time a run in which any of the four rules stayed
silent, and then times one full `Run`: 4,520 incidents in 14.9 ms against the
500 ms budget. The `-race` figure of 97 ms leaves roughly 5x headroom on this
16 thread runner. The fixture is deliberately the expensive shape: 3,920 of its
4,520 incidents come from `dependency-error` alone, and the other three rules
contribute 200 each.

The real journal tells the other side of that number. `hibiki incidents --since
7d --limit 10000 --format json` returned 10,000 events and 1 incident, a
`pid-errors` match, in 0.58 s at 79.1 MB peak RSS, so a quiet desktop never
approaches the synthetic worst case.

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

Correlation holds both sides of that answer at once: the events, and the
incident list that groups them. Measured on 2026-10-18,
`hibiki incidents --since 24h --limit 0 --format json` returned 113,243 events
and 5 incidents in 6.6 s at 737 MB to 820 MB peak RSS, where the same window
through `timeline` alone cost 699 MB to 795 MB. That day's 24 hour window held
more events than the 96,399 above, measured two days earlier, which is why both
figures moved up; the range inside each pair is the collector, over four runs in
fresh processes.

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
