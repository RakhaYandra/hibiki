package event

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
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

// NormalizeBootID returns the dashed lowercase form of a boot id. The journal
// stores 32 hex digits, the event model carries the dashed uuid a reader
// recognizes, and journalctl accepts only the plain form, so both directions
// agree here instead of at every call site.
func NormalizeBootID(id string) (string, error) {
	plain := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(id), "-", ""))
	if len(plain) != 32 || !isHex(plain) {
		return "", errors.New("a boot id is 32 hex digits, optionally written with dashes")
	}
	return plain[0:8] + "-" + plain[8:12] + "-" + plain[12:16] + "-" + plain[16:20] + "-" + plain[20:], nil
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
