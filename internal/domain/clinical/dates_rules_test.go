package clinical

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func mustCalendarDate(t *testing.T, y int, m time.Month, d int) Date {
	t.Helper()
	date, err := domain.NewDate(y, m, d)
	require.NoError(t, err)
	return date
}

func TestOrder(t *testing.T) {
	t.Parallel()

	jan1 := mustCalendarDate(t, 2026, time.January, 1)
	jan2 := mustCalendarDate(t, 2026, time.January, 2)

	assert.Nil(t, Order(Ref{Field: "started_on", Value: jan1}, Ref{Field: "ended_on", Value: jan2}))
	assert.Nil(t, Order(Ref{Field: "started_on", Value: jan1}, Ref{Field: "ended_on", Value: jan1}), "equality is accepted")
	assert.Nil(t, Order(Ref{Field: "started_on", Value: Date{}}, Ref{Field: "ended_on", Value: jan1}), "absent earlier passes")
	assert.Nil(t, Order(Ref{Field: "started_on", Value: jan1}, Ref{Field: "ended_on", Value: Date{}}), "absent later passes")

	err := Order(Ref{Field: "started_on", Value: jan2}, Ref{Field: "ended_on", Value: jan1})
	require.NotNil(t, err)
	assert.Equal(t, "ended_on", err.Field)
	assert.Equal(t, CodeEndBeforeStart, err.Code)
}

func TestNotFuture(t *testing.T) {
	t.Parallel()

	today := mustCalendarDate(t, 2026, time.June, 15)
	tomorrow := mustCalendarDate(t, 2026, time.June, 16)

	assert.Nil(t, NotFuture(Ref{Field: "occurred_on", Value: today}, today))
	assert.Nil(t, NotFuture(Ref{Field: "occurred_on", Value: Date{}}, today), "absent passes")

	err := NotFuture(Ref{Field: "occurred_on", Value: tomorrow}, today)
	require.NotNil(t, err)
	assert.Equal(t, "occurred_on", err.Field)
	assert.Equal(t, CodeNotFuture, err.Code)
}

func TestRequiredWhen(t *testing.T) {
	t.Parallel()

	set := mustCalendarDate(t, 2026, time.June, 15)

	assert.Nil(t, RequiredWhen(false, Ref{Field: "resolved_on", Value: Date{}}))
	assert.Nil(t, RequiredWhen(true, Ref{Field: "resolved_on", Value: set}))

	err := RequiredWhen(true, Ref{Field: "resolved_on", Value: Date{}})
	require.NotNil(t, err)
	assert.Equal(t, "resolved_on", err.Field)
	assert.Equal(t, domain.CodeRequired, err.Code)
}

// Two simultaneous violations must both be reported (FR-004, FR-013): an
// entity's Validate() accumulates each rule's *FieldError independently rather
// than short-circuiting on the first.
func TestTwoSimultaneousDateViolationsAreBothReported(t *testing.T) {
	t.Parallel()

	today := mustCalendarDate(t, 2026, time.June, 15)
	future := mustCalendarDate(t, 2026, time.June, 20)
	earlier := mustCalendarDate(t, 2026, time.June, 10)

	var invalid domain.ValidationError

	if err := Order(Ref{Field: "onset_on", Value: future}, Ref{Field: "resolved_on", Value: earlier}); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}
	if err := NotFuture(Ref{Field: "onset_on", Value: future}, today); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}

	require.False(t, invalid.Empty())
	assert.Len(t, invalid.Fields, 2)
}
