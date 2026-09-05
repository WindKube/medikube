// Package encountertest is the second Repository and Authorizer Principle I
// asks for: an in-memory stand-in for internal/service/encounter's own unit
// tests, deliberately coarser than the real store (no cursor, no keyset
// paging) — the repository and kind contracts that DO hold this kind to the
// store's own fidelity run against a real instance instead
// (internal/web/api/encounter_contract_test.go).
package encountertest

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/encounter"
)

// The one account and patient every unit test in this package's sibling
// (service_test.go) builds an actor and a draft from. Fake identifiers, not
// the seeded fixture's — nothing here reaches a database.
const (
	OwnerID    = "mkfakeowner0001"
	StrangerID = "mkfakestrangr01"
	PatientID  = "mkfakepatient01"
)

// Repository is the in-memory encounter.Repository.
type Repository struct {
	mu     sync.Mutex
	rows   map[string]clinical.Encounter
	next   int
	calls  []string
	writes []string
}

func NewRepository() *Repository { return &Repository{rows: make(map[string]clinical.Encounter)} }

// Forget clears the call log between the setup half of a test and the half
// it asserts on, the same way medicationtest.Repository.Forget does.
func (r *Repository) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls, r.writes = nil, nil
}

func (r *Repository) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}

func (r *Repository) Writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.writes)
}

func (r *Repository) record(method string) {
	r.calls = append(r.calls, method)
}

func (r *Repository) List(_ context.Context, patientID string, _ encounter.Query) (domain.Page[clinical.Encounter], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("List")

	var items []clinical.Encounter

	for _, row := range r.rows {
		if row.PatientID == patientID {
			items = append(items, row)
		}
	}

	slices.SortFunc(items, func(a, b clinical.Encounter) int { return strings.Compare(a.ID, b.ID) })

	return domain.Page[clinical.Encounter]{Items: items}, nil
}

func (r *Repository) Get(_ context.Context, id string) (clinical.Encounter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("Get")

	row, ok := r.rows[id]
	if !ok {
		return clinical.Encounter{}, domain.ErrNotFound
	}

	return row, nil
}

func (r *Repository) Create(_ context.Context, entity clinical.Encounter) (clinical.Encounter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("Create")
	r.writes = append(r.writes, "Create")

	r.next++
	entity.ID = fmt.Sprintf("mkfakeenc%05d", r.next)
	entity.Version = "1"
	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.Encounter, expectedVersion string) (clinical.Encounter, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("Update")

	current, ok := r.rows[entity.ID]
	if !ok {
		return clinical.Encounter{}, domain.ErrNotFound
	}

	if current.Version != expectedVersion {
		return clinical.Encounter{}, domain.ErrVersionMismatch
	}

	r.writes = append(r.writes, "Update")

	next, _ := strconv.Atoi(current.Version)
	entity.Version = strconv.Itoa(next + 1)
	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Delete(_ context.Context, id, expectedVersion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("Delete")

	current, ok := r.rows[id]
	if !ok {
		return domain.ErrNotFound
	}

	if current.Version != expectedVersion {
		return domain.ErrVersionMismatch
	}

	r.writes = append(r.writes, "Delete")
	delete(r.rows, id)

	return nil
}

// Authorizer is a checkpoint that answers for one account, coarse the same
// way medicationtest.Authorizer is: it does not model a patient-to-account
// mapping (that is internal/service/access's own job), only whether the
// actor is the one owner it knows.
type Authorizer struct {
	mu sync.Mutex

	owner       string
	level       access.Permission
	err         error
	calls       int
	lastPatient string
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

func (a *Authorizer) LastPatient() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.lastPatient
}

func (a *Authorizer) Patient(_ context.Context, actor access.Actor, patientID string, _ access.Permission) (access.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.calls++
	a.lastPatient = patientID

	if a.err != nil {
		return access.Grant{}, a.err
	}

	if actor.UserID != a.owner {
		return access.Grant{}, domain.ErrNotFound
	}

	return access.Grant{Level: a.level}, nil
}
