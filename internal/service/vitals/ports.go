package vitals

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
)

const (
	FieldRecordedAt = "recorded_at"
	FieldUpdated    = "updated"
)

const ParamSort = "sort"

// Sorts is the published ordering allowlist — dates only
// (contracts/records-clinical.md §1: vitals publishes no other narrowing).
func Sorts() []domain.SortKey {
	return []domain.SortKey{
		{Field: FieldRecordedAt, Desc: true},
		{Field: FieldRecordedAt},
		{Field: FieldUpdated, Desc: true},
		{Field: FieldUpdated},
	}
}

type Query struct {
	PatientID string
	Search    string
	Sort      []domain.SortKey
	Limit     int
	Cursor    string
	Count     bool
}

type Repository interface {
	List(ctx context.Context, patientID string, query Query) (domain.Page[clinical.Vitals], error)
	Get(ctx context.Context, id string) (clinical.Vitals, error)
	Create(ctx context.Context, vitals clinical.Vitals) (clinical.Vitals, error)
	Update(ctx context.Context, vitals clinical.Vitals, expectedVersion string) (clinical.Vitals, error)
	Delete(ctx context.Context, id, expectedVersion string) error
}

type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}
