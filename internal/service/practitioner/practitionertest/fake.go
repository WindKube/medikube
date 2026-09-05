package practitionertest

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
	"medikube/internal/domain/audit"
	"medikube/internal/domain/directory"
	"medikube/internal/service/practitioner"
)

// The two accounts the fakes answer for. Own identifiers, not the seeded
// fixture's: nothing here reaches a database.
const (
	OwnerID    = "mkfakeowner0001"
	StrangerID = "mkfakestrangr01"
)

// epoch is where the fake's clock starts, exactly as medicationtest's is.
var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

const defaultLimit = 25

// Repository is the in-memory practitioner.Repository.
type Repository struct {
	mu sync.Mutex

	rows  map[string]directory.Practitioner
	next  int
	clock time.Time

	// facilities maps a facility id to the owner it belongs to, for Create and
	// Update's FR-042 check. AllowFacility populates it.
	facilities map[string]string

	usage map[string]practitioner.Usage

	calls     []string
	writes    []string
	lastOwner string
}

func NewRepository() *Repository {
	return &Repository{
		rows:       make(map[string]directory.Practitioner),
		clock:      epoch,
		facilities: make(map[string]string),
		usage:      make(map[string]practitioner.Usage),
	}
}

// Calls is every method reached since the last Forget, in order.
func (r *Repository) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}

// Writes is the subset that changed something.
func (r *Repository) Writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.writes)
}

// Forget clears the journal and nothing else.
func (r *Repository) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls, r.writes = nil, nil
}

// LastOwner is the account the last owner-scoped call was made for.
func (r *Repository) LastOwner() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastOwner
}

// AllowFacility registers a facility as belonging to ownerID, so Create and
// Update accept it. Anything not registered is refused as domain.ErrNotFound,
// mirroring the store's own lookup against the facilities collection.
func (r *Repository) AllowFacility(ownerID, facilityID string) *Repository {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.facilities[facilityID] = ownerID

	return r
}

// SetUsage fixes what Usage answers for one id, for the tests that assert the
// service or an edge reads it correctly. Every id not set here answers zero.
func (r *Repository) SetUsage(id string, usage practitioner.Usage) *Repository {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.usage[id] = usage

	return r
}

func (r *Repository) List(_ context.Context, ownerID string, query practitioner.Query) (domain.Page[directory.Practitioner], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "list")
	r.lastOwner = ownerID

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = practitioner.Sorts()
	}

	matched := make([]directory.Practitioner, 0, len(r.rows))

	for _, row := range r.rows {
		if row.OwnerID != ownerID || !matches(row, query) {
			continue
		}

		matched = append(matched, row)
	}

	slices.SortFunc(matched, func(left, right directory.Practitioner) int {
		return compareRows(left, right, sortKeys)
	})

	total := len(matched)

	if query.Cursor != "" {
		boundary, err := decodeBoundary(query.Cursor, sortKeys)
		if err != nil {
			return domain.Page[directory.Practitioner]{}, err
		}

		after := make([]directory.Practitioner, 0, len(matched))

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
			return domain.Page[directory.Practitioner]{}, err
		}

		next = &token
	}

	page := domain.NewPage(matched, next)
	if query.Count {
		page = page.WithTotal(total)
	}

	return page, nil
}

func (r *Repository) Get(_ context.Context, ownerID, id string) (directory.Practitioner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "get")
	r.lastOwner = ownerID

	return r.owned(ownerID, id)
}

func (r *Repository) Owner(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "owner")

	row, exists := r.rows[id]
	if !exists {
		return "", fmt.Errorf("practitionertest: no such record: %w", domain.ErrNotFound)
	}

	return row.OwnerID, nil
}

func (r *Repository) Usage(_ context.Context, ownerID, id string) (practitioner.Usage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "usage")
	r.lastOwner = ownerID

	if _, err := r.owned(ownerID, id); err != nil {
		return practitioner.Usage{}, err
	}

	return r.usage[id], nil
}

func (r *Repository) Create(_ context.Context, entity directory.Practitioner) (directory.Practitioner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "create")

	if err := r.checkFacility(entity.OwnerID, entity.FacilityID); err != nil {
		return directory.Practitioner{}, err
	}

	if err := r.checkUnique(entity.OwnerID, "", entity.Name, entity.Specialty); err != nil {
		return directory.Practitioner{}, err
	}

	r.writes = append(r.writes, "create")
	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakeprc%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity directory.Practitioner, expectedVersion string) (directory.Practitioner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "update")

	current, err := r.owned(entity.OwnerID, entity.ID)
	if err != nil {
		return directory.Practitioner{}, err
	}

	if current.Version != expectedVersion {
		return directory.Practitioner{}, domain.ErrVersionMismatch
	}

	if err := r.checkFacility(entity.OwnerID, entity.FacilityID); err != nil {
		return directory.Practitioner{}, err
	}

	if err := r.checkUnique(entity.OwnerID, entity.ID, entity.Name, entity.Specialty); err != nil {
		return directory.Practitioner{}, err
	}

	r.writes = append(r.writes, "update")
	r.clock = r.clock.Add(time.Millisecond)

	entity.OwnerID = current.OwnerID
	entity.CreatedAt = current.CreatedAt
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, revision(current.Version)+1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Delete(_ context.Context, ownerID, id, expectedVersion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "delete")

	current, err := r.owned(ownerID, id)
	if err != nil {
		return err
	}

	if current.Version != expectedVersion {
		return domain.ErrVersionMismatch
	}

	r.writes = append(r.writes, "delete")
	delete(r.rows, id)
	delete(r.usage, id)

	return nil
}

// checkFacility is FR-042: a facility named on the draft that is not this
// owner's is refused exactly as one that does not exist.
func (r *Repository) checkFacility(ownerID, facilityID string) error {
	if facilityID == "" {
		return nil
	}

	if owner, declared := r.facilities[facilityID]; declared && owner == ownerID {
		return nil
	}

	return fmt.Errorf("practitionertest: no such facility for this owner: %w", domain.ErrNotFound)
}

// checkUnique is FR-038: the same (owner, LOWER(name), specialty) cannot be
// recorded twice. selfID excludes the row being updated from its own check.
func (r *Repository) checkUnique(ownerID, selfID, name string, specialty directory.Specialty) error {
	folded := asciiLower(name)

	for id, row := range r.rows {
		if id == selfID || row.OwnerID != ownerID {
			continue
		}

		if asciiLower(row.Name) == folded && row.Specialty == specialty {
			return fmt.Errorf(
				"practitionertest: %s already has a practitioner named %q under this specialty: %w",
				ownerID, name, domain.ErrConflict)
		}
	}

	return nil
}

// owned is FR-037: a row that is not there and a row that is somebody else's
// are the same refusal, with the same error value.
func (r *Repository) owned(ownerID, id string) (directory.Practitioner, error) {
	row, exists := r.rows[id]
	if !exists || row.OwnerID != ownerID {
		return directory.Practitioner{}, fmt.Errorf("practitionertest: no such record for this owner: %w", domain.ErrNotFound)
	}

	return row, nil
}

func version(id string, revision int) string {
	return id + "-" + strconv.Itoa(revision)
}

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

func matches(row directory.Practitioner, query practitioner.Query) bool {
	if query.Specialty != "" && row.Specialty != query.Specialty {
		return false
	}

	if query.FacilityID != "" && row.FacilityID != query.FacilityID {
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

func sortValue(row directory.Practitioner, field string) string {
	if field == practitioner.FieldName {
		return asciiLower(row.Name)
	}

	return ""
}

func compareRows(left, right directory.Practitioner, sortKeys []domain.SortKey) int {
	for _, key := range sortKeys {
		compared := strings.Compare(sortValue(left, key.Field), sortValue(right, key.Field))
		if key.Desc {
			compared = -compared
		}

		if compared != 0 {
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

var errInvalidCursor = errors.New("practitionertest: the boundary is not one this repository issued for this ordering")

func encodeBoundary(row directory.Practitioner, sortKeys []domain.SortKey) (string, error) {
	values := make([]string, 0, len(sortKeys))
	for _, key := range sortKeys {
		values = append(values, sortValue(row, key.Field))
	}

	payload, err := json.Marshal(boundary{Sort: sortKeys, Values: values, ID: row.ID})
	if err != nil {
		return "", fmt.Errorf("practitionertest: sealing the boundary: %w", err)
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

func compareToBoundary(row directory.Practitioner, after boundary, sortKeys []domain.SortKey) int {
	for i, key := range sortKeys {
		compared := strings.Compare(sortValue(row, key.Field), after.Values[i])
		if key.Desc {
			compared = -compared
		}

		if compared != 0 {
			return compared
		}
	}

	return strings.Compare(after.ID, row.ID)
}

// Authorizer is a checkpoint that answers for one account, mirroring
// medicationtest's.
type Authorizer struct {
	mu sync.Mutex

	owner string
	level access.Permission
	err   error
	calls int
}

func NewAuthorizer(owner string) *Authorizer {
	return &Authorizer{owner: owner, level: access.PermOwn}
}

// Refuse makes every answer this error.
func (a *Authorizer) Refuse(err error) *Authorizer {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.err = err

	return a
}

// Grant fixes the level the checkpoint resolves to.
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

func (a *Authorizer) Actor(_ context.Context, actor access.Actor) (access.Grant, error) {
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

// Auditor collects the rows the service writes.
type Auditor struct {
	mu sync.Mutex

	events []audit.Event
	err    error
}

func NewAuditor() *Auditor { return &Auditor{} }

func (a *Auditor) Fail(err error) *Auditor {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.err = err

	return a
}

func (a *Auditor) Events() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return slices.Clone(a.events)
}

func (a *Auditor) Record(_ context.Context, event audit.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.err != nil {
		return a.err
	}

	a.events = append(a.events, event)

	return nil
}
