package event

import (
	"fmt"
	"hash/fnv"
)

// A journal cursor is unique inside one journal only, not across machines or
// journals (PRD 7.5), so the source type and boot id join the hash input.
func IDFor(src Source) string {
	if src.Cursor != nil && *src.Cursor != "" {
		return "evt_" + shortHash(src.Type, derefString(src.BootID), *src.Cursor)
	}
	return IDForFallback(src.Type, derefString(src.BootID))
}

func IDForFallback(kind string, parts ...string) string {
	return "evt_" + shortHash(append([]string{kind}, parts...)...)
}

func shortHash(parts ...string) string {
	h := fnv.New64a()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return fmt.Sprintf("%016x", h.Sum64())[:12]
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
