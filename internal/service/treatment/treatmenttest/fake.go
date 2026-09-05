// Package treatmenttest is the second Repository and Authorizer Principle I
// asks for: an in-memory stand-in for internal/service/treatment's own unit
// tests, deliberately coarser than the real store (no cursor, no keyset
// paging) — the repository and kind contracts that DO hold this kind to the
// store's own fidelity run against a real instance instead
// (internal/web/api/treatment_contract_test.go).
package treatmenttest

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
	"medikube/internal/service/treatment"
)

const (
	OwnerID    = "mkfakeowner0001"
	StrangerID = "mkfakestrangr01"
	PatientID  = "mkfakepatient01"
)

// Repository is the in-memory treatment.Repository.
type Repository struct {
	mu     sync.Mutex
	rows   map[string]clinical.Treatment
	next   int
	calls  []string
	writes []string

	// linked is what SamePatient answers from: the set of ids this fake
	// considers to belong to PatientID. FR-057's own cross-patient refusal is
	// proven against a real instance (internal/store/treatment); this fake
	// only needs to tell a linked id from an unknown one.
	linked map[string]bool
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]clinical.Treatment), linked: make(map[string]bool)}
}

// Link marks an id as belonging to the patient a SamePatient check names, so
// a unit test can build a treatment that names it without reaching a store.
func (r *Repository) Link(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.linked[id] = true
}

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

func (r *Repository) record(method string) { r.calls = append(r.calls, method) }

func (r *Repository) List(_ context.Context, patientID string, _ treatment.Query) (domain.Page[clinical.Treatment], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("List")

	var items []clinical.Treatment

	for _, row := range r.rows {
		if row.PatientID == patientID {
			items = append(items, row)
		}
	}

	slices.SortFunc(items, func(a, b clinical.Treatment) int { return strings.Compare(a.ID, b.ID) })

	return domain.Page[clinical.Treatment]{Items: items}, nil
}

func (r *Repository) Get(_ context.Context, id string) (clinical.Treatment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("Get")

	row, ok := r.rows[id]
	if !ok {
		return clinical.Treatment{}, domain.ErrNotFound
	}

	return row, nil
}

func (r *Repository) Create(_ context.Context, entity clinical.Treatment) (clinical.Treatment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("Create")
	r.writes = append(r.writes, "Create")

	r.next++
	entity.ID = fmt.Sprintf("mkfaketrt%05d", r.next)
	entity.Version = "1"
	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.Treatment, expectedVersion string) (clinical.Treatment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("Update")

	current, ok := r.rows[entity.ID]
	if !ok {
		return clinical.Treatment{}, domain.ErrNotFound
	}

	if current.Version != expectedVersion {
		return clinical.Treatment{}, domain.ErrVersionMismatch
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

// SamePatient answers true only when every id in target was previously
// Link-ed. patientID is not consulted: this fake is not a second patient
// store, only a way to tell a linked id from an unknown one.
func (r *Repository) SamePatient(_ context.Context, _ string, target treatment.LinkTarget) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.record("SamePatient")

	for _, id := range target.IDs {
		if !r.linked[id] {
			return false, nil
		}
	}

	return true, nil
}

// Authorizer is a checkpoint that answers for one account, coarse the same
// way medicationtest.Authorizer is.
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
