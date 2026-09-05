package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainaudit "medikube/internal/domain/audit"
)

// fakeReader is the consumer-declared Reader port, by hand.
type fakeReader struct {
	events []domainaudit.Event
	err    error

	gotPatient string
	gotLimit   int
}

func (f *fakeReader) RecentForPatient(_ context.Context, patientID string, limit int) ([]domainaudit.Event, error) {
	f.gotPatient = patientID
	f.gotLimit = limit

	if f.err != nil {
		return nil, f.err
	}

	return f.events, nil
}

func recentEvent(patientID string) domainaudit.Event {
	return domainaudit.Event{
		OccurredAt: time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC),
		ActorID:    "usr0000000001",
		ActorKind:  domainaudit.ActorKindUser,
		Action:     domainaudit.ActionUpdate,
		TargetKind: domainaudit.TargetKindPatient,
		TargetID:   patientID,
		RequestID:  "01K3Q8Z0000000000000000000",
		PatientID:  patientID,
	}
}

func TestRecentForPatientPassesThroughTheReadersRows(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{events: []domainaudit.Event{recentEvent("mkptamara00001")}}
	activity, err := NewRecentActivity(reader)
	require.NoError(t, err)

	events, err := activity.RecentForPatient(t.Context(), "mkptamara00001", 5)
	require.NoError(t, err)

	assert.Equal(t, reader.events, events)
	assert.Equal(t, "mkptamara00001", reader.gotPatient)
	assert.Equal(t, 5, reader.gotLimit)

	// The whole point of the type this reads back: nothing in the row is a
	// name, a value or a diff (data-model §3), so asserting that here is
	// asserting a property of the transport, not re-testing Event.Validate.
	for _, event := range events {
		assert.NotZero(t, event.Action)
		assert.NotZero(t, event.TargetKind)
		assert.NotEmpty(t, event.RequestID)
	}
}

func TestRecentForPatientAppliesTheDefaultLimitToANonPositiveOne(t *testing.T) {
	t.Parallel()

	cases := []int{0, -1, -100}

	for _, limit := range cases {
		reader := &fakeReader{}
		activity, err := NewRecentActivity(reader)
		require.NoError(t, err)

		_, err = activity.RecentForPatient(t.Context(), "mkptamara00001", limit)
		require.NoError(t, err)
		assert.Equal(t, DefaultRecentLimit, reader.gotLimit)
	}
}

func TestRecentForPatientRefusesAnEmptyPatientID(t *testing.T) {
	t.Parallel()

	activity, err := NewRecentActivity(&fakeReader{})
	require.NoError(t, err)

	_, err = activity.RecentForPatient(t.Context(), "", 5)
	assert.Error(t, err)
}

func TestRecentForPatientReportsAReaderFailure(t *testing.T) {
	t.Parallel()

	broken := errors.New("the read could not be made")
	activity, err := NewRecentActivity(&fakeReader{err: broken})
	require.NoError(t, err)

	_, err = activity.RecentForPatient(t.Context(), "mkptamara00001", 5)
	require.ErrorIs(t, err, broken)
}

func TestNewRecentActivityRefusesAWiringWithNoReader(t *testing.T) {
	t.Parallel()

	activity, err := NewRecentActivity(nil)
	require.Error(t, err)
	assert.Nil(t, activity)
}
