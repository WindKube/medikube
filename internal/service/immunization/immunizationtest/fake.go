// Package immunizationtest is the in-memory immunization.Repository and
// Authorizer, mirroring internal/service/medication/medicationtest's shape
// exactly: the same cursor encoding, the same ordering rules, the same
// coarse per-account Authorizer.
package immunizationtest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/immunization"
)

const (
	OwnerID    = "mkfakeowner0002"
	StrangerID = "mkfakestrangr02"

	PatientID         = "mkfakepatient02"
	StrangerPatientID = "mkfakestrangp2"
)

var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

var errInvalidCursor = errors.New("immunizationtest: the boundary is not one this repository issued for this ordering")

type Repository struct {
	mu    sync.Mutex
	rows  map[string]clinical.Immunization
	next  int
	clock time.Time
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]clinical.Immunization), clock: epoch}
}

func (r *Repository) List(_ context.Context, patientID string, query immunization.Query) (domain.Page[clinical.Immunization], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = immunization.Sorts()[:1]
	}

	matched := make([]clinical.Immunization, 0, len(r.rows))
	for _, row := range r.rows {
		if row.PatientID != patientID || !matches(row, query) {
			continue
		}
		matched = append(matched, row)
	}

	slices.SortFunc(matched, func(left, right clinical.Immunization) int {
		return compareRows(left, right, sortKeys)
	})

	total := len(matched)

	if query.Cursor != "" {
		boundary, err := decodeBoundary(query.Cursor, sortKeys)
		if err != nil {
			return domain.Page[clinical.Immunization]{}, err
		}

		after := make([]clinical.Immunization, 0, len(matched))
		for _, row := range matched {
			if compareToBoundary(row, boundary, sortKeys) > 0 {
				after = append(after, row)
			}
		}
		matched = after
	}

	limit := query.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	var next *string

	if len(matched) > limit {
		matched = matched[:limit]

		token, err := encodeBoundary(matched[len(matched)-1], sortKeys)
		if err != nil {
			return domain.Page[clinical.Immunization]{}, err
		}
		next = &token
	}

	page := domain.NewPage(matched, next)
	if query.Count {
		page = page.WithTotal(total)
	}

	return page, nil
}

const defaultLimit = 25

func (r *Repository) Get(_ context.Context, id string) (clinical.Immunization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.byID(id)
}

func (r *Repository) Create(_ context.Context, entity clinical.Immunization) (clinical.Immunization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakeimm%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.Immunization, expectedVersion string) (clinical.Immunization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, err := r.byID(entity.ID)
	if err != nil {
		return clinical.Immunization{}, err
	}

	if current.Version != expectedVersion {
		return clinical.Immunization{}, domain.ErrVersionMismatch
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

func (r *Repository) byID(id string) (clinical.Immunization, error) {
	row, exists := r.rows[id]
	if !exists {
		return clinical.Immunization{}, fmt.Errorf("immunizationtest: no such record: %w", domain.ErrNotFound)
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

func matches(row clinical.Immunization, query immunization.Query) bool {
	if query.Search == "" {
		return true
	}

	needle := asciiLower(query.Search)

	return strings.Contains(asciiLower(row.VaccineName), needle) ||
		strings.Contains(asciiLower(row.TradeName), needle)
}

func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

func sortValue(row clinical.Immunization, field string) string {
	switch field {
	case immunization.FieldVaccineName:
		return asciiLower(row.VaccineName)
	case immunization.FieldAdministeredOn:
		return row.AdministeredOn.String()
	case immunization.FieldUpdated:
		return row.UpdatedAt.UTC().Format("2006-01-02 15:04:05.000000000")
	default:
		return ""
	}
}

func compareValues(field string, left, right string, desc bool) int {
	if field == immunization.FieldAdministeredOn && (left == "" || right == "") {
		switch {
		case left == "" && right == "":
			return 0
		case left == "":
			return 1
		default:
			return -1
		}
	}

	compared := strings.Compare(left, right)
	if desc {
		return -compared
	}

	return compared
}

func compareRows(left, right clinical.Immunization, sortKeys []domain.SortKey) int {
	for _, key := range sortKeys {
		if compared := compareValues(key.Field, sortValue(left, key.Field), sortValue(right, key.Field), key.Desc); compared != 0 {
			return compared
		}
	}

	return strings.Compare(right.ID, left.ID)
}

type boundary struct {
	Sort   []domain.SortKey `json:"sort"`
	Values []string         `json:"values"`
	ID     string           `json:"id"`
}

func encodeBoundary(row clinical.Immunization, sortKeys []domain.SortKey) (string, error) {
	values := make([]string, 0, len(sortKeys))
	for _, key := range sortKeys {
		values = append(values, sortValue(row, key.Field))
	}

	payload, err := json.Marshal(boundary{Sort: sortKeys, Values: values, ID: row.ID})
	if err != nil {
		return "", fmt.Errorf("immunizationtest: sealing the boundary: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeBoundary(token string, sortKeys []domain.SortKey) (boundary, error) {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return boundary{}, errInvalidCursor
	}

	var decoded boundary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return boundary{}, errInvalidCursor
	}

	if !slices.Equal(decoded.Sort, sortKeys) || len(decoded.Values) != len(sortKeys) || decoded.ID == "" {
		return boundary{}, errInvalidCursor
	}

	return decoded, nil
}

func compareToBoundary(row clinical.Immunization, after boundary, sortKeys []domain.SortKey) int {
	for i, key := range sortKeys {
		if compared := compareValues(key.Field, sortValue(row, key.Field), after.Values[i], key.Desc); compared != 0 {
			return compared
		}
	}

	return strings.Compare(after.ID, row.ID)
}

// Authorizer is a checkpoint that answers for one account, mirroring
// medicationtest.Authorizer.
type Authorizer struct {
	mu    sync.Mutex
	owner string
	level access.Permission
	err   error
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

func (a *Authorizer) Patient(_ context.Context, actor access.Actor, _ string, _ access.Permission) (access.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.err != nil {
		return access.Grant{}, a.err
	}

	if actor.UserID != a.owner {
		return access.Grant{}, domain.ErrNotFound
	}

	return access.Grant{Level: a.level}, nil
}
