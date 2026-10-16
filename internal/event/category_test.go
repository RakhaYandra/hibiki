package event

import "testing"

func TestCategoryFromJournal(t *testing.T) {
	cases := []struct {
		name string
		f    map[string]string
		want Category
	}{
		{"kernel", map[string]string{"_TRANSPORT": "kernel"}, CategorySystem},
		{"audit", map[string]string{"_TRANSPORT": "audit"}, CategorySecurity},
		{"unit starting", map[string]string{"_TRANSPORT": "journal", "_SYSTEMD_UNIT": "a.service", "MESSAGE_ID": "7d4958e842da4a758f6c1cdc7b36dcc5"}, CategoryService},
		{"unit failed", map[string]string{"_SYSTEMD_UNIT": "a.service", "MESSAGE_ID": "d9b373ed55a64feb8242e02dbe79a49c"}, CategoryService},
		{"user unit finished", map[string]string{"_SYSTEMD_USER_UNIT": "u.service", "MESSAGE_ID": "39f53479d3a045ac8e11786248231fbf"}, CategoryService},
		{"unknown message id", map[string]string{"_SYSTEMD_UNIT": "a.service", "MESSAGE_ID": "ffffffffffffffffffffffffffffffff"}, CategoryApplication},
		{"unit app log", map[string]string{"_SYSTEMD_UNIT": "a.service"}, CategoryApplication},
		{"app stdout", map[string]string{"_TRANSPORT": "stdout", "_COMM": "nginx"}, CategoryApplication},
		{"syslog", map[string]string{"_TRANSPORT": "syslog", "SYSLOG_IDENTIFIER": "sshd"}, CategoryApplication},
		{"driver", map[string]string{"_TRANSPORT": "driver"}, CategoryUnknown},
		{"empty", map[string]string{}, CategoryUnknown},
	}
	for _, c := range cases {
		if got := CategoryFromJournal(c.f); got != c.want {
			t.Errorf("%s: category = %q, want %q", c.name, got, c.want)
		}
	}
}

// Every lifecycle id observed on the reference machine classifies as service.
// A transition is known from its id, never from reading the message text.
func TestCategoryTreatsEveryObservedLifecycleIDAsService(t *testing.T) {
	cases := []struct{ id, message string }{
		{"7d4958e842da4a758f6c1cdc7b36dcc5", "Starting example.service..."},
		{"39f53479d3a045ac8e11786248231fbf", "Started example.service."},
		{"de5b426a63be47a7b6ac3eaac82e2f6f", "Stopping example.service..."},
		{"9d1aaa27d60140bd96365438aad20286", "Stopped example.service."},
		{"d34d037fff1847e6ae669a370e694725", "Reloading example.service..."},
		{"7b05ebc668384222baa8881179cfda54", "Reloaded example.service."},
		{"7ad2d189f7e94e70a38c781354912448", "Deactivated example.service."},
		{"98e322203f7a4ed290d09fe03c09fe15", "example.service: Main process exited, code=exited, status=2/INVALIDARGUMENT"},
		{"d9b373ed55a64feb8242e02dbe79a49c", "example.service: Failed with result 'exit-code'."},
		{"5eb03494b6584870a536b337290809b3", "example.service: Scheduled restart job, restart counter is at 2148."},
		{"ae8f7b866b0347b9af31fe1c80b127c0", "Failed to start example.service."},
	}
	for _, c := range cases {
		f := map[string]string{
			"MESSAGE_ID":         c.id,
			"MESSAGE":            c.message,
			"_TRANSPORT":         "journal",
			"_SYSTEMD_USER_UNIT": "example.service",
		}
		if got := CategoryFromJournal(f); got != CategoryService {
			t.Errorf("%s (%q) = %q, want service", c.id, c.message, got)
		}
	}
}

func TestCategoryFromJournalIgnoresMessageText(t *testing.T) {
	f := map[string]string{"MESSAGE": "Failed to start example.service", "_TRANSPORT": "journal", "_COMM": "python3"}
	if got := CategoryFromJournal(f); got != CategoryApplication {
		t.Errorf("category = %q, want application (text must not drive classification)", got)
	}
}
