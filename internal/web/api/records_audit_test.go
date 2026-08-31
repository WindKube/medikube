package api_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/apitest"
)

// T152, FR-036 and FR-038. Every write is recorded once, and the row says what
// happened and to which record — never what was written.
//
// The rows are read out of the trail rather than counted through a double, so
// what is asserted is what an operator would find in the collection.

func TestEachWriteRecordsExactlyOneAuditRow(t *testing.T) {
	t.Parallel()

	for name, write := range map[string]struct {
		do     func(*caller) (string, int)
		action audit.Action
	}{
		"a create": {
			do: func(c *caller) (string, int) {
				answer := c.post(collectionURL(), everyField)

				return answer.medication(t).ID, answer.Status
			},
			action: audit.ActionCreate,
		},
		"a change": {
			do: func(c *caller) (string, int) {
				subject := testsupport.NameOnlyMedicationID
				answer := c.patch(recordURL(subject), `{"dosage":"1 g"}`, web.ETag(storedVersion(t, c, subject)))

				return subject, answer.Status
			},
			action: audit.ActionUpdate,
		},
		"a delete": {
			do: func(c *caller) (string, int) {
				subject := testsupport.SingleDayMedicationID
				answer := c.delete(recordURL(subject), web.ETag(storedVersion(t, c, subject)))

				return subject, answer.Status
			},
			action: audit.ActionDelete,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			caller := newCaller(t)

			require.Empty(t, apitest.Events(t, caller.app),
				"the fixture already holds audit rows, so nothing below is attributable to this request")

			subject, status := write.do(caller)
			require.Less(t, status, http.StatusBadRequest, "the write was refused, so there is nothing to have recorded")

			events := apitest.Events(t, caller.app)
			require.Len(t, events, 1, "one write, one row")

			event := events[0]

			assert.Equal(t, write.action, event.Action)
			assert.Equal(t, audit.TargetKindMedication, event.TargetKind)
			assert.Equal(t, subject, event.TargetID)
			assert.Equal(t, audit.ActorKindUser, event.ActorKind)
			assert.Equal(t, testsupport.AccountAID, event.ActorID)
			assert.NotEmpty(t, event.RequestID, "the row correlates to no request, so it cannot be joined to the log line")
			assert.False(t, event.OccurredAt.IsZero())
		})
	}
}

// FR-038, from the only side that can prove it. The row is scanned for every
// value the request carried, so a column added in a later phase that happened to
// hold one of them fails here rather than at the next audit.
func TestAnAuditRowCarriesNoClinicalContent(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	created := caller.post(collectionURL(), everyField)
	require.Equal(t, http.StatusCreated, created.Status, created.Body)

	events := apitest.Events(t, caller.app)
	require.Len(t, events, 1)

	// The values everyField sent, as they were sent. Any of them appearing in
	// any column of the row is the disclosure FR-038 exists to stop.
	for _, secret := range []string{
		"Amoxicillin", "Amoxil", "500 mg", "three times daily",
		"chest infection", "2025-02-01", "2025-02-08", "mild nausea", "finish the course",
	} {
		for _, written := range columns(events[0]) {
			assert.NotContainsf(t, written, secret,
				"an audit row carries %q, which is content and not the fact that something happened", secret)
		}
	}
}

// The trail's shape is the other half of FR-038: a column that could hold
// content is a column somebody will eventually write content into, so the row's
// string members are enumerated here and a new one fails this test until
// somebody says what it holds.
func TestTheAuditRowHasNoColumnContentCouldBeWrittenInto(t *testing.T) {
	t.Parallel()

	shape := reflect.TypeOf(audit.Event{})

	members := make([]string, 0, shape.NumField())
	for index := range shape.NumField() {
		members = append(members, shape.Field(index).Name)
	}

	assert.ElementsMatch(t,
		[]string{"OccurredAt", "ActorID", "ActorKind", "Action", "TargetKind", "TargetID", "RequestID"},
		members,
		"the audit event grew a member; if it can hold a name, a value or a note, FR-038 no longer holds structurally")
}

// columns is every string an event carries, which is what a scan for content
// has to cover. It reads them reflectively so a member added to the struct is
// scanned without this test being edited — the shape test above is where the
// addition is argued about.
func columns(event audit.Event) []string {
	var written []string

	value := reflect.ValueOf(event)
	for index := range value.NumField() {
		field := value.Field(index)
		if field.Kind() == reflect.String {
			written = append(written, field.String())
		}
	}

	return written
}
