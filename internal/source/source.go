package source

import (
	"context"
	"time"

	"github.com/RakhaYandra/hibiki/internal/event"
)

type Query struct {
	Since  time.Time
	Until  time.Time
	BootID string
	Limit  int
}

type Status struct {
	Name      string
	Available bool
	Detail    string
	Err       error
}

// Limitation is not a failure: the query still holds events, and the caller
// reports this next to them instead of failing the whole command. A source
// returns it when it had to leave something out.
type Limitation struct{ Message string }

func (l *Limitation) Error() string { return l.Message }

type Iterator interface {
	Next(ctx context.Context) (event.Event, error)
	Close() error
}

type Source interface {
	Name() string
	Available(ctx context.Context) Status
	Query(ctx context.Context, q Query) (Iterator, error)
}
