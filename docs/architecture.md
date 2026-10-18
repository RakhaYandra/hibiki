# Architecture

Hibiki has one direction of flow: a source yields records, the normalizer turns
them into events, the timeline engine merges and filters, a renderer prints, and
a snapshot keeps the last answer around so `inspect` can find an event again.

```
journalctl --output=json            or  HIBIKI_JOURNAL_FILE=<file>
    |  os/exec with an argument array, never a shell string
    v
internal/source/journal             adapter, JSON Lines decode, normalization
    |  source.Iterator: Next/Close, ascending per source, io.EOF at the end
    v
internal/timeline                   k-way merge heap, filters, limit, cursor
    |  timeline.Result: events, coverage, sources, limitations
    +--> internal/correlation        named rules, windows, confidence, evidence
    +--> internal/output            text or a JSON envelope
    +--> internal/session           last.json snapshot (0600) for inspect
```

`inspect` reads the snapshot instead of querying again, so the event the user
picked is the event they inspected, even if the journal rotated in between. It
then runs two extra windowed queries to collect the events before and after.

`internal/correlation` sits between the merge and the presentation: it takes the
`timeline.Result` a query already produced, never a source of its own, so
correlating adds a pass over events that were already read and filtered.
`timeline` and `events` do not call it, which is why their envelopes carry no
`correlation_ids`; `incidents` calls it over the whole result, and `inspect` and
the TUI detail pane call it over the window around one event.

## Packages

| Package                     | Responsibility                                                                             |
| --------------------------- | ------------------------------------------------------------------------------------------ |
| `internal/cli`              | Cobra commands, flag parsing, exit codes, wiring of every seam                              |
| `internal/timeparse`        | `--since`/`--until` syntax (spans and RFC 3339) and the `--since @<epoch>` form journalctl needs |
| `internal/event`            | The event model, the category and severity rule tables, validation, limitation vocabulary    |
| `internal/source`           | The `Source` and `Iterator` contracts, `Query`, `Status`, `Limitation`                       |
| `internal/source/journal`   | The systemd journal adapter: process handling, JSON Lines decode, normalization              |
| `internal/source/systemd`   | Current unit state through `systemctl show`, used only for the "unit not found" warning      |
| `internal/timeline`         | Merge, filter, ordering, limit, opaque cursor                                                |
| `internal/correlation`      | The four correlation rules, their windows, confidence and evidence                           |
| `internal/output`           | Text and JSON renderers, the envelope type                                                   |
| `internal/inspect`          | The window around one event: before, after, sources, limitations                             |
| `internal/session`          | Saving and loading the last query as a restricted file                                       |
| `internal/tui`              | The interactive timeline (bubbletea); the only package that imports Charm                    |
| `internal/diagnostics`      | The six doctor checks and the shared terminal check                                          |
| `internal/exit`             | The exit code table from PRD 9.7                                                             |

## The TUI layer

`internal/tui` is a client of the engine, not a second implementation of it: it
builds a `timeline.Filter`, runs `timeline.Run` in a command goroutine, and
renders the `timeline.Result` the same CLI commands would get. The dependency
points one way.

```
internal/cli --> internal/tui --> internal/timeline
                    |                  ^
                    +-> internal/inspect
```

The rule this layout exists to protect: the core never imports Charm. Bubbletea,
lipgloss, bubbles and x/ansi live only inside `internal/tui`, so a query on the
command line and a query on the screen go through the same engine, and the rest
of the program builds, tests and runs without a terminal. Quitting cancels the
context of the query in flight and waits for it, a successful TUI query still
writes the snapshot so `hibiki inspect` can pick up an event seen on screen, and
every exit code (including 130 for an interrupt) is decided in `internal/cli`.

## One query, step by step

1. Cobra parses the flags. `--since` is resolved once against a single `now`,
   so every part of the query uses the same clock.
2. If a `--service` was given, Hibiki asks systemd whether the unit exists (in
   the system scope, then the user scope) and warns on stderr when it does not.
   The query still runs.
3. `timeline.Run` asks each source for its status and opens an iterator. The
   first event of every source goes into a min-heap ordered by
   `event.orderLess` (realtime, then monotonic only within one boot).
4. The heap is popped until the limit is reached. Every popped event is matched
   against the filter; a rejected event still advances its source.
5. Coverage is computed from the events that were actually returned, never from
   the filter. A truncated result carries a cursor built from the last returned
   event, so the next page starts there instead of rescanning.
6. The renderer prints, the snapshot is written, and the process exits with a
   code that says what happened.

## Invariants

- Everything is read-only. Hibiki runs `journalctl` and `systemctl show`, and
  writes only to its own cache directory or a path the user named.
- Each source yields events in ascending order; the engine never sorts a whole
  result set, it merges.
- Internal monotonic time is microseconds in a `*int64`, and `null` means "the
  source did not provide one".
- `raw` keeps every source field verbatim, including ones Hibiki has no rule
  for. Unknown fields are never dropped.
- A limit truncates the display of a result, never the analysis silently: a
  truncated result is flagged, and a cursor says how to continue.
- Correlation never mutates its input. `correlation.Run` sorts a copy of the
  events it was handed, and the only `correlation_ids` it writes are on the
  events inside the incidents it returns.
- The `timeline` and `events` envelopes keep `correlation_ids` empty, because
  neither command correlates. `incidents` correlates the result of a query, and
  `inspect` and the TUI detail pane correlate the window around one event, so a
  query that asked for events alone gets events alone.
- Correlation windows compare realtime only. `timestamp_monotonic_us` is never
  compared, so an event that has no monotonic value can still be matched, and
  the window never shifts under a clock change.
- Hibiki reports order and evidence. It never states that one event caused
  another, and no reason string a rule can emit does either: every one is a count
  of events inside a window.
- One cluster yields one incident. Every rule either resumes after the run it
  emitted or records a suppression deadline at the primary, so the same events
  are never the evidence of two incidents of the same rule.
- A failure to read one source degrades the answer instead of hiding it: the
  event list may still be useful, and the envelope says which source was missing.
- The core never imports Charm: bubbletea and its companions stay inside
  `internal/tui`, so the engine keeps working headless.
