package facilitytest

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
	"medikube/internal/service/facility"
)

// The two accounts the fakes answer for. They are this package's own
// identifiers and not a seeded fixture's: nothing here reaches a database.
const (
	OwnerID    = "mkfakeowner0002"
	StrangerID = "mkfakestrangr02"
)

// epoch is where the fake's clock starts. Every write advances it by a
// millisecond, so two rows never share an update time.
var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

var errInvalidCursor = errors.New("facilitytest: the boundary is not one this repository issued for this ordering")

// Repository is the in-memory facility.Repository: the second implementation
// Principle I requires, and the one the service's own tests run against.
type Repository struct {
	mu sync.Mutex

	rows  map[string]directory.Facility
	next  int
	clock time.Time

	// practitionerFacility and pharmacyFacility are what Usage counts against:
	// a facility id to however many rows this fake was told point at it.
	// facilitytest carries no practitioner or medication entity of its own —
	// that would import a sibling service package this one has no business
	// depending on — so a test wires the count in directly with SetUsage.
	practitionerFacility map[string]int
	pharmacyFacility     map[string]int

	calls     []string
	writes    []string
	lastOwner string
	lastQuery facility.Query
}

func NewRepository() *Repository {
	return &Repository{
		rows:                 make(map[string]directory.Facility),
		clock:                epoch,
		practitionerFacility: make(map[string]int),
		pharmacyFacility:     make(map[string]int),
	}
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

func (r *Repository) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls, r.writes = nil, nil
}

func (r *Repository) LastOwner() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastOwner
}

func (r *Repository) LastQuery() facility.Query {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastQuery
}

// SetUsage fixes what Usage(id) answers: how many practitioners and how many
// medications a test wants to say point at this facility.
func (r *Repository) SetUsage(id string, practitioners, records int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.practitionerFacility[id] = practitioners
	r.pharmacyFacility[id] = records
}

func (r *Repository) List(_ context.Context, ownerID string, query facility.Query) (domain.Page[directory.Facility], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "list")
	r.lastOwner, r.lastQuery = ownerID, query

	sortKeys := facility.Sorts()

	matched := make([]directory.Facility, 0, len(r.rows))

	for _, row := range r.rows {
		if row.OwnerID != ownerID || !matches(row, query) {
			continue
		}

		matched = append(matched, row)
	}

	slices.SortFunc(matched, func(left, right directory.Facility) int {
		return compareRows(left, right, sortKeys)
	})

	total := len(matched)

	if query.Cursor != "" {
		boundary, err := decodeBoundary(query.Cursor, sortKeys)
		if err != nil {
			return domain.Page[directory.Facility]{}, err
		}

		after := make([]directory.Facility, 0, len(matched))

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
			return domain.Page[directory.Facility]{}, err
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

func (r *Repository) Get(_ context.Context, ownerID, id string) (directory.Facility, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "get")
	r.lastOwner = ownerID

	return r.owned(ownerID, id)
}

func (r *Repository) Create(_ context.Context, entity directory.Facility) (directory.Facility, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "create")
	r.writes = append(r.writes, "create")

	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakefac%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity directory.Facility, expectedVersion string) (directory.Facility, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "update")

	current, err := r.owned(entity.OwnerID, entity.ID)
	if err != nil {
		return directory.Facility{}, err
	}

	if current.Version != expectedVersion {
		return directory.Facility{}, domain.ErrVersionMismatch
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

	return nil
}

// Owner answers who owns id, without regard to which account is asking — the
// service is what scopes the answer, exactly as the port documents.
func (r *Repository) Owner(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "owner")

	row, exists := r.rows[id]
	if !exists {
		return "", fmt.Errorf("facilitytest: no such record: %w", domain.ErrNotFound)
	}

	return row.OwnerID, nil
}

func (r *Repository) Usage(_ context.Context, ownerID, id string) (facility.Usage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "usage")

	if _, err := r.owned(ownerID, id); err != nil {
		return facility.Usage{}, err
	}

	return facility.Usage{Practitioners: r.practitionerFacility[id], Records: r.pharmacyFacility[id]}, nil
}

// owned is FR-037 in this repository: a row that is not there and a row that
// is somebody else's are the same refusal, with the same error value.
func (r *Repository) owned(ownerID, id string) (directory.Facility, error) {
	row, exists := r.rows[id]
	if !exists || row.OwnerID != ownerID {
		return directory.Facility{}, fmt.Errorf("facilitytest: no such record for this owner: %w", domain.ErrNotFound)
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

func matches(row directory.Facility, query facility.Query) bool {
	if query.Kind != "" && row.Kind != query.Kind {
		return false
	}

	if query.Search == "" {
		return true
	}

	needle := asciiLower(query.Search)

	return strings.Contains(asciiLower(row.Name), needle) ||
		strings.Contains(asciiLower(row.Brand), needle)
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

func sortValue(row directory.Facility, field string) string {
	switch field {
	case facility.FieldKind:
		return string(row.Kind)
	case facility.FieldName:
		return asciiLower(row.Name)
	default:
		return ""
	}
}

func compareValues(left, right string) int {
	return strings.Compare(left, right)
}

// compareRows is the ordering, tiebreaker included: kind, name, and the
// identity descending — matching every other index in this codebase.
func compareRows(left, right directory.Facility, sortKeys []domain.SortKey) int {
	for _, key := range sortKeys {
		if compared := compareValues(sortValue(left, key.Field), sortValue(right, key.Field)); compared != 0 {
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

func encodeBoundary(row directory.Facility, sortKeys []domain.SortKey) (string, error) {
	values := make([]string, 0, len(sortKeys))
	for _, key := range sortKeys {
		values = append(values, sortValue(row, key.Field))
	}

	payload, err := json.Marshal(boundary{Sort: sortKeys, Values: values, ID: row.ID})
	if err != nil {
		return "", fmt.Errorf("facilitytest: sealing the boundary: %w", err)
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

func compareToBoundary(row directory.Facility, after boundary, sortKeys []domain.SortKey) int {
	for i, key := range sortKeys {
		if compared := compareValues(sortValue(row, key.Field), after.Values[i]); compared != 0 {
			return compared
		}
	}

	return strings.Compare(after.ID, row.ID)
}

// Authorizer is the same decision defaultAuthorizer makes — authenticated and
// not a superuser grants own, everything else grants nothing — so that the
// service's tests exercise the real shape of the checkpoint and not a stand-in
// with different rules. There is no owner here to compare against: FR-037's
// ownership question is Repository.Owner's, not this interface's.
type Authorizer struct {
	mu sync.Mutex

	level access.Permission
	err   error
	calls int
}

func NewAuthorizer() *Authorizer {
	return &Authorizer{level: access.PermOwn}
}

// Refuse makes every answer this error, simulating a checkpoint that could not
// answer at all.
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

func (a *Authorizer) Actor(_ context.Context, actor access.Actor) (access.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.calls++

	if a.err != nil {
		return access.Grant{}, a.err
	}

	if !actor.Authenticated() || actor.IsSuperuser {
		return access.Grant{}, nil
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
