package correlation

import (
	"testing"

	"github.com/RakhaYandra/hibiki/internal/event"
)

func TestIncidentID(t *testing.T) {
	restart := restartEvents()
	got := incidentID("restart-loop", restart[0], restart[1:])
	if got != "inc_195c260735d6" {
		t.Errorf("incidentID(restart-loop) = %q, want %q", got, "inc_195c260735d6")
	}

	cluster := clusterEvents()
	got = incidentID("failed-then-error", cluster[0], cluster[1:])
	if got != "inc_77f356e7feb7" {
		t.Errorf("incidentID(failed-then-error) = %q, want %q", got, "inc_77f356e7feb7")
	}

	got = incidentID("failed-then-error", cluster[0], cluster[1:2])
	if got == "inc_77f356e7feb7" {
		t.Errorf("a smaller related set must produce a different id")
	}
}

func TestRun_RestartMessageIDCriterion(t *testing.T) {
	t.Run("info priority restart events match", func(t *testing.T) {
		events := []event.Event{
			testEvent("e1", baseUS, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e2", baseUS+1_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e3", baseUS+2_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
		}
		for i := range events {
			events[i].Raw["MESSAGE_ID"] = restartMessageID
		}
		got := Run(events, Options{})
		if len(got) != 1 {
			t.Fatalf("Run() returned %d incidents, want 1", len(got))
		}
		if got[0].Rule != "restart-loop" {
			t.Errorf("Rule = %q, want %q", got[0].Rule, "restart-loop")
		}
		if len(got[0].Related) != 2 {
			t.Errorf("len(Related) = %d, want 2", len(got[0].Related))
		}
	})

	t.Run("info events without the message id do not match", func(t *testing.T) {
		events := []event.Event{
			testEvent("e1", baseUS, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e2", baseUS+1_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
			testEvent("e3", baseUS+2_000_000, event.CategoryService, event.SeverityInfo, "nginx.service"),
		}
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})
}

func TestRun_BootGuard(t *testing.T) {
	t.Run("differing boot ids split the group", func(t *testing.T) {
		events := restartEvents()
		events[2] = withBoot(events[2], "boot-b")
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})

	t.Run("identical boot ids keep the group", func(t *testing.T) {
		events := restartEvents()
		for i := range events {
			events[i] = withBoot(events[i], "boot-a")
		}
		if got := Run(events, Options{}); len(got) != 1 {
			t.Errorf("Run() returned %d incidents, want 1", len(got))
		}
	})

	t.Run("nil and non-nil boot ids never group together", func(t *testing.T) {
		events := restartEvents()
		events[0] = withBoot(events[0], "boot-a")
		if got := Run(events, Options{}); len(got) != 0 {
			t.Errorf("Run() returned %d incidents, want 0", len(got))
		}
	})
}
