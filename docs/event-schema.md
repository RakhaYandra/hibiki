# Event schema

`schema_version` is `1`. A consumer should reject a document whose
`schema_version` is not the one it was written against.

## Envelope

Every query returns an envelope, in JSON mode and in the saved snapshot.

| Field           | Type                  | Note                                                              |
| --------------- | --------------------- | ----------------------------------------------------------------- |
| `schema_version`| integer               | Contract version of the events inside                              |
| `generated_at`  | RFC 3339              | When Hibiki produced the answer                                    |
| `covered_since` | RFC 3339 or `null`    | Earliest event actually returned; a statement about observation    |
| `covered_until` | RFC 3339 or `null`    | Latest event actually returned                                     |
| `truncated`     | boolean               | `true` when the limit cut the result short                         |
| `next_cursor`   | string or `null`      | Set only when `truncated`; pass it to `--cursor` to continue        |
| `sources`       | array                 | Name, availability, detail, and the reason when unavailable         |
| `limitations`   | array                 | Result level limitations, see below                                |
| `events`        | array                 | The events, in chronological order                                 |

`covered_since` and `covered_until` describe the events in this answer, not the
window that was asked for. A 30 minute window that returned nothing has `null`
coverage, because nothing was observed.

## Event

| Field                    | Type             | Rule                                                        |
| ------------------------ | ---------------- | ----------------------------------------------------------- |
| `schema_version`         | integer          | `1`                                                          |
| `id`                     | string           | Stable identity of the event within its source                |
| `timestamp`              | RFC 3339         | UTC representation of realtime                                |
| `timestamp_unix_us`      | integer          | Realtime in microseconds                                      |
| `timestamp_monotonic_us` | nullable integer | Microseconds since boot, when the source provides one         |
| `time_basis`             | enum             | `realtime`, `monotonic`, or `unknown`                         |
| `source`                 | object           | `type`, `host`, `boot_id`, `cursor`                           |
| `category`               | enum             | `system`, `service`, `application`, `process`, `security`, `unknown` |
| `severity`               | enum             | `debug`, `info`, `warning`, `error`, `critical`, `unknown`    |
| `message`                | string           | Human readable message, verbatim from the source              |
| `service`                | object           | `unit`, `active_state`, `sub_state`, `result`                 |
| `process`                | object           | `pid`, `comm`, `executable`                                   |
| `correlation_ids`        | array            | Always empty in v0.1.0; the correlation engine is v0.3.0      |
| `raw`                    | object           | Every source field, verbatim                                  |
| `limitations`            | array            | Event level limitations, see below                            |

A value that is unavailable is `null`, never an empty string. An empty
`correlation_ids` means "nothing references this event", which is different from
a field nobody could read.

### Implementation notes

- `source.boot_id` carries dashes (`30107f27-d01e-4e42-...`). The journal stores
  it without them, and Hibiki normalizes both forms.
- `source.cursor` is the journal cursor. When a source record has none, the `id`
  falls back to a hash of the other identity fields and the event says
  `identity_from_fallback`.
- `service.unit` prefers `_SYSTEMD_USER_UNIT` and falls back to
  `_SYSTEMD_UNIT`. A user service is the common case on a desktop, and its unit
  name is the one a person recognizes.
- `service.active_state`, `sub_state` and `result` are always `null` from the
  journal: a journal record is evidence of the past, and the current state of a
  unit is not part of it. Reading it would be a claim about now, not then.
- `severity` follows the syslog meaning of the journal `PRIORITY`, and a record
  without `PRIORITY` is `unknown`, not `info`.
- `category` comes from a fixed rule table over the source fields (transport,
  unit fields and the systemd message ids for unit lifecycle transitions). The
  message text never takes part in the decision.

## Limitation vocabulary

Event level, inside `limitations` of one event:

| Value                          | Meaning                                                    |
| ------------------------------ | ---------------------------------------------------------- |
| `identity_from_fallback`       | The source had no cursor, so the id is derived             |
| `message_non_utf8`             | The message was a byte array and had to be converted        |
| `source_realtime_implausible`  | The realtime value is outside a plausible range             |
| `unit_unresolved`              | No unit field could be resolved                             |
| `message_truncated`            | Reserved; a message is currently never truncated            |
| `malformed_records_skipped`    | Reserved for an event that had to skip a damaged record     |

Result level, inside `limitations` of the envelope:

| Value                          | Meaning                                                       |
| ------------------------------ | ------------------------------------------------------------- |
| `source_unavailable:<name>`    | The source could not be read at all                            |
| `source_failed_midstream:<name>` | The source failed after it had already yielded events        |
| `malformed_records:<n>`        | `n` journal lines were damaged and skipped                     |
| `result_not_limited`           | `--limit 0` was asked for, so the result is not bounded        |

A damaged line makes the query keep the records that did decode and report
`malformed_records:<n>`. Only a source where nothing at all decoded is exit 4.

## Example

```json
{
  "schema_version": 1,
  "id": "evt_7bfa3ff5c762",
  "timestamp": "2026-10-16T09:28:40.27772Z",
  "timestamp_unix_us": 1792142920277720,
  "timestamp_monotonic_us": 4936375957000,
  "time_basis": "realtime",
  "source": {
    "type": "systemd-journal",
    "host": "host",
    "boot_id": "30107f27-d01e-4e42-b248-46e4da81db1a",
    "cursor": "s=cl;i=3"
  },
  "category": "service",
  "severity": "error",
  "message": "omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT",
  "service": {
    "unit": "omarchy-speaker-loudness.service",
    "active_state": null,
    "sub_state": null,
    "result": null
  },
  "process": { "pid": 1, "comm": "systemd", "executable": null },
  "correlation_ids": [],
  "raw": {
    "MESSAGE": "omarchy-speaker-loudness.service: Main process exited, code=exited, status=2/INVALIDARGUMENT",
    "MESSAGE_ID": "98e322203f7a4ed290d09fe03c09fe15",
    "PRIORITY": "3"
  },
  "limitations": []
}
```

The message above is a record shape from a real machine but the fixture it comes
from is handmade; no real journal content is published with this repository.
