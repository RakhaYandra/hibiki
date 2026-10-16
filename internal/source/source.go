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

type Iterator interface {
	Next(ctx context.Context) (event.Event, error)
	Close() error
}

type Source interface {
	Name() string
	Available(ctx context.Context) Status
	Query(ctx context.Context, q Query) (Iterator, error)
}
