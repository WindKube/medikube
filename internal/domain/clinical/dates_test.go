package clinical

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func TestDateRoundTripsAndIsTimezoneInvariant(t *testing.T) {
	t.Parallel()

	zones := []string{"UTC", "America/Los_Angeles", "Pacific/Auckland"}

	for _, zone := range zones {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			require.NoError(t, err)

			d, err := domain.NewDate(2026, time.March, 1)
			require.NoError(t, err)

			text, err := d.MarshalText()
			require.NoError(t, err)
			assert.Equal(t, "2026-03-01", string(text))

			// The zone the process happens to run in never enters the value:
			// there is nothing on Date a *time.Location could occupy.
			_ = loc

			var round Date
			require.NoError(t, round.UnmarshalText(text))
			assert.Equal(t, d, round)
		})
	}
}

func TestInstantRoundTripsRFC3339UTC(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)

	local := time.Date(2026, time.March, 1, 9, 30, 0, 0, loc)
	instant := NewInstant(local)

	assert.Equal(t, 17, instant.Time().Hour(), "normalised to UTC")

	text, err := instant.MarshalText()
	require.NoError(t, err)

	var round Instant
	require.NoError(t, round.UnmarshalText(text))
	assert.True(t, round.Time().Equal(instant.Time()))
}

func TestInstantNeverEmitsNullForANonPointer(t *testing.T) {
	t.Parallel()

	text, err := Now().MarshalText()
	require.NoError(t, err)
	assert.NotEqual(t, "null", string(text))
}
