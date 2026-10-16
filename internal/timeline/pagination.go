package timeline

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
)

// The cursor carries every field orderLess compares, not just the timestamp:
// journal records share microseconds in bursts, so a timestamp-only cursor
// would repeat events or loop forever when resuming.
type cursorPayload struct {
	TimestampUS int64   `json:"t"`
	MonotonicUS *int64  `json:"m"`
	BootID      *string `json:"b"`
	SourceType  string  `json:"s"`
	ID          string  `json:"i"`
}

func EncodeCursor(e event.Event) string {
	payload, err := json.Marshal(cursorPayload{
		TimestampUS: e.TimestampUnixUS,
		MonotonicUS: e.TimestampMonotonicUS,
		BootID:      e.Source.BootID,
		SourceType:  e.Source.Type,
		ID:          e.ID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func DecodeCursor(s string) (event.Event, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err == nil {
		var payload cursorPayload
		if json.Unmarshal(raw, &payload) == nil && payload.ID != "" {
			return event.Event{
				ID:                   payload.ID,
				TimestampUnixUS:      payload.TimestampUS,
				TimestampMonotonicUS: payload.MonotonicUS,
				Source:               event.Source{Type: payload.SourceType, BootID: payload.BootID},
			}, nil
		}
	}
	return event.Event{}, exit.Usage("%q is not a usable cursor: pass a cursor from a previous run or drop the flag", s)
}

func decodeAfter(after string) (*event.Event, error) {
	if after == "" {
		return nil, nil
	}
	e, err := DecodeCursor(after)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
