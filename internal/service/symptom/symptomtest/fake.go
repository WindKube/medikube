// Package symptomtest is the in-memory Repository the unit tier and the
// registration-tier contract run against.
package symptomtest

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/symptom"
)

// FakeRepository is symptom.Repository, in memory. It computes FR-031's
// aggregate on every read exactly as the real store does, over
// (patient, LOWER(name)) — so a test proves the service and the aggregate
// together without a database.
type FakeRepository struct {
	mu   sync.Mutex
	next int
	byID map[string]clinical.Symptom
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{byID: make(map[string]clinical.Symptom)}
}

var _ symptom.Repository = (*FakeRepository)(nil)

func (r *FakeRepository) List(_ context.Context, patientID string, query symptom.Query) (domain.Page[clinical.Symptom], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var ids []string
	for id, row := range r.byID {
		if row.PatientID == patientID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	items := make([]clinical.Symptom, 0, len(ids))
	for _, id := range ids {
		items = append(items, r.withAggregate(patientID, r.byID[id]))
	}

	sortItems(items, query.Sort)

	limit := query.Limit
	if limit == 0 {
		limit = 25
	}

	if len(items) > limit {
		items = items[:limit]
	}

	return domain.NewPage(items, nil), nil
}

func (r *FakeRepository) Get(_ context.Context, id string) (clinical.Symptom, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	row, ok := r.byID[id]
	if !ok {
		return clinical.Symptom{}, domain.ErrNotFound
	}

	return r.withAggregate(row.PatientID, row), nil
}

func (r *FakeRepository) Create(_ context.Context, s clinical.Symptom) (clinical.Symptom, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.next++
	s.ID = "mkfakesymptom" + strconv.Itoa(r.next)
	s.Version = "v1"
	r.byID[s.ID] = s

	return r.withAggregate(s.PatientID, s), nil
}

func (r *FakeRepository) Update(_ context.Context, s clinical.Symptom, expectedVersion string) (clinical.Symptom, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.byID[s.ID]
	if !ok {
		return clinical.Symptom{}, domain.ErrNotFound
	}

	if current.Version != expectedVersion {
		return clinical.Symptom{}, domain.ErrVersionMismatch
	}

	s.Version = current.Version + "'"
	r.byID[s.ID] = s

	return r.withAggregate(s.PatientID, s), nil
}

func (r *FakeRepository) Delete(_ context.Context, id, expectedVersion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.byID[id]
	if !ok {
		return domain.ErrNotFound
	}

	if current.Version != expectedVersion {
		return domain.ErrVersionMismatch
	}

	delete(r.byID, id)

	return nil
}

// withAggregate is FR-031, computed fresh on every read.
func (r *FakeRepository) withAggregate(patientID string, s clinical.Symptom) clinical.Symptom {
	key := strings.ToLower(s.Name)

	count := 0
	var last clinical.Instant

	for _, row := range r.byID {
		if row.PatientID != patientID || strings.ToLower(row.Name) != key {
			continue
		}

		count++

		if last.IsZero() || row.OccurredAt.After(last) {
			last = row.OccurredAt
		}
	}

	s.EpisodeCount = count
	s.LastOccurredAt = last

	return s
}

func sortItems(items []clinical.Symptom, keys []domain.SortKey) {
	if len(keys) == 0 {
		return
	}

	sort.SliceStable(items, func(i, j int) bool {
		for _, key := range keys {
			less, equal := compare(items[i], items[j], key)
			if !equal {
				return less
			}
		}

		return items[i].ID > items[j].ID
	})
}

func compare(a, b clinical.Symptom, key domain.SortKey) (less, equal bool) {
	var av, bv string

	switch key.Field {
	case symptom.FieldName:
		av, bv = strings.ToLower(a.Name), strings.ToLower(b.Name)
	default:
		av, bv = a.OccurredAt.String(), b.OccurredAt.String()
	}

	if av == bv {
		return false, true
	}

	if key.Desc {
		return av > bv, false
	}

	return av < bv, false
}

// Authorizer grants everything for OwnerID's patients and refuses everybody
// else with ErrNotFound.
type Authorizer struct {
	OwnerID string
}

func (a Authorizer) Patient(_ context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error) {
	if patientID == "" || actor.UserID != a.OwnerID {
		return access.Grant{}, domain.ErrNotFound
	}

	grant := access.Grant{Level: access.PermOwn}
	if !grant.Allows(need) {
		return access.Grant{}, domain.ErrNotFound
	}

	return grant, nil
}
