package patienttest

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
)

// The two accounts the fakes answer for, mirroring
// internal/service/medication/medicationtest's own constants.
const (
	OwnerID    = "mkfakeowner0001"
	StrangerID = "mkfakestrangr01"
)

var epoch = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

var errInvalidCursor = errors.New("patienttest: the boundary is not one this repository issued")

// Repository is the in-memory patient.Repository.
type Repository struct {
	mu sync.Mutex

	rows  map[string]person.Patient
	next  int
	clock time.Time
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]person.Patient), clock: epoch}
}

// Seed inserts a row directly, bypassing Create's owner/self-record
// enforcement, for tests that need a fixture rather than an act.
func (r *Repository) Seed(p person.Patient) person.Patient {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.ID == "" {
		p.ID = r.mintID()
	}

	r.clock = r.clock.Add(time.Millisecond)
	p.CreatedAt, p.UpdatedAt = r.clock, r.clock
	p.Version = versionOf(r.clock)
	r.rows[p.ID] = p

	return p
}

func (r *Repository) mintID() string {
	r.next++

	return fmt.Sprintf("mkfakepatient%02d", r.next)
}

func (r *Repository) List(_ context.Context, ownerID string, query patient.Query) (domain.Page[person.Patient], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sortKeys := query.Sort
	if len(sortKeys) == 0 {
		sortKeys = defaultSort()
	}

	matched := make([]person.Patient, 0, len(r.rows))

	for _, row := range r.rows {
		if row.OwnerID != ownerID {
			continue
		}

		if query.Search != "" && !matchesSearch(row, query.Search) {
			continue
		}

		matched = append(matched, row)
	}

	slices.SortFunc(matched, func(left, right person.Patient) int {
		return compareRows(left, right, sortKeys)
	})

	total := len(matched)

	if query.Cursor != "" {
		afterID, err := decodeCursor(query.Cursor)
		if err != nil {
			return domain.Page[person.Patient]{}, err
		}

		idx := slices.IndexFunc(matched, func(p person.Patient) bool { return p.ID == afterID })
		if idx >= 0 {
			matched = matched[idx+1:]
		}
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 25
	}

	var next *string

	if len(matched) > limit {
		token := encodeCursor(matched[limit-1].ID)
		next = &token
		matched = matched[:limit]
	}

	page := domain.NewPage(matched, next)

	if query.Count {
		page = page.WithTotal(total)
	}

	return page, nil
}

func (r *Repository) Get(_ context.Context, ownerID, id string) (person.Patient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	row, ok := r.rows[id]
	if !ok || row.OwnerID != ownerID {
		return person.Patient{}, domain.ErrNotFound
	}

	return row, nil
}

func (r *Repository) Create(_ context.Context, draft person.Patient) (person.Patient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if draft.IsSelfRecord {
		for _, row := range r.rows {
			if row.OwnerID == draft.OwnerID && row.IsSelfRecord {
				return person.Patient{}, domain.ErrConflict
			}
		}
	}

	draft.ID = r.mintID()
	r.clock = r.clock.Add(time.Millisecond)
	draft.CreatedAt, draft.UpdatedAt = r.clock, r.clock
	draft.Version = versionOf(r.clock)

	r.rows[draft.ID] = draft

	return draft, nil
}

func (r *Repository) Update(_ context.Context, changed person.Patient, expectedVersion string) (person.Patient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.rows[changed.ID]
	if !ok || current.OwnerID != changed.OwnerID {
		return person.Patient{}, domain.ErrNotFound
	}

	if current.Version != expectedVersion {
		return person.Patient{}, domain.ErrVersionMismatch
	}

	r.clock = r.clock.Add(time.Millisecond)
	changed.CreatedAt = current.CreatedAt
	changed.UpdatedAt = r.clock
	changed.Version = versionOf(r.clock)

	r.rows[changed.ID] = changed

	return changed, nil
}

func (r *Repository) SelfRecord(_ context.Context, ownerID string) (person.Patient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, row := range r.rows {
		if row.OwnerID == ownerID && row.IsSelfRecord {
			return row, nil
		}
	}

	return person.Patient{}, domain.ErrNotFound
}

func matchesSearch(row person.Patient, search string) bool {
	needle := strings.ToLower(search)

	return strings.Contains(strings.ToLower(row.FirstName), needle) ||
		strings.Contains(strings.ToLower(row.LastName), needle)
}

func defaultSort() []domain.SortKey {
	return []domain.SortKey{{Field: "last_name"}, {Field: "first_name"}, {Field: "id"}}
}

func compareRows(left, right person.Patient, sortKeys []domain.SortKey) int {
	for _, key := range sortKeys {
		var cmp int

		switch key.Field {
		case "last_name":
			cmp = strings.Compare(strings.ToLower(left.LastName), strings.ToLower(right.LastName))
		case "first_name":
			cmp = strings.Compare(strings.ToLower(left.FirstName), strings.ToLower(right.FirstName))
		default:
			cmp = strings.Compare(left.ID, right.ID)
		}

		if key.Desc {
			cmp = -cmp
		}

		if cmp != 0 {
			return cmp
		}
	}

	return strings.Compare(left.ID, right.ID)
}

func encodeCursor(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

func decodeCursor(token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errInvalidCursor, err)
	}

	return string(raw), nil
}

func versionOf(at time.Time) string {
	return strconv.FormatInt(at.UnixNano(), 36)
}

// Authorizer is the in-memory patient.Authorizer: the owner reaches PermOwn,
// everybody else is refused exactly as
// internal/service/access.Authorizer.Patient refuses (domain.ErrUnauthenticated
// for no session, domain.ErrNotFound otherwise), so a service test run
// against this fake exercises the same shape of refusal the real checkpoint
// produces.
type Authorizer struct {
	mu      sync.Mutex
	repo    *Repository
	auditor *Auditor
}

func NewAuthorizer(repo *Repository, auditor *Auditor) *Authorizer {
	return &Authorizer{repo: repo, auditor: auditor}
}

func (a *Authorizer) Patient(ctx context.Context, actor access.Actor, patientID string, _ access.Permission) (access.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !actor.Authenticated() {
		a.deny(ctx, actor, patientID)

		return access.Grant{}, domain.ErrUnauthenticated
	}

	row, err := a.repo.Get(ctx, actor.UserID, patientID)
	if err != nil || row.OwnerID != actor.UserID {
		a.deny(ctx, actor, patientID)

		return access.Grant{}, domain.ErrNotFound
	}

	return access.Grant{Level: access.PermOwn}, nil
}

func (a *Authorizer) deny(ctx context.Context, actor access.Actor, patientID string) {
	if a.auditor == nil {
		return
	}

	_ = a.auditor.Record(ctx, audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionAccessDenied,
		TargetKind: audit.TargetKindPatient,
		TargetID:   patientID,
		RequestID:  actor.RequestID,
		PatientID:  patientID,
	})
}

// ActivePatientStore is the in-memory patient.ActivePatientStore: one
// pointer per account, mirroring users.active_patient.
type ActivePatientStore struct {
	mu      sync.Mutex
	pointer map[string]string
}

func NewActivePatientStore() *ActivePatientStore {
	return &ActivePatientStore{pointer: make(map[string]string)}
}

func (s *ActivePatientStore) ActivePatient(_ context.Context, userID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.pointer[userID], nil
}

func (s *ActivePatientStore) SetActivePatient(_ context.Context, userID, patientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if patientID == "" {
		delete(s.pointer, userID)

		return nil
	}

	s.pointer[userID] = patientID

	return nil
}

// Auditor is the in-memory recorder every unit test can inspect.
type Auditor struct {
	mu     sync.Mutex
	events []audit.Event
}

func NewAuditor() *Auditor { return &Auditor{} }

func (a *Auditor) Record(_ context.Context, event audit.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events = append(a.events, event)

	return nil
}

func (a *Auditor) Events() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return slices.Clone(a.events)
}

// PhotoStore is the in-memory patient.PhotoStore. It sniffs content the way
// PocketBase does — from bytes, never from the declared name — so the unit
// tests this fake serves (T046) exercise the same refusal FR-008 requires
// without a real filesystem.
type PhotoStore struct {
	mu           sync.Mutex
	maxBytes     int64
	allowed      []string
	photos       map[string]photoRecord
	removedCount int
}

type photoRecord struct {
	contentType string
	sizes       []string
	updatedAt   time.Time
}

func NewPhotoStore(maxBytes int64, allowed []string) *PhotoStore {
	return &PhotoStore{maxBytes: maxBytes, allowed: allowed, photos: make(map[string]photoRecord)}
}

func (p *PhotoStore) Put(_ context.Context, _, patientID string, upload patient.Upload) (patient.PhotoMeta, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if upload.Size > p.maxBytes {
		return patient.PhotoMeta{}, domain.ErrTooLarge
	}

	buf := make([]byte, 512)

	n, _ := upload.Reader.Read(buf)
	sniffed := sniffImageType(buf[:n])

	if !slices.Contains(p.allowed, sniffed) {
		return patient.PhotoMeta{}, domain.ErrUnsupportedMedia
	}

	now := time.Now().UTC()
	p.photos[patientID] = photoRecord{contentType: sniffed, sizes: []string{"original", "100x100t", "400x400f"}, updatedAt: now}

	return patient.PhotoMeta{Sizes: []string{"original", "100x100t", "400x400f"}, UpdatedAt: now.Format(time.RFC3339)}, nil
}

// sniffImageType is this fake's own content sniff — from the bytes, never
// the declared name, the same rule FR-008 states — kept to the three formats
// contracts/patient-photo.md ever allows so this package (internal/service
// may import only the standard library and zerolog) needs no net/http or
// third-party sniffer to stand in for PocketBase's real one.
func sniffImageType(buf []byte) string {
	switch {
	case bytes.HasPrefix(buf, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png"
	case bytes.HasPrefix(buf, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case len(buf) >= 12 && bytes.Equal(buf[0:4], []byte("RIFF")) && bytes.Equal(buf[8:12], []byte("WEBP")):
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

func (p *PhotoStore) Remove(_ context.Context, _, patientID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.photos[patientID]; ok {
		delete(p.photos, patientID)
		p.removedCount++
	}

	return nil
}

// Has reports whether a photograph is currently stored, and its sizes.
func (p *PhotoStore) Has(patientID string) ([]string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	record, ok := p.photos[patientID]
	if !ok {
		return nil, false
	}

	return slices.Clone(record.sizes), true
}

// RemovedCount is how many times Remove actually deleted something, for
// asserting a replacement removed the previous file exactly once (US1-5).
func (p *PhotoStore) RemovedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.removedCount
}
