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
    +--> internal/output            text or a JSON envelope
    +--> internal/session           last.json snapshot (0600) for inspect
```

`inspect` reads the snapshot instead of querying again, so the event the user
picked is the event they inspected, even if the journal rotated in between. It
then runs two extra windowed queries to collect the events before and after.

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
| `internal/output`           | Text and JSON renderers, the envelope type                                                   |
| `internal/inspect`          | The window around one event: before, after, sources, limitations                             |
| `internal/session`          | Saving and loading the last query as a restricted file                                       |
| `internal/diagnostics`      | The six doctor checks                                                                        |
| `internal/exit`             | The exit code table from PRD 9.7                                                             |

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
- Hibiki reports order and evidence. It never states that one event caused
  another; the correlation engine that will have to justify a claim is v0.3.0.
- A failure to read one source degrades the answer instead of hiding it: the
  event list may still be useful, and the envelope says which source was missing.
