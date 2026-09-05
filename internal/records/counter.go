package records

import (
	"context"
	"errors"

	"medikube/internal/service/patient"
)

// CountFunc is one kind's own indexed `COUNT(*) WHERE patient = ?`, injected
// rather than held here so this package never names PocketBase (research
// D-22): the composition root supplies the one query shape every registered
// kind's collection answers the same way.
type CountFunc func(ctx context.Context, collection, patientID string) (int, error)

// Counter is patient.RecordCounter's sole implementation, walking the
// registry rather than switching on kind (plan.md, Principle II). Phase 003
// registering eleven more kinds changes nothing here.
type Counter struct {
	registry *Registry
	count    CountFunc
}

func NewCounter(registry *Registry, count CountFunc) (*Counter, error) {
	if registry == nil {
		return nil, errors.New("records: the counter is wired with no registry, so it would answer with no kinds")
	}

	if count == nil {
		return nil, errors.New("records: the counter is wired with no count function, so every tile would be an error")
	}

	return &Counter{registry: registry, count: count}, nil
}

// CountsByKind is patient.RecordCounter's one method: one entry per
// registered kind, in registration order, including a kind with zero records
// for this patient (FR-030).
func (c *Counter) CountsByKind(ctx context.Context, patientID string) ([]patient.CountEntry, error) {
	counts, err := c.registry.CountByKind(ctx, func(ctx context.Context, collection string) (int, error) {
		return c.count(ctx, collection, patientID)
	})
	if err != nil {
		return nil, err
	}

	entries := c.registry.Entries()
	result := make([]patient.CountEntry, 0, len(entries))

	for _, entry := range entries {
		result = append(result, patient.CountEntry{
			Kind:  entry.Kind,
			Path:  "/" + entry.Segment,
			Label: entry.Inventory.Title,
			Count: counts[entry.Kind],
		})
	}

	return result, nil
}
