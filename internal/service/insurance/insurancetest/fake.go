// Package insurancetest is the in-memory second implementation of
// insurance.Repository (Principle I).
package insurancetest

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
	"medikube/internal/service/insurance"
)

const (
	OwnerID    = "mkfakeowner0003"
	StrangerID = "mkfakestrangr03"

	PatientID         = "mkfakepatient03"
	StrangerPatientID = "mkfakestrangp3"
)

var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

var errInvalidCursor = errors.New("insurancetest: the boundary is not one this repository issued for this ordering")

const defaultLimit = 25

// Repository is the in-memory insurance.Repository.
type Repository struct {
	mu sync.Mutex

	rows  map[string]clinical.Insurance
	next  int
	clock time.Time
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]clinical.Insurance), clock: epoch}
}

// DeletePatient removes every row filed against patientID.
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

func (r *Repository) List(_ context.Context, patientID string, query insurance.Query) (domain.Page[clinical.Insurance], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = insurance.Sorts()[:1]
	}

	matched := make([]clinical.Insurance, 0, len(r.rows))

	for _, row := range r.rows {
		if row.PatientID != patientID || !matches(row, query) {
			continue
		}

		matched = append(matched, row)
	}

	slices.SortFunc(matched, func(left, right clinical.Insurance) int {
		return compareRows(left, right, sortKeys)
	})

	total := len(matched)

	if query.Cursor != "" {
		boundary, err := decodeBoundary(query.Cursor, sortKeys)
		if err != nil {
			return domain.Page[clinical.Insurance]{}, err
		}

		after := make([]clinical.Insurance, 0, len(matched))

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
			return domain.Page[clinical.Insurance]{}, err
		}

		next = &token
	}

	page := domain.NewPage(matched, next)
	if query.Count {
		page = page.WithTotal(total)
	}

	return page, nil
}

func (r *Repository) Get(_ context.Context, id string) (clinical.Insurance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.byID(id)
}

func (r *Repository) Create(_ context.Context, entity clinical.Insurance) (clinical.Insurance, *insurance.Displaced, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakeins%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	displaced := r.displace(entity)

	r.rows[entity.ID] = entity

	return entity, displaced, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.Insurance, expectedVersion string) (clinical.Insurance, *insurance.Displaced, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, err := r.byID(entity.ID)
	if err != nil {
		return clinical.Insurance{}, nil, err
	}

	if current.Version != expectedVersion {
		return clinical.Insurance{}, nil, domain.ErrVersionMismatch
	}

	r.clock = r.clock.Add(time.Millisecond)

	entity.PatientID = current.PatientID
	entity.CreatedAt = current.CreatedAt
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, revision(current.Version)+1)

	displaced := r.displace(entity)

	r.rows[entity.ID] = entity

	return entity, displaced, nil
}

// displace unsets is_primary on every other policy of this patient when
// entity is primary, and reports the one it displaced (FR-045). It is called
// with the lock already held: the whole point is that this and the write of
// entity itself are one atomic step, exactly as internal/store/insurance does
// inside a single transaction.
func (r *Repository) displace(entity clinical.Insurance) *insurance.Displaced {
	if !entity.IsPrimary {
		return nil
	}

	var displaced *insurance.Displaced

	for id, row := range r.rows {
		if id == entity.ID || row.PatientID != entity.PatientID || !row.IsPrimary {
			continue
		}

		row.IsPrimary = false
		r.rows[id] = row
		displaced = &insurance.Displaced{ID: id}
	}

	return displaced
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

func (r *Repository) byID(id string) (clinical.Insurance, error) {
	row, exists := r.rows[id]
	if !exists {
		return clinical.Insurance{}, fmt.Errorf("insurancetest: no such record: %w", domain.ErrNotFound)
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

func matches(row clinical.Insurance, query insurance.Query) bool {
	if len(query.Types) > 0 && !slices.Contains(query.Types, row.Type) {
		return false
	}

	if len(query.Statuses) > 0 && !slices.Contains(query.Statuses, row.Status) {
		return false
	}

	if query.IsPrimary != nil && row.IsPrimary != *query.IsPrimary {
		return false
	}

	if query.ExpiringWithinDays != nil && len(insurance.ExpiringBasis(row, *query.ExpiringWithinDays)) == 0 {
		return false
	}

	if query.Search == "" {
		return true
	}

	return strings.Contains(asciiLower(row.Company), asciiLower(query.Search))
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

func sortValue(row clinical.Insurance, field string) string {
	switch field {
	case insurance.FieldCompany:
		return asciiLower(row.Company)
	case insurance.FieldEffectiveOn:
		return row.EffectiveOn.String()
	case insurance.FieldUpdated:
		return row.UpdatedAt.UTC().Format("2006-01-02 15:04:05.000000000")
	default:
		return ""
	}
}

func compareValues(field string, left, right string, desc bool) int {
	if field == insurance.FieldEffectiveOn && (left == "" || right == "") {
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

func compareRows(left, right clinical.Insurance, sortKeys []domain.SortKey) int {
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

func encodeBoundary(row clinical.Insurance, sortKeys []domain.SortKey) (string, error) {
	values := make([]string, 0, len(sortKeys))
	for _, key := range sortKeys {
		values = append(values, sortValue(row, key.Field))
	}

	payload, err := json.Marshal(boundary{Sort: sortKeys, Values: values, ID: row.ID})
	if err != nil {
		return "", fmt.Errorf("insurancetest: sealing the boundary: %w", err)
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

func compareToBoundary(row clinical.Insurance, after boundary, sortKeys []domain.SortKey) int {
	for i, key := range sortKeys {
		if compared := compareValues(key.Field, sortValue(row, key.Field), after.Values[i], key.Desc); compared != 0 {
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

// The stand-in DTOs, for a kind contract run without internal/web/api.
type (
	Summary struct {
		ID   string
		Kind string
		Name string
	}

	Detail struct {
		Summary

		Displaced *insurance.Displaced
	}

	Create struct {
		Patient     string
		Type        string
		Company     string
		MemberName  string
		MemberID    string
		EffectiveOn string
	}

	Patch struct {
		Company *string
	}
)

type Codec struct{}

func NewCodec() Codec { return Codec{} }

func (Codec) Summary(entity clinical.Insurance) any {
	return &Summary{ID: entity.ID, Kind: kind.Insurance.Enum(), Name: entity.Company}
}

func (Codec) Detail(entity clinical.Insurance, displaced *insurance.Displaced) any {
	return &Detail{
		Summary:   Summary{ID: entity.ID, Kind: kind.Insurance.Enum(), Name: entity.Company},
		Displaced: displaced,
	}
}

func (Codec) Draft(body any) (clinical.Insurance, error) {
	create, minted := body.(*Create)
	if !minted {
		return clinical.Insurance{}, fmt.Errorf("insurancetest: a create was handed %T", body)
	}

	effectiveOn, _ := domain.ParseDate(create.EffectiveOn)

	return clinical.Insurance{
		PatientID:   create.Patient,
		Type:        clinical.InsuranceType(create.Type),
		Company:     create.Company,
		MemberName:  create.MemberName,
		MemberID:    create.MemberID,
		EffectiveOn: effectiveOn,
	}, nil
}

func (Codec) Patch(body any) (insurance.Patch, error) {
	patch, minted := body.(*Patch)
	if !minted {
		return insurance.Patch{}, fmt.Errorf("insurancetest: a patch was handed %T", body)
	}

	return insurance.Patch{Company: patch.Company}, nil
}

func Shapes() records.Schema {
	return records.Schema{
		NewSummary: func() any { return &Summary{} },
		NewDetail:  func() any { return &Detail{} },
		NewCreate:  func() any { return &Create{} },
		NewPatch:   func() any { return &Patch{} },
	}
}
