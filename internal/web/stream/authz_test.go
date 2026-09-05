package stream

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/realtime"
	"medikube/internal/records/recordstest"
)

// T154, the mechanical half. contracts/streams.md: the checkpoint runs "for
// every single event" and the check "must not be hoisted out of the loop".
// Hoisting it is the defect this file exists to catch, and it is invisible from
// the outside: a stream that authorised once at subscribe time behaves exactly
// like a correct one for as long as nothing changes.

// records one allowed record and returns its id.
func seeded(t *testing.T, rig *rig) string {
	t.Helper()

	created, err := rig.services[kind.Medication].Create(t.Context(), actorOf(recordstest.OwnerID),
		&recordstest.Create{Name: "Amoxicillin"})
	require.NoError(t, err)

	return created.ID
}

func TestTheCheckpointIsConsultedOncePerEventAndNotOncePerStream(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	id := seeded(t, rig)

	rig.drainHeartbeat()

	const events = 4

	for range events {
		rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, PatientID: recordstest.OwnerID})
		require.Equal(t, "datastar-patch-elements", rig.next().Event)
	}

	consultations := rig.authorizer.consultations()

	require.Lenf(t, consultations, events,
		"%d events produced %d authorization checks — a check made once and reused cannot notice access being lost while the stream is open",
		events, len(consultations))

	for index, call := range consultations {
		assert.Equalf(t, recordstest.OwnerID, call.Actor, "check %d was made for somebody else", index)
		assert.Equalf(t, recordstest.OwnerID, call.PatientID, "check %d named the wrong patient", index)
		assert.Equalf(t, access.PermView, call.Need, "check %d asked for the wrong permission", index)
	}
}

// A refusal is zero frames. Not an empty patch, not a removal: a removal names
// a record id, and naming one is the disclosure FR-033 closes.
//
// Authorization is patient-level (research D-13): a stream is opened for one
// patient, so a refusal is refused for every one of that patient's records and
// not for one record while a sibling stays visible.
func TestARefusedEventProducesNoFrameAtAll(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	refused := seeded(t, rig)

	rig.authorizer.deny(recordstest.OwnerID)

	rig.drainHeartbeat()

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: refused, PatientID: recordstest.OwnerID})

	select {
	case f := <-rig.frames:
		t.Fatalf("a refused event produced a frame: %+v", f)
	case <-time.After(200 * time.Millisecond):
	}

	require.Len(t, rig.authorizer.consultations(), 1,
		"the refused event never reached the checkpoint, so this test would pass with the check removed")
}

// contracts/streams.md's Edge Case: a subscriber who loses access mid-stream
// "stops receiving patches without the stream erroring". Both halves matter —
// a stream that closed would reconnect straight back into the same refusal.
func TestLosingAccessMidStreamStopsThePatchesAndNotTheStream(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	id := seeded(t, rig)

	rig.drainHeartbeat()

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, PatientID: recordstest.OwnerID})
	require.Equal(t, rowSelector(kind.Medication, id), rig.next().selector())

	rig.authorizer.deny(recordstest.OwnerID)

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, PatientID: recordstest.OwnerID})

	select {
	case f := <-rig.frames:
		t.Fatalf("a patch was sent for a patient the subscriber may no longer see: %+v", f)
	case <-time.After(200 * time.Millisecond):
	}

	assert.NoError(t, rig.failure(), "losing access closed the stream instead of quietly stopping the patches")
}

// A checkpoint that could not answer has refused nobody. Treating its failure
// as a refusal would make a database outage read as "that record is not yours"
// and leave a live view silently wrong; ending the stream is what the browser
// reconnects from.
func TestACheckpointThatFailsEndsTheStreamRatherThanPatchingAnyway(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	id := seeded(t, rig)

	rig.drainHeartbeat()

	broken := errors.New("the checkpoint could not answer")
	rig.authorizer.failWith(broken)

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, PatientID: recordstest.OwnerID})

	select {
	case _, open := <-rig.frames:
		assert.False(t, open, "a frame was sent for an event the checkpoint could not decide")
	case <-time.After(5 * time.Second):
		t.Fatal("the stream neither ended nor sent anything")
	}

	assert.ErrorIs(t, rig.failure(), broken)
}

// The kind's own filter runs before the checkpoint and is not a stand-in for
// it. A filter that refuses is zero frames too, but the reason is different and
// so is the layer.
func TestTheKindsOwnFilterCanKeepAnEventOffEveryStream(t *testing.T) {
	t.Parallel()

	rig := newRig(t, withStreamFilterDenying())

	id := seeded(t, rig)

	rig.drainHeartbeat()

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, PatientID: recordstest.OwnerID})

	select {
	case f := <-rig.frames:
		assert.Equal(t, "datastar-patch-signals", f.Event,
			"the kind refused the event and it was streamed anyway")
	case <-time.After(200 * time.Millisecond):
	}

	assert.Empty(t, rig.authorizer.consultations(),
		"an event the kind refused still reached the checkpoint")
}

// The removal path is patient-scoped, and that guard is load-bearing:
// an event naming a patient other than the one this stream was opened for is
// filtered before it reaches the checkpoint at all (step 0 of patch), so a
// deletion on another patient's records never puts that patient's record id on
// this subscriber's wire.
func TestARecordThatVanishedOnAnotherPatientProducesNoRemoval(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	mine := seeded(t, rig)

	rig.drainHeartbeat()

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: "mkstranger000001", PatientID: "mkacctsomebody01"})
	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: mine, PatientID: recordstest.OwnerID})

	frame := rig.next()
	assert.Equal(t, rowSelector(kind.Medication, mine), frame.selector(),
		"a removal naming another patient's record id reached this subscriber")
}
