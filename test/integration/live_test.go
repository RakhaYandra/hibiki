package integration

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/source"
	"github.com/RakhaYandra/hibiki/internal/source/journal"
)

// Live checks are opt-in: HIBIKI_LIVE=1 makes the suite read the real journal,
// so a developer machine is never the only source of truth (PRD 15.2).
func TestLiveJournalQuery(t *testing.T) {
	if os.Getenv("HIBIKI_LIVE") == "" {
		t.Skip("set HIBIKI_LIVE=1 to query the real journal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	src := journal.New()
	if st := src.Available(ctx); !st.Available {
		t.Fatalf("journal must be readable in this environment: %+v", st)
	}

	start := time.Now()
	it, err := src.Query(ctx, source.Query{Since: time.Now().Add(-30 * time.Minute), Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	count := 0
	for {
		ev, err := it.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next after %d events: %v", count, err)
		}
		count++
		if err := ev.Validate(); err != nil {
			t.Fatalf("event %s does not validate: %v", ev.ID, err)
		}
		if ev.Source.BootID == nil || ev.Source.Cursor == nil {
			t.Fatalf("event %s lost its provenance: %+v", ev.ID, ev.Source)
		}
	}
	t.Logf("events=%d elapsed=%s", count, time.Since(start))
	if count == 0 {
		t.Fatal("expected at least one event in the last 30 minutes")
	}
}

func TestLiveEmptyWindowIsSuccess(t *testing.T) {
	if os.Getenv("HIBIKI_LIVE") == "" {
		t.Skip("set HIBIKI_LIVE=1 to run live checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	it, err := journal.New().Query(ctx, source.Query{Since: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	if _, err := it.Next(ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("an empty window must read as io.EOF, got %v", err)
	}
}
