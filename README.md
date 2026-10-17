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
├── sources     what can be read here, and how far back
├── doctor      six read-only checks and what to do about failures
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
from a file written with `--save`.

```
Event evt_bb5b9f9dbc12
  time         2026-10-16T10:20:02.253815Z
  time_basis   realtime
  monotonic_us 8029601093
  category     service
  severity     info
  source_type  systemd-journal
  host         host
  boot_id      30107f27-d01e-4e42-b248-46e4da81db1a
  cursor       s=e49096f9...;i=1392b0;b=30107f27...;m=1de99fd45;t=65df281d997f7
  unit         -
  pid          1205
  comm         systemd
  executable   /usr/lib/systemd/systemd
  message      omarchy-speaker-loudness.service: Scheduled restart job, restart counter is at 3557.
  correlation  none (correlation arrives in v0.3.0)

Raw journal fields
  MESSAGE                omarchy-speaker-loudness.service: Scheduled restart job, restart counter is at 3557.
  MESSAGE_ID             5eb03494b6584870a536b337290809b3
  N_RESTARTS             3557
  PRIORITY               6
  SYSLOG_IDENTIFIER      systemd
  USER_UNIT              omarchy-speaker-loudness.service
  _BOOT_ID               30107f27d01e4e42b24846e4da81db1a
  _CMDLINE               /usr/lib/systemd/systemd --user
  ... every other field, verbatim

Before (1 events within 5s)
  17:19:29.505 | INFO     | service | omarchy-speaker-loudness.service | Started Omarchy speaker loudness compensation.
After (0 events within 5s)

Sources
  systemd-journal: ok (journal is readable)

Limitations
  no following events found within the window
```

An empty side is stated, never left blank: the limitation line says
`no following events found within the window`. Nothing is elided from `raw`,
only from this example.

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

An empty result is exit 0. See [docs/troubleshooting.md](docs/troubleshooting.md)
for symptoms, causes, and what to do.

## Limits and known gaps

- Process history is not reconstructable. The journal records that a process
  logged a line; it does not record what the process did between lines.
- No correlation and no causality, deliberately. `inspect` shows what happened
  before and after an event and says nothing about why. The correlation engine,
  with explicit rules and evidence, is v0.3.0.
- No system state. `service.active_state` is `null` because a journal record
  cannot tell you the state of a unit now, only what it said then.
- `--limit 0` holds the whole result in memory, which on a 24 hour window is
  hundreds of megabytes. `--limit 200` on a 30 minute window is tens of
  milliseconds. Numbers: [docs/perf.md](docs/perf.md).
- TUI: not in v0.1.0. `doctor` reports whether the terminal is ready for it.

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

- v0.2.0: interactive TUI (Phases 4 of the product plan).
- v0.3.0: correlation engine with rule ids, evidence, and explicit confidence.
- Later: more adapters, journal sizes above 2 GB, Debian and Fedora permission
  matrices.

Status: v0.1.0. The journal adapter, event model, timeline engine, and CLI are
in scope; the TUI and the correlation engine are not.
