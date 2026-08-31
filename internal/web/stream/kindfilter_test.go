package stream

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/realtime"
	"medikube/internal/records/recordstest"
	"medikube/internal/web/views/ids"
)

// T155. The `kind` parameter narrows the stream. All three cases matter
// independently: a filter that admitted everything would pass the first
// assertion, a filter that admitted nothing would pass the second, and a
// parameter that was read and ignored would pass both.

// medications is the row selector a medication event should produce, and
// fakeRow the synthetic kind's — built from the kind table, never spelled.
func rowSelector(k kind.Kind, recordID string) string {
	return "#" + ids.RecordRow(k, recordID)
}

// seed puts one record of each registered kind into the rig's fake services and
// returns their ids.
func seed(t *testing.T, rig *rig) map[kind.Kind]string {
	t.Helper()

	seeded := make(map[kind.Kind]string, len(rig.services))

	for k, service := range rig.services {
		created, err := service.Create(t.Context(), actorOf(recordstest.OwnerID), &recordstest.Create{Name: "Amoxicillin"})
		require.NoError(t, err)

		seeded[k] = created.ID
	}

	return seeded
}

func TestASelectedKindIsStreamedAndAnUnselectedOneIsNot(t *testing.T) {
	t.Parallel()

	rig := newRig(t,
		withKinds(kind.Medication, recordstest.Kind),
		withQuery("?kind="+kind.Medication.Segment()))

	seeded := seed(t, rig)

	rig.drainHeartbeat()

	// The unselected kind first, then the selected one. Once the second frame
	// has arrived the first event has provably been through the loop, so
	// "nothing was sent for it" is a fact rather than a wait.
	rig.publish(realtime.Event{Kind: recordstest.Kind, RecordID: seeded[recordstest.Kind], OwnerID: recordstest.OwnerID})
	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: seeded[kind.Medication], OwnerID: recordstest.OwnerID})

	frame := rig.next()
	assert.Equal(t, rowSelector(kind.Medication, seeded[kind.Medication]), frame.selector(),
		"the frame that arrived is for the kind the subscriber did not ask for")
}

func TestAnAbsentKindParameterSubscribesToEveryRegisteredKind(t *testing.T) {
	t.Parallel()

	rig := newRig(t, withKinds(kind.Medication, recordstest.Kind))

	seeded := seed(t, rig)

	rig.drainHeartbeat()

	rig.publish(realtime.Event{Kind: recordstest.Kind, RecordID: seeded[recordstest.Kind], OwnerID: recordstest.OwnerID})
	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: seeded[kind.Medication], OwnerID: recordstest.OwnerID})

	assert.Equal(t, rowSelector(recordstest.Kind, seeded[recordstest.Kind]), rig.next().selector())
	assert.Equal(t, rowSelector(kind.Medication, seeded[kind.Medication]), rig.next().selector())
}

func TestSeveralSelectedKindsAreAllStreamed(t *testing.T) {
	t.Parallel()

	rig := newRig(t,
		withKinds(kind.Medication, recordstest.Kind),
		withQuery("?kind="+kind.Medication.Segment()+","+recordstest.Segment))

	seeded := seed(t, rig)

	rig.drainHeartbeat()

	rig.publish(realtime.Event{Kind: recordstest.Kind, RecordID: seeded[recordstest.Kind], OwnerID: recordstest.OwnerID})
	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: seeded[kind.Medication], OwnerID: recordstest.OwnerID})

	assert.Equal(t, rowSelector(recordstest.Kind, seeded[recordstest.Kind]), rig.next().selector())
	assert.Equal(t, rowSelector(kind.Medication, seeded[kind.Medication]), rig.next().selector())
}

// An unregistered kind is refused before the stream opens, and refused rather
// than ignored: a stream that silently narrowed to nothing is a live view that
// never updates and never says why.
func TestAnUnregisteredKindIsRefusedBeforeTheStreamOpens(t *testing.T) {
	t.Parallel()

	for name, query := range map[string]string{
		"a kind nothing declares":              "?kind=unicorns",
		"a declared but unregistered kind":     "?kind=" + recordstest.Segment,
		"one good and one bad":                 "?kind=" + kind.Medication.Segment() + ",unicorns",
		"a spelling that only differs by case": "?kind=" + "Medications",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rig := newRig(t, withQuery(query))

			assert.Equal(t, http.StatusUnprocessableEntity, rig.response.StatusCode)
			assert.NotEqual(t, "text/event-stream", rig.response.Header.Get("Content-Type"),
				"the refusal was delivered as a stream, which a client cannot tell from a working stream that never sends anything")
		})
	}
}
