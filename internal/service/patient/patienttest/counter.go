package patienttest

import (
	"context"
	"slices"
	"sync"

	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/service/patient"
)

// Counter is the in-memory patient.RecordCounter. It answers a fixed set of
// kinds (registered by SetCount) so a unit test can assert that a kind with
// no rows for a patient still renders its tile at zero (FR-030) without a
// real registry behind it.
type Counter struct {
	mu     sync.Mutex
	kinds  []kind.Kind
	counts map[string]int // kind.Kind + "\x00" + patientID -> count
}

func NewCounter(kinds ...kind.Kind) *Counter {
	return &Counter{kinds: slices.Clone(kinds), counts: make(map[string]int)}
}

// SetCount is the fixture: how many of kind k the patient has.
func (c *Counter) SetCount(k kind.Kind, patientID string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts[string(k)+"\x00"+patientID] = count
}

func (c *Counter) CountsByKind(_ context.Context, patientID string) ([]patient.CountEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make([]patient.CountEntry, 0, len(c.kinds))

	for _, k := range c.kinds {
		entries = append(entries, patient.CountEntry{
			Kind:  k,
			Path:  "/" + k.Segment(),
			Label: string(k),
			Count: c.counts[string(k)+"\x00"+patientID],
		})
	}

	return entries, nil
}

// Activity is the in-memory patient.RecentActivityReader: a fixed slice of
// events, one test seeds per patient id.
type Activity struct {
	mu     sync.Mutex
	events map[string][]domainaudit.Event
}

func NewActivity() *Activity {
	return &Activity{events: make(map[string][]domainaudit.Event)}
}

// Seed appends one event to patientID's own list, most-recent-last: the
// fake reverses it to match the real reader's "newest first" contract.
func (a *Activity) Seed(patientID string, event domainaudit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events[patientID] = append(a.events[patientID], event)
}

func (a *Activity) RecentForPatient(_ context.Context, patientID string, limit int) ([]domainaudit.Event, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	all := a.events[patientID]

	newestFirst := make([]domainaudit.Event, len(all))
	for i, event := range all {
		newestFirst[len(all)-1-i] = event
	}

	if limit <= 0 {
		limit = 10
	}

	if len(newestFirst) > limit {
		newestFirst = newestFirst[:limit]
	}

	return newestFirst, nil
}
