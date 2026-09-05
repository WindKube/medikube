// Package vitalstest is the in-memory Repository the unit tier and the
// registration-tier contract run against.
package vitalstest

import (
	"context"
	"sort"
	"strconv"
	"sync"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/vitals"
)

type FakeRepository struct {
	mu   sync.Mutex
	next int
	byID map[string]clinical.Vitals
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{byID: make(map[string]clinical.Vitals)}
}

var _ vitals.Repository = (*FakeRepository)(nil)

func (r *FakeRepository) List(_ context.Context, patientID string, query vitals.Query) (domain.Page[clinical.Vitals], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var ids []string
	for id, row := range r.byID {
		if row.PatientID == patientID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	items := make([]clinical.Vitals, 0, len(ids))
	for _, id := range ids {
		items = append(items, r.byID[id])
	}

	desc := len(query.Sort) == 0 || query.Sort[0].Desc

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RecordedAt.String() == items[j].RecordedAt.String() {
			return items[i].ID > items[j].ID
		}

		if desc {
			return items[i].RecordedAt.After(items[j].RecordedAt)
		}

		return items[i].RecordedAt.Before(items[j].RecordedAt)
	})

	limit := query.Limit
	if limit == 0 {
		limit = 25
	}

	if len(items) > limit {
		items = items[:limit]
	}

	return domain.NewPage(items, nil), nil
}

func (r *FakeRepository) Get(_ context.Context, id string) (clinical.Vitals, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	row, ok := r.byID[id]
	if !ok {
		return clinical.Vitals{}, domain.ErrNotFound
	}

	return row, nil
}

func (r *FakeRepository) Create(_ context.Context, v clinical.Vitals) (clinical.Vitals, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.next++
	v.ID = "mkfakemeasurements" + strconv.Itoa(r.next)
	v.Version = "v1"
	r.byID[v.ID] = v

	return v, nil
}

func (r *FakeRepository) Update(_ context.Context, v clinical.Vitals, expectedVersion string) (clinical.Vitals, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.byID[v.ID]
	if !ok {
		return clinical.Vitals{}, domain.ErrNotFound
	}

	if current.Version != expectedVersion {
		return clinical.Vitals{}, domain.ErrVersionMismatch
	}

	v.Version = current.Version + "'"
	r.byID[v.ID] = v

	return v, nil
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
