package medicationtest

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
	"medikube/internal/service/medication"
)

// The two accounts the fakes answer for. They are this package's own
// identifiers and not the seeded fixture's: nothing here reaches a database,
// and a fake that borrowed a real id would read as though it did.
const (
	OwnerID    = "mkfakeowner0001"
	StrangerID = "mkfakestrangr01"

	// PatientID and StrangerPatientID are phase 002's addition: the anchor a
	// medication is now scoped and authorized by is a patient, not the
	// account directly (research D-13). The fake Authorizer still answers
	// from the actor's account alone — it does not model a patient-to-account
	// mapping — because that mapping is access.Authorizer.Patient's own job
	// and is covered by internal/service/access's tests.
	PatientID         = "mkfakepatient01"
	StrangerPatientID = "mkfakestrangp1"
)

// epoch is where the fake's clock starts. Every write advances it by a
// millisecond, so two rows never share an update time and the ordering by last
// change is deterministic — which is the one thing a real database cannot
// promise and the reason the contract asserts that ordering as a property
// rather than as a sequence.
var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

var errInvalidCursor = errors.New("medicationtest: the boundary is not one this repository issued for this ordering")

// Repository is the in-memory medication.Repository: the second implementation
// Principle I requires, and the one the service's own tests run against.
//
// It enforces owner scoping, the version check and the ordering rules exactly
// as the PocketBase repository must, because both are held to
// RunRepositoryContract. A fake that was more forgiving than the store would
// make every service test above it worthless.
type Repository struct {
	mu sync.Mutex

	rows  map[string]clinical.Medication
	next  int
	clock time.Time

	calls       []string
	writes      []string
	lastPatient string
	lastQuery   medication.Query
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]clinical.Medication), clock: epoch}
}

// Calls is every method reached since the last Forget, in order. It is what the
// service's tests assert emptiness of: a method that reached the store after
// the checkpoint refused has an entry here.
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

// Forget clears the journal and nothing else, so a test can seed through the
// repository and still assert that the call under test touched it.
func (r *Repository) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls, r.writes = nil, nil
}

// LastPatient is the patient the last patient-scoped call was made for.
func (r *Repository) LastPatient() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastPatient
}

// LastQuery is the query the last list was asked, after the service resolved
// it.
func (r *Repository) LastQuery() medication.Query {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastQuery
}

func (r *Repository) List(_ context.Context, patientID string, query medication.Query) (domain.Page[clinical.Medication], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "list")
	r.lastPatient, r.lastQuery = patientID, query

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = medication.Sorts()[:1]
	}

	matched := make([]clinical.Medication, 0, len(r.rows))

	for _, row := range r.rows {
		if row.PatientID != patientID || !matches(row, query) {
			continue
		}

		matched = append(matched, row)
	}

	slices.SortFunc(matched, func(left, right clinical.Medication) int {
		return compareRows(left, right, sortKeys)
	})

	total := len(matched)

	if query.Cursor != "" {
		boundary, err := decodeBoundary(query.Cursor, sortKeys)
		if err != nil {
			return domain.Page[clinical.Medication]{}, err
		}

		after := make([]clinical.Medication, 0, len(matched))

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
			return domain.Page[clinical.Medication]{}, err
		}

		next = &token
	}

	page := domain.NewPage(matched, next)
	if query.Count {
		page = page.WithTotal(total)
	}

	return page, nil
}

// defaultLimit is this repository's own published default. The bounds a request
// is held to are the edge's (internal/web's MinLimit, MaxLimit, DefaultLimit);
// this is only what an unstated limit means here.
const defaultLimit = 25

func (r *Repository) Get(_ context.Context, id string) (clinical.Medication, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "get")

	return r.byID(id)
}

func (r *Repository) Create(_ context.Context, entity clinical.Medication) (clinical.Medication, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "create")
	r.writes = append(r.writes, "create")

	r.next++
	r.clock = r.clock.Add(time.Millisecond)

	entity.ID = fmt.Sprintf("mkfakemed%06d", r.next)
	entity.CreatedAt = r.clock
	entity.UpdatedAt = r.clock
	entity.Version = version(entity.ID, 1)

	r.rows[entity.ID] = entity

	return entity, nil
}

func (r *Repository) Update(_ context.Context, entity clinical.Medication, expectedVersion string) (clinical.Medication, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "update")

	current, err := r.byID(entity.ID)
	if err != nil {
		return clinical.Medication{}, err
	}

	if current.Version != expectedVersion {
		return clinical.Medication{}, domain.ErrVersionMismatch
	}

	r.writes = append(r.writes, "update")
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

	r.writes = append(r.writes, "delete")
	delete(r.rows, id)

	return nil
}

// byID is FR-033 in this repository for a row that is not there: unscoped, as
// internal/store/medication's own Get/Update/Delete are (research D-13) — the
// service resolves and authorizes the patient from the row itself.
func (r *Repository) byID(id string) (clinical.Medication, error) {
	row, exists := r.rows[id]
	if !exists {
		return clinical.Medication{}, fmt.Errorf("medicationtest: no such record: %w", domain.ErrNotFound)
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

func matches(row clinical.Medication, query medication.Query) bool {
	if len(query.Statuses) > 0 && !slices.Contains(query.Statuses, row.Status) {
		return false
	}

	if query.Search == "" {
		return true
	}

	// ASCII folding and not strings.ToLower, because SQLite's LIKE folds ASCII
	// and nothing else — a fake that folded the whole of Unicode would match
	// rows the store does not.
	needle := asciiLower(query.Search)

	return strings.Contains(asciiLower(row.Name), needle) ||
		strings.Contains(asciiLower(row.AlternativeName), needle)
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

// sortValue is the one string a row is compared by for a sort field. It is the
// same value the boundary carries, because a comparison and a cursor that read
// the row differently page incorrectly and say nothing about it.
func sortValue(row clinical.Medication, field string) string {
	switch field {
	case medication.FieldName:
		return asciiLower(row.Name)
	case medication.FieldStartedOn:
		return row.StartedOn.String()
	case medication.FieldUpdated:
		return row.UpdatedAt.UTC().Format("2006-01-02 15:04:05.000000000")
	default:
		return ""
	}
}

// compareValues orders two values of one column, and it is where the absent
// date is decided.
//
// contracts/records.md fixes it: a row with no start date sorts last under both
// directions. The empty string would sort first ascending and last descending
// if it were left to the comparison, so absence is separated out before the
// direction is applied at all.
func compareValues(field string, left, right string, desc bool) int {
	if field == medication.FieldStartedOn && (left == "" || right == "") {
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

// compareRows is the ordering, tiebreaker included. The identity is always last
// and always descending, matching the trailing `id DESC` on every index in
// data-model §2.
func compareRows(left, right clinical.Medication, sortKeys []domain.SortKey) int {
	for _, key := range sortKeys {
		if compared := compareValues(key.Field, sortValue(left, key.Field), sortValue(right, key.Field), key.Desc); compared != 0 {
			return compared
		}
	}

	return strings.Compare(right.ID, left.ID)
}

// boundary is the fake's cursor payload: the ordering it belongs to, one value
// per term and the identity. It is not encrypted — internal/store's is, and has
// to be, because a boundary value there is a drug name in a query string.
type boundary struct {
	Sort   []domain.SortKey `json:"sort"`
	Values []string         `json:"values"`
	ID     string           `json:"id"`
}

func encodeBoundary(row clinical.Medication, sortKeys []domain.SortKey) (string, error) {
	values := make([]string, 0, len(sortKeys))
	for _, key := range sortKeys {
		values = append(values, sortValue(row, key.Field))
	}

	payload, err := json.Marshal(boundary{Sort: sortKeys, Values: values, ID: row.ID})
	if err != nil {
		return "", fmt.Errorf("medicationtest: sealing the boundary: %w", err)
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

	// The ordering is part of the boundary. A boundary read under another
	// ordering names a row that is somewhere else entirely in this sequence,
	// so continuing from it serves an arbitrary slice of the list.
	if !slices.Equal(decoded.Sort, sortKeys) || len(decoded.Values) != len(sortKeys) || decoded.ID == "" {
		return boundary{}, errInvalidCursor
	}

	return decoded, nil
}

// compareToBoundary answers "does this row come after that one", which is the
// keyset predicate: a boundary is a row, and a row does not move when another
// row is inserted above it.
func compareToBoundary(row clinical.Medication, after boundary, sortKeys []domain.SortKey) int {
	for i, key := range sortKeys {
		if compared := compareValues(key.Field, sortValue(row, key.Field), after.Values[i], key.Desc); compared != 0 {
			return compared
		}
	}

	return strings.Compare(after.ID, row.ID)
}

// Authorizer is a checkpoint that answers for one account.
//
// It reports the level it resolved and leaves the comparison against what was
// needed to the service, which is the thing worth testing: a checkpoint that
// answered with a zero grant and no error would be granting nothing, and a
// service that read only the error would let it through.
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

// Refuse makes every answer this error. domain.ErrNotFound is a refusal;
// anything else is a checkpoint that could not answer, and the two are not the
// same thing to the service.
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

// LastPatient is the patient id the last call was made with.
func (a *Authorizer) LastPatient() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.lastPatient
}

// Patient answers from the actor's account alone. It does not model a
// patient-to-account mapping — the fake is coarse on purpose, the same way
// phase 001's fake ignored the record it was asked about — because that
// mapping is access.Authorizer.Patient's own job (internal/service/access).
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

// The stand-in DTOs. internal/web/api owns the real four (contracts/records.md
// §"The medication DTOs"); these exist so the adapter can be tested for the
// thing it actually does — carrying values between the kind-agnostic layer and
// the typed one — without the edge's package existing yet.
//
// They are real structs with real fields, because the failure worth catching is
// the adapter handing on a value of the wrong type, and a map[string]any could
// not be the wrong type.
type (
	// Summary is what a list returns.
	Summary struct {
		ID   string
		Kind string
		Name string
	}

	// Detail is what a read returns.
	Detail struct {
		Summary

		Dosage string
	}

	// Create carries no identity and no version, which is how FR-032 is
	// enforced by shape rather than by a check. Patient is required, mirroring
	// the real DTO (contracts/medications-rescope.md).
	Create struct {
		Patient string
		Name    string
		Dosage  string
	}

	// Patch distinguishes a field that was sent from one that was not.
	Patch struct {
		Name   *string
		Dosage *string
	}
)

// Codec is a medication.Codec over the stand-in DTOs above.
type Codec struct{}

func NewCodec() Codec { return Codec{} }

func (Codec) Summary(entity clinical.Medication) any {
	return &Summary{ID: entity.ID, Kind: kind.Medication.Enum(), Name: entity.Name}
}

func (Codec) Detail(entity clinical.Medication) any {
	return &Detail{
		Summary: Summary{ID: entity.ID, Kind: kind.Medication.Enum(), Name: entity.Name},
		Dosage:  entity.Dosage,
	}
}

func (Codec) Draft(body any) (clinical.Medication, error) {
	create, minted := body.(*Create)
	if !minted {
		return clinical.Medication{}, fmt.Errorf("medicationtest: a create was handed %T and not this package's own create type", body)
	}

	return clinical.Medication{PatientID: create.Patient, Name: create.Name, Dosage: create.Dosage}, nil
}

func (Codec) Patch(body any) (medication.Patch, error) {
	patch, minted := body.(*Patch)
	if !minted {
		return medication.Patch{}, fmt.Errorf("medicationtest: a patch was handed %T and not this package's own patch type", body)
	}

	return medication.Patch{Name: patch.Name, Dosage: patch.Dosage}, nil
}

// Shapes is the four DTO constructors as the registry takes them. The ordering
// and the narrowing are the service's vocabulary and are filled in by
// medication.Register, so they are deliberately absent here.
func Shapes() records.Schema {
	return records.Schema{
		NewSummary: func() any { return &Summary{} },
		NewDetail:  func() any { return &Detail{} },
		NewCreate:  func() any { return &Create{} },
		NewPatch:   func() any { return &Patch{} },
	}
}
