// Package allergytest is the in-memory allergy.Repository and a scriptable
// allergy.Authorizer, for internal/service/allergy's own unit tests.
package allergytest

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/allergy"
)

const (
	OwnerID    = "mkfakeowner0001"
	StrangerID = "mkfakestrangr01"

	PatientID         = "mkfakepatient01"
	StrangerPatientID = "mkfakestrangp1"
)

var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

// Repository is the second implementation of allergy.Repository Principle I
// requires. It is a plain full scan: pagination correctness is proven against
// a real instance by recordstest.RunRepositoryContract, and this fake exists
// for the service's own business-rule tests.
type Repository struct {
	mu    sync.Mutex
	rows  map[string]clinical.Allergy
	next  int
	clock time.Time
	calls []string
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]clinical.Allergy), clock: epoch}
}

func (r *Repository) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}

func (r *Repository) List(_ context.Context, patientID string, query allergy.Query) (domain.Page[clinical.Allergy], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "list")

	matched := make([]clinical.Allergy, 0, len(r.rows))

	for _, row := range r.rows {
		if row.PatientID == patientID {
			matched = append(matched, row)
		}
	}

	slices.SortFunc(matched, func(left, right clinical.Allergy) int {
		return strings.Compare(right.ID, left.ID)
	})

	limit := query.Limit
	if limit <= 0 || limit > len(matched) {
		limit = len(matched)
	}

	return domain.NewPage(matched[:limit], nil), nil
}

func (r *Repository) Get(_ context.Context, id string) (clinical.Allergy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "get")

	return r.byID(id)
}

func (r *Repository) Create(_ context.Context, entity clinical.Allergy) (clinical.Allergy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "create")
	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakealg%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.Allergy, expectedVersion string) (clinical.Allergy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "update")

	current, err := r.byID(entity.ID)
	if err != nil {
		return clinical.Allergy{}, err
	}

	if current.Version != expectedVersion {
		return clinical.Allergy{}, domain.ErrVersionMismatch
	}

	r.clock = r.clock.Add(time.Millisecond)
	entity.PatientID = current.PatientID
	entity.CreatedAt = current.CreatedAt
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, revision(current.Version)+1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Delete(_ context.Context, id, expectedVersion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "delete")

	current, err := r.byID(id)
	if err != nil {
		return err
	}

	if current.Version != expectedVersion {
		return domain.ErrVersionMismatch
	}

	delete(r.rows, id)

	return nil
}

func (r *Repository) byID(id string) (clinical.Allergy, error) {
	row, exists := r.rows[id]
	if !exists {
		return clinical.Allergy{}, fmt.Errorf("allergytest: no such record: %w", domain.ErrNotFound)
	}

	return row, nil
}

func version(id string, revision int) string { return id + "-" + strconv.Itoa(revision) }

func revision(version string) int {
	_, digits, found := strings.Cut(version, "-")
	if !found {
		return 0
	}

	parsed, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}

	return parsed
}

// Authorizer is a checkpoint that answers for one account.
type Authorizer struct {
	mu    sync.Mutex
	owner string
	level access.Permission
	err   error
	calls int
}

func NewAuthorizer(owner string) *Authorizer {
	return &Authorizer{owner: owner, level: access.PermOwn}
}

func (a *Authorizer) Refuse(err error) *Authorizer {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.err = err

	return a
}

func (a *Authorizer) Grant(level access.Permission) *Authorizer {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.level = level

	return a
}

func (a *Authorizer) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.calls
}

func (a *Authorizer) Patient(_ context.Context, actor access.Actor, _ string, _ access.Permission) (access.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.calls++

	if a.err != nil {
		return access.Grant{}, a.err
	}

	if actor.UserID != a.owner {
		return access.Grant{}, domain.ErrNotFound
	}

	return access.Grant{Level: a.level}, nil
}
