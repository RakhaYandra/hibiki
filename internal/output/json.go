package output

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

var nowFunc = time.Now

// A raw source.Status would marshal its error as {}, so consumers get an
// explicit projection instead.
type SourceStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
}

// The envelope carries the limitations and the covered range next to the
// events, so a machine consumer can tell a full answer from a partial one
// (PRD 8.4).
type Envelope struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	CoveredSince  *time.Time     `json:"covered_since"`
	CoveredUntil  *time.Time     `json:"covered_until"`
	Truncated     bool           `json:"truncated"`
	NextCursor    *string        `json:"next_cursor"`
	Sources       []SourceStatus `json:"sources"`
	Limitations   []string       `json:"limitations"`
	Events        []event.Event  `json:"events"`
}

// RenderJSON streams the envelope. encoding/json builds the whole document in
// one buffer before writing a byte, which with ten thousand events means a
// second copy of the result and the peak RSS of the command (PRD 14). Events is
// the last field, so the array is written one event at a time.
func RenderJSON(w io.Writer, res timeline.Result) error {
	env := NewEnvelope(res)

	head, err := json.MarshalIndent(envelopeHead{
		SchemaVersion: env.SchemaVersion,
		GeneratedAt:   env.GeneratedAt,
		CoveredSince:  env.CoveredSince,
		CoveredUntil:  env.CoveredUntil,
		Truncated:     env.Truncated,
		NextCursor:    env.NextCursor,
		Sources:       env.Sources,
		Limitations:   env.Limitations,
	}, "", "  ")
	if err != nil {
		return err
	}
	if n := len(head); n == 0 || head[n-1] != '}' {
		return errors.New("cannot render the envelope head")
	}
	// Drop the closing brace and the newline before it, then the events array
	// closes the object.
	head = bytes.TrimRight(head[:len(head)-1], "\n")
	head = append(head, ',')

	out := bufio.NewWriter(w)
	if _, err := out.Write(head); err != nil {
		return err
	}
	if _, err := out.WriteString("\n  \"events\": ["); err != nil {
		return err
	}
	for i, e := range env.Events {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if i > 0 {
			if err := out.WriteByte(','); err != nil {
				return err
			}
		}
		if _, err := out.WriteString("\n    "); err != nil {
			return err
		}
		if _, err := out.Write(line); err != nil {
			return err
		}
	}
	if len(env.Events) > 0 {
		if _, err := out.WriteString("\n  "); err != nil {
			return err
		}
	}
	if _, err := out.WriteString("]\n}\n"); err != nil {
		return err
	}
	return out.Flush()
}

// envelopeHead mirrors Envelope without the events, in the same order.
type envelopeHead struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	CoveredSince  *time.Time     `json:"covered_since"`
	CoveredUntil  *time.Time     `json:"covered_until"`
	Truncated     bool           `json:"truncated"`
	NextCursor    *string        `json:"next_cursor"`
	Sources       []SourceStatus `json:"sources"`
	Limitations   []string       `json:"limitations"`
}

func RenderEventJSON(w io.Writer, e event.Event) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}

func NewEnvelope(res timeline.Result) Envelope {
	env := Envelope{
		SchemaVersion: event.SchemaVersion,
		GeneratedAt:   nowFunc().UTC(),
		Truncated:     res.Truncated,
		Sources:       make([]SourceStatus, 0, len(res.Sources)),
		Limitations:   sliceOrEmpty(res.Limitations),
		Events:        sliceOrEmpty(res.Events),
	}
	if !res.CoveredSince.IsZero() {
		since := res.CoveredSince.UTC()
		env.CoveredSince = &since
	}
	if !res.CoveredUntil.IsZero() {
		until := res.CoveredUntil.UTC()
		env.CoveredUntil = &until
	}
	if res.NextCursor != "" {
		cursor := res.NextCursor
		env.NextCursor = &cursor
	}
	env.Sources = SourceStatuses(res.Sources)
	return env
}

func SourceStatuses(in []source.Status) []SourceStatus {
	out := make([]SourceStatus, 0, len(in))
	for _, s := range in {
		status := SourceStatus{Name: s.Name, Available: s.Available, Detail: s.Detail}
		if s.Err != nil {
			status.Error = s.Err.Error()
		}
		out = append(out, status)
	}
	return out
}

func sliceOrEmpty[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}
