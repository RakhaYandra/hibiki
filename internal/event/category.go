package event

// Unit lifecycle MESSAGE_ID values as emitted by systemd 261, captured from
// journalctl on the reference machine (docs/spike-2026-10-16.md and a 7 day
// scan of this boot's journal). Missing an id here means a lifecycle transition
// is misread as an application log line.
var lifecycleMessageIDs = map[string]bool{
	"7d4958e842da4a758f6c1cdc7b36dcc5": true, // Starting
	"39f53479d3a045ac8e11786248231fbf": true, // Started, Activated, Finished, Reached target
	"7ad2d189f7e94e70a38c781354912448": true, // Deactivated
	"98e322203f7a4ed290d09fe03c09fe15": true, // Main process exited
	"d9b373ed55a64feb8242e02dbe79a49c": true, // Failed with result
	"5eb03494b6584870a536b337290809b3": true, // Scheduled restart
	"ae8f7b866b0347b9af31fe1c80b127c0": true, // Failed to start
	"de5b426a63be47a7b6ac3eaac82e2f6f": true, // Stopping
	"9d1aaa27d60140bd96365438aad20286": true, // Stopped, Removed slice
	"d34d037fff1847e6ae669a370e694725": true, // Reloading
	"7b05ebc668384222baa8881179cfda54": true, // Reloaded
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
