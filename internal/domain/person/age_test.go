package person

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func TestAgeAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		birth      domain.Date
		on         time.Time
		wantYears  int
		wantMonths int
		wantDays   int
		wantString string
		wantRecord bool
	}{
		{
			name:       "born today",
			birth:      mustDate(t, 2026, 9, 5),
			on:         time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
			wantString: "0 days",
			wantRecord: true,
		},
		{
			name:       "born yesterday",
			birth:      mustDate(t, 2026, 9, 4),
			on:         time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
			wantDays:   1,
			wantString: "1 day",
			wantRecord: true,
		},
		{
			name:       "the day before a birthday",
			birth:      mustDate(t, 2020, 6, 15),
			on:         time.Date(2021, 6, 14, 0, 0, 0, 0, time.UTC),
			wantMonths: 11,
			wantDays:   30,
			wantString: "11 months, 30 days",
			wantRecord: true,
		},
		{
			name:       "on a birthday",
			birth:      mustDate(t, 2020, 6, 15),
			on:         time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC),
			wantYears:  1,
			wantString: "1 year",
			wantRecord: true,
		},
		{
			name:       "29 February birth date evaluated in a non-leap year",
			birth:      mustDate(t, 2020, 2, 29),
			on:         time.Date(2021, 2, 28, 0, 0, 0, 0, time.UTC),
			wantMonths: 11,
			wantDays:   30,
			wantString: "11 months, 30 days",
			wantRecord: true,
		},
		{
			name:       "unset birth date",
			birth:      domain.Date{},
			on:         time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
			wantString: "not recorded",
			wantRecord: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			age := AgeAt(tt.birth, tt.on)

			assert.Equal(t, tt.wantRecord, age.Recorded())
			assert.Equal(t, tt.wantYears, age.Years())
			assert.Equal(t, tt.wantMonths, age.Months())
			assert.Equal(t, tt.wantDays, age.Days())
			assert.Equal(t, tt.wantString, age.String())
		})
	}
}

func mustDate(t *testing.T, year int, month time.Month, day int) domain.Date {
	t.Helper()
	d, err := domain.NewDate(year, month, day)
	require.NoError(t, err)
	return d
}
