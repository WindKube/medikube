// Package familymembertest is the in-memory second implementation of
// familymember.Repository (Principle I), used by the service's own unit tests
// and by the kind-registration contract.
package familymembertest

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
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/familymember"
)

const (
	OwnerID    = "mkfakeowner0003"
	StrangerID = "mkfakestrangr03"

	PatientID         = "mkfakepatient03"
	StrangerPatientID = "mkfakestrangp3"
)

var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

var errInvalidCursor = errors.New("familymembertest: the boundary is not one this repository issued for this ordering")

const defaultLimit = 25

// Repository is the in-memory familymember.Repository.
type Repository struct {
	mu sync.Mutex

	rows  map[string]clinical.FamilyMember
	next  int
	clock time.Time
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]clinical.FamilyMember), clock: epoch}
}

// DeletePatient removes every row filed against patientID, the fake's own
// equivalent of the cascade a real patient delete performs.
func (r *Repository) DeletePatient(_ context.Context, patientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, row := range r.rows {
		if row.PatientID == patientID {
			delete(r.rows, id)
		}
	}

	return nil
}

func (r *Repository) List(_ context.Context, patientID string, query familymember.Query) (domain.Page[clinical.FamilyMember], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = familymember.Sorts()[:1]
	}

	matched := make([]clinical.FamilyMember, 0, len(r.rows))

	for _, row := range r.rows {
		if row.PatientID != patientID || !matches(row, query) {
			continue
		}

		matched = append(matched, row)
	}

	slices.SortFunc(matched, func(left, right clinical.FamilyMember) int {
		return compareRows(left, right, sortKeys)
	})

	total := len(matched)

	if query.Cursor != "" {
		boundary, err := decodeBoundary(query.Cursor, sortKeys)
		if err != nil {
			return domain.Page[clinical.FamilyMember]{}, err
		}

		after := make([]clinical.FamilyMember, 0, len(matched))

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
			return domain.Page[clinical.FamilyMember]{}, err
		}

		next = &token
	}

	page := domain.NewPage(matched, next)
	if query.Count {
		page = page.WithTotal(total)
	}

	return page, nil
}

func (r *Repository) Get(_ context.Context, id string) (clinical.FamilyMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.byID(id)
}

func (r *Repository) Create(_ context.Context, entity clinical.FamilyMember) (clinical.FamilyMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakefam%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.FamilyMember, expectedVersion string) (clinical.FamilyMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, err := r.byID(entity.ID)
	if err != nil {
		return clinical.FamilyMember{}, err
	}

	if current.Version != expectedVersion {
		return clinical.FamilyMember{}, domain.ErrVersionMismatch
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

func (r *Repository) byID(id string) (clinical.FamilyMember, error) {
	row, exists := r.rows[id]
	if !exists {
		return clinical.FamilyMember{}, fmt.Errorf("familymembertest: no such record: %w", domain.ErrNotFound)
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

func matches(row clinical.FamilyMember, query familymember.Query) bool {
	if len(query.Relationships) > 0 && !slices.Contains(query.Relationships, row.Relationship) {
		return false
	}

	if query.Search == "" {
		return true
	}

	return strings.Contains(asciiLower(row.Name), asciiLower(query.Search))
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

func sortValue(row clinical.FamilyMember, field string) string {
	switch field {
	case familymember.FieldRelationship:
		return string(row.Relationship)
	case familymember.FieldName:
		return asciiLower(row.Name)
	case familymember.FieldUpdated:
		return row.UpdatedAt.UTC().Format("2006-01-02 15:04:05.000000000")
	default:
		return ""
	}
}

func compareValues(left, right string, desc bool) int {
	compared := strings.Compare(left, right)
	if desc {
		return -compared
	}

	return compared
}

func compareRows(left, right clinical.FamilyMember, sortKeys []domain.SortKey) int {
	for _, key := range sortKeys {
		if compared := compareValues(sortValue(left, key.Field), sortValue(right, key.Field), key.Desc); compared != 0 {
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

func encodeBoundary(row clinical.FamilyMember, sortKeys []domain.SortKey) (string, error) {
	values := make([]string, 0, len(sortKeys))
	for _, key := range sortKeys {
		values = append(values, sortValue(row, key.Field))
	}

	payload, err := json.Marshal(boundary{Sort: sortKeys, Values: values, ID: row.ID})
	if err != nil {
		return "", fmt.Errorf("familymembertest: sealing the boundary: %w", err)
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

func compareToBoundary(row clinical.FamilyMember, after boundary, sortKeys []domain.SortKey) int {
	for i, key := range sortKeys {
		if compared := compareValues(sortValue(row, key.Field), after.Values[i], key.Desc); compared != 0 {
			return compared
		}
	}

	return strings.Compare(after.ID, row.ID)
}

// Authorizer is a checkpoint that answers for one account.
type Authorizer struct {
	mu sync.Mutex

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

// The stand-in DTOs, used only by this test's own Codec so a kind contract
// can be run without depending on internal/web/api.
type (
	Summary struct {
		ID   string
		Kind string
		Name string
	}

	Detail struct {
		Summary

		Relationship string
	}

	Create struct {
		Patient      string
		Name         string
		Relationship string
	}

	Patch struct {
		Name *string
	}
)

type Codec struct{}

func NewCodec() Codec { return Codec{} }

func (Codec) Summary(entity clinical.FamilyMember) any {
	return &Summary{ID: entity.ID, Kind: kind.FamilyMember.Enum(), Name: entity.Name}
}

func (Codec) Detail(entity clinical.FamilyMember) any {
	return &Detail{
		Summary:      Summary{ID: entity.ID, Kind: kind.FamilyMember.Enum(), Name: entity.Name},
		Relationship: string(entity.Relationship),
	}
}

func (Codec) Draft(body any) (clinical.FamilyMember, error) {
	create, minted := body.(*Create)
	if !minted {
		return clinical.FamilyMember{}, fmt.Errorf("familymembertest: a create was handed %T", body)
	}

	return clinical.FamilyMember{
		PatientID:    create.Patient,
		Name:         create.Name,
		Relationship: clinical.FamilyRelationship(create.Relationship),
	}, nil
}

func (Codec) Patch(body any) (familymember.Patch, error) {
	patch, minted := body.(*Patch)
	if !minted {
		return familymember.Patch{}, fmt.Errorf("familymembertest: a patch was handed %T", body)
	}

	return familymember.Patch{Name: patch.Name}, nil
}

func Shapes() records.Schema {
	return records.Schema{
		NewSummary: func() any { return &Summary{} },
		NewDetail:  func() any { return &Detail{} },
		NewCreate:  func() any { return &Create{} },
		NewPatch:   func() any { return &Patch{} },
	}
}
