# Troubleshooting

## The timeline is empty

An empty result is a successful query, not an error: exit code 0 and a line
that says so.

```
no events matched in the requested window
covered: nothing observed
```

"covered: nothing observed" is deliberate: coverage describes what the answer
proves, and an empty answer proves nothing about the window.

Check, in this order:

1. Is the window what you think it is? `--since 30m` means the last 30 minutes
   from the moment the query started.
2. Does the journal reach that far back? `hibiki sources` prints the reachable
   range. Rolled over or vacuumed records are gone.
3. Is a filter narrowing it to nothing? `--severity error --since 5m` on a quiet
   service returns nothing, correctly.

## `hibiki: journalctl is not installed or not on PATH` (exit 3)

Hibiki shells out to `journalctl`. If PATH is empty or journalctl is missing,
there is nothing to read.

```bash
command -v journalctl
hibiki doctor
```

`hibiki sources --check` is the scripted form of this check: it prints the full
source report first, then exits 3 when a source that queries cannot be read.

## `unit not found: <unit>` on stderr, exit 0

The unit name you passed does not exist now, in the system scope or in your user
scope. The query still ran, because a unit can have been removed after the
events it produced were recorded. Check the name:

```bash
systemctl status <unit> --no-pager
systemctl --user status <unit> --no-pager
```

If the unit existed and produced events you can see in the output, the warning is
just a warning.

## `--since 30m` works here but `journalctl --since 30m` failed

Hibiki parses `30m`, `2h`, `1h30m`, `2d` and RFC 3339 itself, then hands
journalctl an absolute timestamp (`--since @1792142920`). That conversion is
deliberate: the relative form was unreliable with some journalctl builds, and
computing `now` once keeps a query internally consistent.

Accepted forms:

```bash
hibiki timeline --since 30m
hibiki timeline --since 1h30m
hibiki timeline --since 2d
hibiki timeline --since 2026-10-16T08:00:00Z
hibiki timeline --since 30m --until 10m
```

A bad value is exit 2 and names the value it could not parse.

## `--boot` takes a boot id, not an offset

```bash
hibiki timeline --since 2d --boot 30107f27-d01e-4e42-b248-46e4da81db1a
hibiki timeline --since 2d --boot 30107f27d01e4e42b24846e4da81db1a
```

Both forms work; the value is normalized once. `hibiki timeline --boot 0` is not
a boot id and exits 2, because journalctl's offset form (`-b -1`) cannot also be
used as a filter on the event model, and silently returning nothing would be
worse than saying so. Get an id from `hibiki inspect <event-id>`; the raw
`_BOOT_ID` field is there too.

## Reading a journalctl export without the live journal

Set `HIBIKI_JOURNAL_FILE` to a file of `journalctl -o json` output and Hibiki
reads that instead of running journalctl. It is the same code path, so this is
also how the test suite runs without touching a real journal.

```bash
journalctl -o json --since "2026-10-16 08:00" > /tmp/window.jsonl
HIBIKI_JOURNAL_FILE=/tmp/window.jsonl hibiki timeline --since 2026-10-16T08:00:00Z
```

Records that cannot be parsed are skipped and counted:

```
"limitations": ["malformed_records:2"]
```

If nothing at all could be parsed, that is exit 4 rather than a silent empty
answer.

## `--limit` shows the oldest events of the window, not the newest

`--limit 200` takes the first 200 matching events in the window and prints a
cursor to continue:

```
warning: showing the first 200 events; continue with --cursor eyJ0IjoxNzky...
```

For the newest events, narrow the window: `--since 5m` is usually what you want
instead of `--since 24h --limit 200`.

## `inspect` says there is no previous query

`inspect` reads the snapshot Hibiki wrote after the last `timeline` or `events`
query. Run one first, or point it at a saved file:

```bash
hibiki timeline --since 30m
hibiki inspect evt_7bfa3ff5c762
hibiki inspect evt_7bfa3ff5c762 --file /tmp/saved.json
```

## The event id is not in the last query

The snapshot holds one query. If the id came from an older query, run that query
again with the same window, or use `--file` with a saved result. Hibiki answers
with exit 1 and says which file it looked in.

## Everything is `unknown` severity or category

`severity` comes from the journal `PRIORITY` field and `category` from a fixed
rule table. A record with no `PRIORITY` is `unknown` rather than guessed, and a
record that matches no rule is `unknown` too. The raw fields are still there: see
`hibiki inspect <id>` for the verbatim record.

## `hibiki: requires a terminal` (exit 2)

`tui` draws on a real terminal and reads keys from it. Piped or redirected
output has neither, and `TERM=dumb` describes a terminal that cannot address the
cursor, so the TUI refuses instead of printing escape sequences into a file.

```bash
hibiki doctor            # the tui-ready check says the same thing
hibiki timeline --since 30m   # the same data, without a screen
```

The CLI commands (`timeline`, `events`, `inspect`) answer everything the TUI
shows. Nothing else is wrong with the machine.

## `hibiki: interrupted` (exit 130)

Ctrl+C in the TUI, or SIGINT while it runs, stops the query in flight, leaves
the alternate screen, and exits 130 like any interrupted Unix command. The
terminal is restored: the prompt is back at the shell and the scrollback is
untouched.

```bash
echo $?
hibiki tui --since 30m    # rerun when you want the screen back
```

Nothing was written to the journal, and the last completed query is still in
the snapshot, so `hibiki inspect` keeps working across an interrupt.

## Exit codes

| Code | Meaning                                    | Typical cause                          |
| ---- | ------------------------------------------ | -------------------------------------- |
| 0    | The query succeeded                         | Including an empty result              |
| 1    | Internal or operational error               | An unreadable snapshot, a write failure |
| 2    | Invalid argument or configuration           | `--severity bogus`, `--pid 0`, bad `--since` |
| 3    | An essential source is unavailable          | Permission denied, journalctl missing  |
| 4    | Invalid data                                | Nothing in the file decoded, damaged snapshot |
| 130  | Interrupted in the TUI                       | Ctrl+C or SIGINT, terminal restored            |

## Still stuck

```bash
hibiki doctor --format json
hibiki sources
```

`doctor` checks journalctl, systemd, journal read access, JSON validity, boot
metadata, and terminal readiness, and every failing check carries the fix it
would suggest to a human.
