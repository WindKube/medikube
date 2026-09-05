// Package tagtest is the in-memory Repository the unit-level tests run
// against, and the contract both it and internal/store/tag are held to.
package tagtest

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/tag"
	svc "medikube/internal/service/tag"
)

// OwnerID and StrangerID are the two accounts the contract's cross-owner
// assertions run between.
const (
	OwnerID    = "mkfaketagowner0"
	StrangerID = "mkfaketagstrgr0"
)

// Repository is the in-memory fake. Not safe for concurrent use beyond what
// the mutex below already serializes — tests do not need more.
type Repository struct {
	mu   sync.Mutex
	next int
	rows map[string]tag.Tag

	// usage is the per-id count Counts answers, set by a test to whatever
	// the scenario needs; absent means zero.
	usage map[string]int
}

func NewRepository() *Repository {
	return &Repository{rows: map[string]tag.Tag{}, usage: map[string]int{}}
}

var (
	_ svc.Repository   = (*Repository)(nil)
	_ svc.Ownership    = (*Repository)(nil)
	_ svc.UsageCounter = (*Repository)(nil)
)

// SetUsage is the fake's own extra: a test sets what Counts answers for an
// id, since a fake usage counter has nothing real to derive from.
func (r *Repository) SetUsage(id string, count int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.usage[id] = count
}

func (r *Repository) List(_ context.Context, ownerID string, query svc.Query) (domain.Page[tag.Tag], error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var items []tag.Tag

	for _, t := range r.rows {
		if t.OwnerID != ownerID {
			continue
		}

		if query.Search != "" && !strings.Contains(strings.ToLower(t.Name), strings.ToLower(query.Search)) {
			continue
		}

		items = append(items, t)
	}

	usageDesc := len(query.Sort) > 0 && query.Sort[0].Field == "usage"

	sort.Slice(items, func(i, j int) bool {
		if usageDesc {
			ui, uj := r.usage[items[i].ID], r.usage[items[j].ID]
			if ui != uj {
				return ui > uj
			}
		}

		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	offset := 0

	if query.Cursor != "" {
		parsed, err := strconv.Atoi(query.Cursor)
		if err != nil {
			return domain.Page[tag.Tag]{}, fmt.Errorf("fake tag cursor: %w", err)
		}

		offset = parsed
	}

	limit := query.Limit
	if limit == 0 {
		limit = 25
	}

	end := offset + limit
	if end > len(items) {
		end = len(items)
	}

	var page []tag.Tag
	if offset < len(items) {
		page = items[offset:end]
	}

	var next *string

	if end < len(items) {
		token := strconv.Itoa(end)
		next = &token
	}

	result := domain.NewPage(page, next)

	if query.Count {
		total := len(items)
		result = result.WithTotal(total)
	}

	return result, nil
}

func (r *Repository) Get(_ context.Context, ownerID, id string) (tag.Tag, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	found, ok := r.rows[id]
	if !ok || found.OwnerID != ownerID {
		return tag.Tag{}, fmt.Errorf("fake tag %s: %w", id, domain.ErrNotFound)
	}

	return found, nil
}

func (r *Repository) Create(_ context.Context, t tag.Tag) (tag.Tag, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.duplicateLocked(t.OwnerID, t.Name, "") {
		return tag.Tag{}, svc.ErrDuplicateName
	}

	r.next++
	t.ID = fmt.Sprintf("mkfaketag%06d", r.next)
	t.CreatedAt = time.Now().UTC()
	t.UpdatedAt = t.CreatedAt
	t.Version = t.UpdatedAt.Format(time.RFC3339Nano)

	r.rows[t.ID] = t

	return t, nil
}

func (r *Repository) Update(_ context.Context, t tag.Tag) (tag.Tag, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.rows[t.ID]
	if !ok || current.OwnerID != t.OwnerID {
		return tag.Tag{}, fmt.Errorf("fake tag %s: %w", t.ID, domain.ErrNotFound)
	}

	if r.duplicateLocked(t.OwnerID, t.Name, t.ID) {
		return tag.Tag{}, svc.ErrDuplicateName
	}

	t.CreatedAt = current.CreatedAt
	t.UpdatedAt = time.Now().UTC()
	t.Version = t.UpdatedAt.Format(time.RFC3339Nano)

	r.rows[t.ID] = t

	return t, nil
}

func (r *Repository) Delete(_ context.Context, ownerID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.rows[id]
	if !ok || current.OwnerID != ownerID {
		return fmt.Errorf("fake tag %s: %w", id, domain.ErrNotFound)
	}

	delete(r.rows, id)
	delete(r.usage, id)

	return nil
}

func (r *Repository) Owned(_ context.Context, ownerID string, ids []string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, id := range ids {
		found, ok := r.rows[id]
		if !ok || found.OwnerID != ownerID {
			return false, nil
		}
	}

	return true, nil
}

func (r *Repository) Counts(_ context.Context, ownerID string, ids []string) (map[string]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	counts := make(map[string]int, len(ids))

	for _, id := range ids {
		if found, ok := r.rows[id]; ok && found.OwnerID == ownerID {
			counts[id] = r.usage[id]
		}
	}

	return counts, nil
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

func (a *Authorizer) Actor(_ context.Context, actor access.Actor) (access.Grant, error) {
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

// Auditor collects the rows the service writes.
type Auditor struct {
	mu sync.Mutex

	events []audit.Event
}

func NewAuditor() *Auditor { return &Auditor{} }

func (a *Auditor) Events() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return slices.Clone(a.events)
}

func (a *Auditor) Record(_ context.Context, event audit.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events = append(a.events, event)

	return nil
}

func (r *Repository) duplicateLocked(ownerID, name, exceptID string) bool {
	for _, t := range r.rows {
		if t.ID == exceptID {
			continue
		}

		if t.OwnerID == ownerID && strings.EqualFold(t.Name, name) {
			return true
		}
	}

	return false
}
