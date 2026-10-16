package timeline

import "github.com/RakhaYandra/hibiki/internal/event"

// The fallback chain keeps the view stable and repeatable. It never proves a
// causal order (PRD 8.3), and monotonic values are only comparable inside one
// boot (PRD 8.1).
func orderLess(a, b event.Event) bool {
	if a.TimestampUnixUS != b.TimestampUnixUS {
		return a.TimestampUnixUS < b.TimestampUnixUS
	}
	if sameBoot(a, b) && a.TimestampMonotonicUS != nil && b.TimestampMonotonicUS != nil {
		if *a.TimestampMonotonicUS != *b.TimestampMonotonicUS {
			return *a.TimestampMonotonicUS < *b.TimestampMonotonicUS
		}
	}
	if a.Source.Type != b.Source.Type {
		return a.Source.Type < b.Source.Type
	}
	return a.ID < b.ID
}

func sameBoot(a, b event.Event) bool {
	return a.Source.BootID != nil && b.Source.BootID != nil && *a.Source.BootID == *b.Source.BootID
}
