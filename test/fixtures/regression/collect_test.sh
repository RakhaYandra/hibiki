#!/usr/bin/env bash
# Read-only collection of journal fixtures from this machine. Never writes to the
# system journal; only reads it into files. Output lands in live/, which stays
# out of git: real journal records can hold paths, tokens and host names (PRD 13.3).
set -euo pipefail
out="$(cd "$(dirname "$0")" && pwd)/live"
mkdir -p "$out"

collect() {
  local expr="$1" name="$2"
  journalctl -b -o json --since "-24h" --no-pager 2>/dev/null \
    | jq -c "$expr" 2>/dev/null | head -n 200 > "$out/$name" || true
}

collect 'select(._SYSTEMD_UNIT and .MESSAGE_ID)' lifecycle.jsonl
collect 'select(.PRIORITY=="3")' errors.jsonl
collect '.' mixed.jsonl
wc -l "$out"/*.jsonl
