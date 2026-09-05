// Package emergencycontacttest is the in-memory emergencycontact.Repository
// and a scriptable emergencycontact.Authorizer, for
// internal/service/emergencycontact's own unit tests.
package emergencycontacttest

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
	"medikube/internal/service/emergencycontact"
)

const (
	OwnerID    = "mkfakeowner0001"
	StrangerID = "mkfakestrangr01"

	PatientID         = "mkfakepatient01"
	StrangerPatientID = "mkfakestrangp1"
)

var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

// Repository is the second implementation of emergencycontact.Repository. It
// applies FR-045/FR-051's primary displacement the same way the real
// repository must, because that is the one rule worth this fake proving
// rather than trusting the store alone.
type Repository struct {
	mu    sync.Mutex
	rows  map[string]clinical.EmergencyContact
	next  int
	clock time.Time
	calls []string
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]clinical.EmergencyContact), clock: epoch}
}

func (r *Repository) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}

func (r *Repository) List(_ context.Context, patientID string, query emergencycontact.Query) (domain.Page[clinical.EmergencyContact], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "list")

	matched := make([]clinical.EmergencyContact, 0, len(r.rows))

	for _, row := range r.rows {
		if row.PatientID == patientID {
			matched = append(matched, row)
		}
	}

	slices.SortFunc(matched, func(left, right clinical.EmergencyContact) int {
		return strings.Compare(right.ID, left.ID)
	})

	limit := query.Limit
	if limit <= 0 || limit > len(matched) {
		limit = len(matched)
	}

	return domain.NewPage(matched[:limit], nil), nil
}

func (r *Repository) Get(_ context.Context, id string) (clinical.EmergencyContact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "get")

	return r.byID(id)
}

func (r *Repository) Create(_ context.Context, entity clinical.EmergencyContact) (clinical.EmergencyContact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "create")
	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakecnt%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	if entity.IsPrimary {
		entity.DisplacedID = r.clearPrimary(entity.PatientID, entity.ID)
	}

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.EmergencyContact, expectedVersion string) (clinical.EmergencyContact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "update")

	current, err := r.byID(entity.ID)
	if err != nil {
		return clinical.EmergencyContact{}, err
	}

	if current.Version != expectedVersion {
		return clinical.EmergencyContact{}, domain.ErrVersionMismatch
	}

	r.clock = r.clock.Add(time.Millisecond)
	entity.PatientID = current.PatientID
	entity.CreatedAt = current.CreatedAt
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, revision(current.Version)+1)
	entity.DisplacedID = ""

	if entity.IsPrimary && !current.IsPrimary {
		entity.DisplacedID = r.clearPrimary(entity.PatientID, entity.ID)
	}

	r.rows[entity.ID] = entity

	return entity, nil
}

// clearPrimary unsets the previously primary contact of the same patient, if
// any other than newPrimaryID, and reports its id.
func (r *Repository) clearPrimary(patientID, newPrimaryID string) string {
	for id, row := range r.rows {
		if row.PatientID == patientID && row.IsPrimary && id != newPrimaryID {
			row.IsPrimary = false
			r.rows[id] = row

			return id
		}
	}

	return ""
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

func (r *Repository) byID(id string) (clinical.EmergencyContact, error) {
	row, exists := r.rows[id]
	if !exists {
		return clinical.EmergencyContact{}, fmt.Errorf("emergencycontacttest: no such record: %w", domain.ErrNotFound)
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
