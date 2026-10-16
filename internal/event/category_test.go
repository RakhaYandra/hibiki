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

func TestCategoryFromJournalIgnoresMessageText(t *testing.T) {
	f := map[string]string{"MESSAGE": "Failed to start example.service", "_TRANSPORT": "journal", "_COMM": "python3"}
	if got := CategoryFromJournal(f); got != CategoryApplication {
		t.Errorf("category = %q, want application (text must not drive classification)", got)
	}
}
