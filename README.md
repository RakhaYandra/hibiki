# hibiki

Read-only Linux diagnostic CLI that normalizes the systemd journal into one
structured event model and reconstructs it as a chronological timeline.

Hibiki finds the order of events and the evidence for them. It never claims that
two events near in time caused each other.

```console
$ hibiki timeline --since 30m
17:19:28.503 | INFO     | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Scheduled restart job, restart counter is at 3542.
17:19:28.505 | INFO     | service     | omarchy-speaker-loudness.service | Started Omarchy speaker loudness compensation.
17:19:28.530 | WARNING  | service     | - | omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
17:19:28.531 | WARNING  | service     | - | omarchy-speaker-loudness.service: Failed with result 'exit-code'.
covered: 2026-10-16T10:19:28.503776Z .. 2026-10-16T10:19:28.531472Z
```

## Install

```bash
git clone https://github.com/RakhaYandra/hibiki && cd hibiki
make build            # bin/hibiki
make install          # ~/.local/bin/hibiki
```

Requirements: Linux with systemd, Go 1.27 or newer to build. Reading the journal
usually needs `systemd-journal` or `adm` group membership, or a read ACL. See
[docs/permissions.md](docs/permissions.md).

## Commands

```
hibiki
├── timeline    chronological view of a window
├── events      the same events, with the same filters
├── inspect     one event, its surroundings and its raw fields
├── incidents   the same events grouped by named correlation rules
├── sources     what can be read here, and how far back
├── doctor      six read-only checks and what to do about failures
├── tui         the same timeline, interactive
└── version
```

### timeline

```bash
hibiki timeline
hibiki timeline --since 30m
hibiki timeline --since 2h
hibiki timeline --since "2026-10-16T08:00:00Z"
hibiki timeline --since 30m --severity error
hibiki timeline --since 1h --service sshd.service
hibiki timeline --since 1h --category service
hibiki timeline --since 30m --limit 100
hibiki timeline --since 30m --format json
hibiki timeline --since 2d --boot 30107f27-d01e-4e42-b248-46e4da81db1a
```

The default window is the last 30 minutes and the default limit is 200 events.
`--since` and `--until` take a span (`30m`, `1h30m`, `2d`) or an RFC 3339
timestamp, and relative spans are resolved once at the start of the query.

`--limit` returns the first matching events of the window and prints a cursor to
continue. To see the most recent events, narrow the window.

`--boot` takes a boot id, dashed or as 32 hex digits, and keeps only the events
of that boot. A value that is not a boot id is exit 2 rather than a silent empty
answer.

### events

```bash
hibiki events
hibiki events --type service              # --type is an alias for --category
hibiki events --severity warning,error
hibiki events --service example.service
hibiki events --pid 1234
hibiki events --query "connection refused"
hibiki events --since 1h --limit 200 --format json
```

Filters combine with AND. Every value checks against the source fields, never
against the message text, except `--query`, which only ever reads the message.

### inspect

```bash
hibiki inspect evt_7bfa3ff5c762
hibiki inspect evt_7bfa3ff5c762 --context 2m
hibiki inspect evt_7bfa3ff5c762 --format json
hibiki inspect evt_7bfa3ff5c762 --file /tmp/saved.json
```

`inspect` shows the normalized event, every raw journal field, and the events
within `--context` before and after it. The event comes from the snapshot of the
last query, so an id printed by a query can still be inspected minutes later, or
from a file written with `--save`. When a correlation rule groups this event with
others, the `correlation` line names the incidents and the `Correlations` section
lists them with the window each rule looked at.

```
Event evt_7bfa3ff5c762
  time         2026-10-16T09:28:40.27772Z
  time_basis   realtime
  monotonic_us 4936375957000
  category     service
  severity     error
  source_type  systemd-journal
  host         host
  boot_id      30107f27-d01e-4e42-b248-46e4da81db1a
  cursor       s=cl;i=3
  unit         omarchy-speaker-loudness.service
  pid          1
  comm         systemd
  message      omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
  correlation  inc_171ce20e78a2, inc_5a685bcf67f9, inc_775419612656

Raw journal fields
  MESSAGE                omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
  MESSAGE_ID             98e322203f7a4ed290d09fe03c09fe15
  PRIORITY               3
  _BOOT_ID               30107f27d01e4e42b24846e4da81db1a
  _COMM                  systemd
  _HOSTNAME              host
  _PID                   1
  _SYSTEMD_UNIT          user@1000.service
  _SYSTEMD_USER_UNIT     omarchy-speaker-loudness.service
  _TRANSPORT             journal
  __CURSOR               s=cl;i=3
  __MONOTONIC_TIMESTAMP  4936375957000
  __REALTIME_TIMESTAMP   1792142920277720

Before (2 events within 2m0s)
  16:28:40.253 | INFO     | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Scheduled restart job, restart counter is at 2148.
  16:28:40.255 | INFO     | service     | omarchy-speaker-loudness.service | Started Omarchy speaker loudness compensation.

After (3 events within 2m0s)
  16:28:40.279 | ERROR    | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Failed with result 'exit-code'.
  16:28:41.280 | INFO     | service     | omarchy-speaker-loudness.service | Starting Omarchy speaker loudness compensation...
  16:28:41.281 | INFO     | service     | omarchy-speaker-loudness.service | Stopped Omarchy speaker loudness compensation.

Correlations (3)
  inc_171ce20e78a2 restart-loop [high]
    window inspected 2026-10-16T09:28:40.25372Z .. 2026-10-16T09:33:40.25372Z
    omarchy-speaker-loudness.service: 3 restart or failure events within 5m
  inc_775419612656 failed-then-error [medium]
    window inspected 2026-10-16T09:28:40.27772Z .. 2026-10-16T09:29:10.27772Z
    omarchy-speaker-loudness.service: failed, then 1 error event(s) from the same unit within 30s
  inc_5a685bcf67f9 pid-errors [low]
    window inspected 2026-10-16T09:28:40.27772Z .. 2026-10-16T09:29:10.27772Z
    pid 1: 2 error events within 30s

Sources
  systemd-journal: ok (reading test/fixtures/regression/crash-loop.jsonl from HIBIKI_JOURNAL_FILE)
```

Both sides are counted, never left blank: the sample above read two events before
and three after, inside the 2 minute `--context`. `Raw journal fields` is the
whole record, nothing elided. The `window inspected` line is the window the rule
looked at, which is not the `--context` above it.

### incidents

```bash
hibiki incidents
hibiki incidents --since 24h
hibiki incidents --since 2h --window 30s
hibiki incidents --severity error,critical
hibiki incidents --service sshd.service
hibiki incidents --since 24h --format json
```

`incidents` runs four fixed rules over the events of the window and prints what
each one grouped. Every incident carries the rule that fired, its confidence, the
window it inspected, one primary event and the events it grouped with it. The
filters, `--since`, `--until`, `--limit` and `--boot` work as they do in
`timeline`, and correlation runs over the result, after the limit.

| Rule                | Fires when                                            | Default window | Confidence |
| ------------------- | ----------------------------------------------------- | -------------- | ---------- |
| `restart-loop`      | 3 or more restart or failure events of one unit       | 5m             | high       |
| `failed-then-error` | a unit failed, then error events of the same unit     | 30s            | medium     |
| `pid-errors`        | 2 or more error events of one pid                     | 30s            | low        |
| `dependency-error`  | an error of one unit lands before another unit failed | 30s            | low        |

`--window` overrides the four windows at once; `0`, the default, keeps the
per-rule defaults, and a negative window is exit 2.

```
Incidents (3)

  inc_171ce20e78a2 restart-loop [high]
    window inspected 2026-10-16T09:28:40.25372Z .. 2026-10-16T09:33:40.25372Z
    reason omarchy-speaker-loudness.service: 3 restart or failure events within 5m
    primary   16:28:40.253 | INFO     | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Scheduled restart job, restart counter is at 2148.
    related 2
      16:28:40.277 | ERROR    | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
      16:28:40.279 | ERROR    | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Failed with result 'exit-code'.

  inc_775419612656 failed-then-error [medium]
    window inspected 2026-10-16T09:28:40.27772Z .. 2026-10-16T09:29:10.27772Z
    reason omarchy-speaker-loudness.service: failed, then 1 error event(s) from the same unit within 30s
    primary   16:28:40.277 | ERROR    | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
    related 1
      16:28:40.279 | ERROR    | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Failed with result 'exit-code'.

  inc_5a685bcf67f9 pid-errors [low]
    window inspected 2026-10-16T09:28:40.27772Z .. 2026-10-16T09:29:10.27772Z
    reason pid 1: 2 error events within 30s
    primary   16:28:40.277 | ERROR    | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
    related 1
      16:28:40.279 | ERROR    | service     | omarchy-speaker-loudness.service | omarchy-speaker-loudness.service: Failed with result 'exit-code'.
covered: 2026-10-16T09:28:40.25372Z .. 2026-10-16T09:28:41.28172Z

Sources
  systemd-journal: ok (reading test/fixtures/regression/crash-loop.jsonl from HIBIKI_JOURNAL_FILE)
```

An incident reports events that were observed together inside one window, with
the rule and its confidence next to them. Nothing in the output says one event
produced another.

`--format json` writes the same envelope as `timeline`, with `incidents` in
place of `events`:

```json
{
  "schema_version": 1,
  "generated_at": "2026-10-18T05:58:38.114757391Z",
  "covered_since": "2026-10-16T09:28:40.25372Z",
  "covered_until": "2026-10-16T09:28:41.28172Z",
  "truncated": false,
  "next_cursor": null,
  "sources": [
    {
      "name": "systemd-journal",
      "available": true,
      "detail": "reading test/fixtures/regression/crash-loop.jsonl from HIBIKI_JOURNAL_FILE"
    }
  ],
  "limitations": [],
  "incidents": [
    {"id":"inc_171ce20e78a2","rule":"restart-loop","confidence":"high","reason":"omarchy-speaker-loudness.service: 3 restart or failure events within 5m","primary":{"schema_version":1,"id":"evt_7bfa3df5c761","correlation_ids":["inc_171ce20e78a2"], ...},"related":[{"id":"evt_7bfa3ff5c762","correlation_ids":["inc_171ce20e78a2","inc_5a685bcf67f9","inc_775419612656"], ...},{"id":"evt_7bfa40f5c762","correlation_ids":["inc_171ce20e78a2","inc_5a685bcf67f9","inc_775419612656"], ...}],"window_start":"2026-10-16T09:28:40.25372Z","window_end":"2026-10-16T09:33:40.25372Z"},
    {"id":"inc_775419612656","rule":"failed-then-error","confidence":"medium","reason":"omarchy-speaker-loudness.service: failed, then 1 error event(s) from the same unit within 30s", ...},
    {"id":"inc_5a685bcf67f9","rule":"pid-errors","confidence":"low","reason":"pid 1: 2 error events within 30s", ...}
  ]
}
```

`...` marks the rest of the event, which the command writes out in full; the
field list is in [docs/event-schema.md](docs/event-schema.md).

Zero incidents is exit 0, and the two empty answers stay apart. A
`covered: nothing observed` line means no event was returned at all, while a
`covered:` range means events were read and no rule fired.

```
Incidents (0)

No incidents found.
covered: 2026-10-16T09:28:40.25372Z .. 2026-10-16T09:28:41.28172Z

Sources
  systemd-journal: ok (reading test/fixtures/regression/crash-loop.jsonl from HIBIKI_JOURNAL_FILE)
```

### sources

```bash
hibiki sources
```

```console
systemd-journal
  status    available
  detail    journal is readable
  coverage  2026-09-20T07:38:06.138Z .. 2026-10-16T10:03:23.781Z
  notes     read only; every journal field is preserved verbatim

process
  status    not available in v0.1.0
  detail    process history cannot be reconstructed from the journal; a process is only observable while it runs
```

The same report doubles as a probe: `hibiki sources --check` prints everything
above and exits 3 when a source that queries cannot be read here, so a script or
CI job can call it without parsing the output. The `process` section is
informational and never fails the check.

### doctor

```bash
hibiki doctor
hibiki doctor --format json
```

```console
journalctl     ok               systemd 261 (261.2-1-arch)
systemd        ok               systemd 261 (261.2-1-arch)
journal-read   ok               read one journal record
journal-json   ok               the record decodes as JSON
boot-metadata  ok               _BOOT_ID 30107f27-d01e-4e42-b248-46e4da81db1a
tui-ready      needs attention  stdout is not a terminal, so colored output stays off
                                fix: run Hibiki in a terminal for color; this check only affects presentation
```

`doctor` changes nothing. It prints the fix a human could apply and stops
there. An unreadable journal is exit code 3.

### tui

```bash
hibiki tui --since 30m
```

The timeline as one interactive screen: move through events, filter them, and
inspect a single event with the fields it came from. Reading is all it does, and
it runs in the alternate screen, so quitting restores the shell exactly as it
was.

```console
HIBIKI / SYSTEM TIMELINE
window 2026-10-16T08:00:00Z .. now
journal ok
> 08:00:00.000  error     service      sshd.service  Failed to start the unit
  08:00:01.000  error     service      sshd.service  Failed to start the unit
  08:00:02.000  error     service      sshd.service  Failed to start the unit
  ...
covered: 2026-10-16T08:00:00Z .. 2026-10-16T08:00:49Z
keys: j/k move  g/G top/bottom  enter inspect  / search  n/N match  f filters...
```

Keys, the table from PRD 10.2 plus the aliases the TUI also answers:

| Key                 | Action                                  |
| ------------------- | --------------------------------------- |
| `↑` / `↓` (`j` / `k`) | move through events                   |
| `g` / `G`           | jump to the top / bottom                |
| `Enter`             | open the detail pane                    |
| `/`                 | search the loaded events                |
| `f`                 | open the filter menu                    |
| `t`                 | change the time window                  |
| `r`                 | refresh                                 |
| `n` / `N`           | next / previous event that matches      |
| `Esc`               | close the detail pane or the menu       |
| `q`                 | quit, exit 0                            |
| `Ctrl+C`            | interrupt, exit 130                     |

No core function needs the mouse.

The layout follows the terminal. At 72 columns and above every row carries time,
severity, category, unit and message; below 72 the category column drops, and in
a narrow terminal the row keeps time, severity and the message, cut with an
ellipsis instead of losing the message. The detail pane takes half the screen
only when both panes still have room, and below that it takes the whole screen
with `Esc` to go back.

Outside a terminal there is no screen to draw on and no keys to read, so the
TUI refuses with exit 2 and the reason on stderr; `TERM=dumb` is refused the
same way. `hibiki doctor` reports the same decision as the `tui-ready` check,
and everything the TUI shows is reachable from `timeline`, `events` and
`inspect`.

## JSON output

`--format json` writes one envelope to stdout and nothing else; warnings go to
stderr, so a pipeline stays clean.

```bash
hibiki events --severity error --since 24h --limit 5 --format json | jq '.events | length'
hibiki timeline --since 30m --format json --save /tmp/timeline.json
```

The schema is documented in [docs/event-schema.md](docs/event-schema.md). It
carries `schema_version`, the covered range, the status of every source, the
limitations of the answer, and the events with their raw fields.

## Permissions

Hibiki reads the journal and writes only its own snapshot
(`$XDG_CACHE_HOME/hibiki/last.json`, mode 0600) or the file named with `--save`.
No command modifies the system, and no code changes a permission. Details and
the two read routes: [docs/permissions.md](docs/permissions.md).

```bash
hibiki doctor        # checks access and prints what a human could change
```

## Exit codes

| Code | Meaning                           |
| ---- | --------------------------------- |
| 0    | The query succeeded                |
| 1    | Internal or operational error      |
| 2    | Invalid argument or configuration  |
| 3    | An essential source is unavailable |
| 4    | Invalid data                       |
| 130  | Interrupted, `Ctrl+C` or SIGINT in the TUI |

An empty result is exit 0. See [docs/troubleshooting.md](docs/troubleshooting.md)
for symptoms, causes, and what to do.

## Limits and known gaps

- Process history is not reconstructable. The journal records that a process
  logged a line; it does not record what the process did between lines.
- No causality, deliberately. `incidents` reports the events one named rule saw
  inside one window, with the rule, its confidence and its evidence. No line of
  the output says one event produced another.
- No system state. `service.active_state` is `null` because a journal record
  cannot tell you the state of a unit now, only what it said then.
- `--limit 0` holds the whole result in memory, which on a 24 hour window is
  hundreds of megabytes. `--limit 200` on a 30 minute window is tens of
  milliseconds. Numbers: [docs/perf.md](docs/perf.md).

## Development

```bash
make lint test race   # gofmt, go vet, unit + integration, race detector
make release          # dist/hibiki-linux-{amd64,arm64} + SHA256SUMS
HIBIKI_LIVE=1 go test ./test/integration/   # reads the real journal, opt in
```

Tests never need the real journal: fixtures in `test/fixtures/regression/` cover
multi-line messages, two boots, missing fields, a crash loop, damaged lines, and
records with 161 fields. Set `HIBIKI_JOURNAL_FILE` to a saved
`journalctl -o json` export to query a captured window instead of the live one.

Layout and invariants: [docs/architecture.md](docs/architecture.md).

## Roadmap

- v0.3.0 (shipped): correlation engine with rule ids, evidence, and explicit
  confidence.
- Later: more adapters, journal sizes above 2 GB, Debian and Fedora permission
  matrices.

Status: v0.3.0. The journal adapter, event model, timeline engine, correlation
engine, CLI, and TUI are in scope.
