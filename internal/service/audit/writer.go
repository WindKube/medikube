package audit

import (
	"context"
	"errors"
	"fmt"

	domainaudit "medikube/internal/domain/audit"
)

// Repository is the storage seam, declared by the consumer (Principle II).
// One method, because the trail only ever grows.
type Repository interface {
	Append(ctx context.Context, event domainaudit.Event) error
}

// Writer is the one way a row reaches the trail.
//
// It validates before it appends, and that ordering is the point: the columns
// are bounded and the three vocabularies are closed, so a row refused by the
// database would be refused AFTER the thing it records has already happened,
// with nowhere useful to report it. Refusing here turns that into an error the
// caller can join to the operation it belongs to.
//
// This is the minimal implementation US1 needs to be reachable. T240 extends
// it; the seam every caller is written against is this one method.
type Writer struct {
	repository Repository
}

func New(repository Repository) (*Writer, error) {
	if repository == nil {
		return nil, errors.New("audit: the writer is wired with no repository, so every row it accepted would be discarded silently")
	}

	return &Writer{repository: repository}, nil
}

// Record writes one event.
func (w *Writer) Record(ctx context.Context, event domainaudit.Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("audit: the event is not one that can be recorded: %w", err)
	}

	return w.repository.Append(ctx, event)
}
