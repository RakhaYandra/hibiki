# hibiki

Read-only Linux diagnostic CLI that normalizes the systemd journal into one
structured event model and reconstructs it as a chronological timeline.

```bash
hibiki timeline --since 30m
hibiki events --severity error --since 2h
hibiki inspect <event-id> --context 2m
hibiki doctor
```

Status: v0.1.0 in progress. The journal adapter, event model, timeline engine,
and CLI (`timeline`, `events`, `inspect`, `sources`, `doctor`) are the v0.1.0
scope. The TUI and the correlation engine land in v0.2.0 and v0.3.0.

Hibiki finds the order of events and the evidence for them. It never claims
that two events near in time caused each other.

## Build

```bash
make build      # bin/hibiki
make test       # go test ./...
make lint test race
make release    # dist/hibiki-linux-{amd64,arm64} + SHA256SUMS
```

Requirements: Linux with systemd, Go 1.27 or newer.
