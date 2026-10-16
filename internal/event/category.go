package event

// Unit lifecycle MESSAGE_ID values as emitted by systemd 261, captured from
// journalctl on the reference machine (docs/spike-2026-10-16.md).
var lifecycleMessageIDs = map[string]bool{
	"7d4958e842da4a758f6c1cdc7b36dcc5": true,
	"39f53479d3a045ac8e11786248231fbf": true,
	"7ad2d189f7e94e70a38c781354912448": true,
	"98e322203f7a4ed290d09fe03c09fe15": true,
	"d9b373ed55a64feb8242e02dbe79a49c": true,
	"5eb03494b6584870a536b337290809b3": true,
	"ae8f7b866b0347b9af31fe1c80b127c0": true,
}

// Classification is a fixed rule table over source fields; message text never
// takes part, so the same record always lands in the same category (PRD 7.3).
func CategoryFromJournal(fields map[string]string) Category {
	if lifecycleMessageIDs[fields["MESSAGE_ID"]] {
		return CategoryService
	}
	switch fields["_TRANSPORT"] {
	case "kernel":
		return CategorySystem
	case "audit":
		return CategorySecurity
	}
	if fields["_SYSTEMD_UNIT"] != "" || fields["_SYSTEMD_USER_UNIT"] != "" {
		return CategoryApplication
	}
	switch fields["_TRANSPORT"] {
	case "stdout", "syslog", "journal":
		return CategoryApplication
	}
	return CategoryUnknown
}
