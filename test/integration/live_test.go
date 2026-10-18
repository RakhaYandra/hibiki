package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
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

func TestLiveIncidentsCommand(t *testing.T) {
	if os.Getenv("HIBIKI_LIVE") == "" {
		t.Skip("set HIBIKI_LIVE=1 to run live checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if st := journal.New().Available(ctx); !st.Available {
		t.Fatalf("journal must be readable in this environment: %+v", st)
	}

	// The fixture variable is dropped rather than inherited: this check is about
	// the real journal, not a fixture left in the shell.
	start := time.Now()
	res := runCLI(t, envWith(nil, "HIBIKI_JOURNAL_FILE"),
		"incidents", "--since", "30m", "--format", "json")
	t.Logf("elapsed=%s", time.Since(start))
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %s", res.code, res.stderr)
	}

	// The typed decode below cannot see a renamed key, so the array is looked up
	// on the document itself.
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(res.stdout), &keys); err != nil {
		t.Fatalf("stdout is not a JSON document: %v\n%s", err, res.stdout)
	}
	if _, ok := keys["incidents"]; !ok {
		t.Fatalf("the document carries no incidents array:\n%s", res.stdout)
	}

	var doc struct {
		SchemaVersion int `json:"schema_version"`
		Incidents     []struct {
			ID         string `json:"id"`
			Rule       string `json:"rule"`
			Confidence string `json:"confidence"`
			Reason     string `json:"reason"`
			Primary    struct {
				ID string `json:"id"`
			} `json:"primary"`
			Related []struct {
				ID string `json:"id"`
			} `json:"related"`
			WindowStart time.Time `json:"window_start"`
			WindowEnd   time.Time `json:"window_end"`
		} `json:"incidents"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
		t.Fatalf("stdout is not a Hibiki incidents document: %v\n%s", err, res.stdout)
	}
	if doc.SchemaVersion != event.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", doc.SchemaVersion, event.SchemaVersion)
	}
	t.Logf("incidents=%d", len(doc.Incidents))

	// A quiet machine must pass this test, so the counts are never asserted.
	// Every incident the journal did produce still has to be complete.
	for _, inc := range doc.Incidents {
		switch {
		case !strings.HasPrefix(inc.ID, "inc_"):
			t.Errorf("incident id %q does not name itself", inc.ID)
		case inc.Rule == "":
			t.Errorf("incident %s lost its rule", inc.ID)
		case inc.Confidence != "high" && inc.Confidence != "medium" && inc.Confidence != "low":
			t.Errorf("incident %s confidence = %q", inc.ID, inc.Confidence)
		case inc.Reason == "":
			t.Errorf("incident %s lost its reason", inc.ID)
		case inc.Primary.ID == "":
			t.Errorf("incident %s lost its primary", inc.ID)
		case len(inc.Related) == 0:
			t.Errorf("incident %s carries no evidence", inc.ID)
		case !inc.WindowEnd.After(inc.WindowStart):
			t.Errorf("incident %s window %s .. %s does not move forward", inc.ID, inc.WindowStart, inc.WindowEnd)
		}
	}
}
