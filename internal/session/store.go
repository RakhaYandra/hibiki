package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RakhaYandra/hibiki/internal/event"
	"github.com/RakhaYandra/hibiki/internal/exit"
	"github.com/RakhaYandra/hibiki/internal/output"
	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/timeline"
)

const (
	fileName = "last.json"
	dirMode  = 0o700
)

// The snapshot is the bridge between a query and inspect: it holds one query's
// events and coverage on disk, so inspect can be run minutes later (PRD 9.4).
func DefaultDir() string {
	if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "hibiki")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "hibiki")
	}
	return filepath.Join(home, ".cache", "hibiki")
}

func Save(dir string, res timeline.Result) (string, error) {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(output.NewEnvelope(res), "", "  ")
	if err != nil {
		return "", fmt.Errorf("cannot encode the query result: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, "last-*.json")
	if err != nil {
		return "", fmt.Errorf("cannot create a snapshot in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("cannot write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("cannot close %s: %w", tmpName, err)
	}

	path := filepath.Join(dir, fileName)
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("cannot replace %s: %w", path, err)
	}
	return path, nil
}

func Load(dir, eventID string) (event.Event, timeline.Result, error) {
	path := filepath.Join(dir, fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return event.Event{}, timeline.Result{}, fmt.Errorf("no previous query to inspect (%s is unreadable: %v); run timeline or events first", path, err)
	}
	var env output.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return event.Event{}, timeline.Result{}, exit.BadData("%s is not a Hibiki result: %v", path, err)
	}
	return FindIn(env, eventID)
}

func FindIn(env output.Envelope, eventID string) (event.Event, timeline.Result, error) {
	for _, e := range env.Events {
		if e.ID == eventID {
			return e, resultOf(env), nil
		}
	}
	return event.Event{}, timeline.Result{}, fmt.Errorf("event %s is not in the last query (%d events); run the query that produced it and inspect again", eventID, len(env.Events))
}

func resultOf(env output.Envelope) timeline.Result {
	res := timeline.Result{
		Events:      env.Events,
		Truncated:   env.Truncated,
		Limitations: env.Limitations,
		Sources:     make([]source.Status, 0, len(env.Sources)),
	}
	if res.Events == nil {
		res.Events = []event.Event{}
	}
	if res.Limitations == nil {
		res.Limitations = []string{}
	}
	if env.CoveredSince != nil {
		res.CoveredSince = *env.CoveredSince
	}
	if env.CoveredUntil != nil {
		res.CoveredUntil = *env.CoveredUntil
	}
	if env.NextCursor != nil {
		res.NextCursor = *env.NextCursor
	}
	for _, s := range env.Sources {
		status := source.Status{Name: s.Name, Available: s.Available, Detail: s.Detail}
		if s.Error != "" {
			status.Err = exit.SourceUnavailable("%s", strings.TrimSpace(s.Error))
		}
		res.Sources = append(res.Sources, status)
	}
	return res
}
