package domain

import (
	"database/sql/driver"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	// The zone database, so the process-zone matrix below does not depend on
	// zoneinfo being installed in the distroless image or in CI.
	_ "time/tzdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustDate(t *testing.T, year int, month time.Month, day int) Date {
	t.Helper()

	d, err := NewDate(year, month, day)
	require.NoError(t, err)
	return d
}

// research D-27, structurally. A time.Time in this type is the bug: it carries
// an instant and a location, and 2026-03-01 stored as midnight UTC is
// 28 February to a reader in UTC-5. The type has no member that could move.
func TestDateCarriesNoInstantAndNoLocation(t *testing.T) {
	t.Parallel()

	banned := map[reflect.Type]string{
		reflect.TypeOf(time.Time{}):      "an instant, which is not a calendar date",
		reflect.TypeOf(&time.Location{}): "a zone, which a calendar date does not have",
		reflect.TypeOf(time.Duration(0)): "an offset, which a calendar date does not have",
	}

	structure := reflect.TypeOf(Date{})
	require.Equal(t, reflect.Struct, structure.Kind())
	require.NotZero(t, structure.NumField(), "Date has no fields at all")

	for i := range structure.NumField() {
		field := structure.Field(i)
		if reason, forbidden := banned[field.Type]; forbidden {
			assert.Failf(t, "Date carries a moving part", "field %s is %s — %s", field.Name, field.Type, reason)
		}
	}
}

func TestNewDateAcceptsOnlyRealCalendarDates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		year  int
		month time.Month
		day   int
		valid bool
	}{
		{name: "an ordinary day", year: 2026, month: time.March, day: 1, valid: true},
		{name: "the last day of a leap February", year: 2024, month: time.February, day: 29, valid: true},
		{name: "the first day representable", year: 1, month: time.January, day: 1, valid: true},
		{name: "the last day representable", year: 9999, month: time.December, day: 31, valid: true},
		{name: "the thirtieth of February", year: 2026, month: time.February, day: 30},
		{name: "the twenty-ninth of a non-leap February", year: 2026, month: time.February, day: 29},
		{name: "the thirty-first of a thirty-day month", year: 2026, month: time.April, day: 31},
		{name: "a thirteenth month", year: 2026, month: time.Month(13), day: 1},
		{name: "a zeroth month", year: 2026, month: time.Month(0), day: 1},
		{name: "a zeroth day", year: 2026, month: time.March, day: 0},
		{name: "a year that cannot be written in four digits", year: 10000, month: time.January, day: 1},
		{name: "a year before the common era", year: 0, month: time.January, day: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			d, err := NewDate(test.year, test.month, test.day)
			if !test.valid {
				// Rolling 30 February forward to 2 March is what time.Date does
				// and is never what a person meant when they typed it.
				require.Error(t, err, "an impossible date was accepted and normalised")
				assert.True(t, d.IsZero(), "a refused date must not leave a usable value behind")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.year, d.Year())
			assert.Equal(t, test.month, d.Month())
			assert.Equal(t, test.day, d.Day())
			assert.False(t, d.IsZero())
		})
	}
}

func TestParseDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
		valid bool
	}{
		{name: "the wire form", input: "2026-03-01", want: "2026-03-01", valid: true},
		{name: "a leap day", input: "2024-02-29", want: "2024-02-29", valid: true},
		{name: "the empty string is the absent date", input: "", want: "", valid: true},
		{name: "an unpadded month", input: "2026-3-01"},
		{name: "an unpadded day", input: "2026-03-1"},
		{name: "no separators", input: "20260301"},
		{name: "slashes", input: "2026/03/01"},
		{name: "day first", input: "01-03-2026"},
		{name: "an impossible day", input: "2026-02-30"},
		{name: "an instant, not a date", input: "2026-03-01T00:00:00Z"},
		{name: "trailing space", input: "2026-03-01 "},
		{name: "leading space", input: " 2026-03-01"},
		{name: "a word", input: "today"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			d, err := ParseDate(test.input)
			if !test.valid {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), test.input,
					"the refused value is in the error text, and a date is medical data (constitution VII)")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, d.String())
		})
	}
}

func TestZeroDateIsTheAbsentDate(t *testing.T) {
	t.Parallel()

	var d Date

	assert.True(t, d.IsZero())
	assert.Equal(t, "", d.String())

	value, err := d.Value()
	require.NoError(t, err)
	assert.Nil(t, value, "an absent date is SQL NULL, not the zeroth day of the zeroth month")

	raw, err := json.Marshal(d)
	require.NoError(t, err)
	assert.JSONEq(t, `""`, string(raw))
}

func TestDateRoundTripsThroughJSON(t *testing.T) {
	t.Parallel()

	type medication struct {
		StartedOn Date  `json:"started_on"`
		EndedOn   *Date `json:"ended_on,omitempty"`
	}

	ended := mustDate(t, 2026, time.April, 30)
	original := medication{StartedOn: mustDate(t, 2026, time.March, 1), EndedOn: &ended}

	raw, err := json.Marshal(original)
	require.NoError(t, err)
	assert.JSONEq(t, `{"started_on":"2026-03-01","ended_on":"2026-04-30"}`, string(raw),
		"a date on the wire is YYYY-MM-DD and never an instant")

	var decoded medication
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, original.StartedOn, decoded.StartedOn)
	require.NotNil(t, decoded.EndedOn)
	assert.Equal(t, *original.EndedOn, *decoded.EndedOn)

	t.Run("an absent optional date is absent, not null", func(t *testing.T) {
		t.Parallel()

		raw, err := json.Marshal(medication{StartedOn: mustDate(t, 2026, time.March, 1)})
		require.NoError(t, err)
		assert.JSONEq(t, `{"started_on":"2026-03-01"}`, string(raw))
	})

	t.Run("a malformed date is refused by the decoder", func(t *testing.T) {
		t.Parallel()

		var decoded medication
		assert.Error(t, json.Unmarshal([]byte(`{"started_on":"2026-02-30"}`), &decoded))
	})
}

func TestDateRoundTripsThroughTheStoreMapping(t *testing.T) {
	t.Parallel()

	t.Run("what Value writes, Scan reads back", func(t *testing.T) {
		t.Parallel()

		original := mustDate(t, 2026, time.March, 1)

		value, err := original.Value()
		require.NoError(t, err)
		assert.Equal(t, driver.Value("2026-03-01"), value)

		var scanned Date
		require.NoError(t, scanned.Scan(value))
		assert.Equal(t, original, scanned)
	})

	tests := []struct {
		name  string
		src   any
		want  string
		valid bool
	}{
		{name: "the canonical form", src: "2026-03-01", want: "2026-03-01", valid: true},
		{name: "bytes from the driver", src: []byte("2026-03-01"), want: "2026-03-01", valid: true},
		{name: "PocketBase's stored date", src: "2026-03-01 00:00:00.000Z", want: "2026-03-01", valid: true},
		{name: "RFC3339 midnight UTC", src: "2026-03-01T00:00:00Z", want: "2026-03-01", valid: true},
		{name: "a time.Time at midnight UTC", src: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC), want: "2026-03-01", valid: true},
		{name: "NULL", src: nil, want: "", valid: true},
		{name: "the empty string", src: "", want: "", valid: true},
		// Truncating a real time of day is how the off-by-one-day returns: it
		// reads as a successful load and moves the record by a day. A date
		// column holding an instant is a schema fault, so it is refused loudly.
		{name: "an instant with a time of day", src: "2026-03-01 13:45:00.000Z"},
		{name: "a time.Time with a time of day", src: time.Date(2026, time.March, 1, 13, 45, 0, 0, time.UTC)},
		{name: "midnight in another zone", src: "2026-03-01T00:00:00+13:00"},
		{name: "an integer", src: 20260301},
		{name: "nonsense", src: "the first of March"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var d Date
			err := d.Scan(test.src)
			if !test.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, d.String())
		})
	}
}

// FR-018 is a comparison between two of these, so the ordering is part of the
// contract and equality has to be accepted (a single-day course is valid).
func TestDateOrdering(t *testing.T) {
	t.Parallel()

	first := mustDate(t, 2026, time.March, 1)
	same := mustDate(t, 2026, time.March, 1)
	later := mustDate(t, 2026, time.March, 2)
	nextMonth := mustDate(t, 2026, time.April, 1)
	nextYear := mustDate(t, 2027, time.January, 1)

	assert.Equal(t, 0, first.Compare(same))
	assert.Equal(t, first, same, "two equal dates are ==, so no Equal method is needed")

	for _, greater := range []Date{later, nextMonth, nextYear} {
		assert.Negative(t, first.Compare(greater))
		assert.Positive(t, greater.Compare(first))
		assert.True(t, first.Before(greater))
		assert.True(t, greater.After(first))
		assert.False(t, greater.Before(first))
	}

	assert.False(t, first.Before(same))
	assert.False(t, first.After(same))
}

const (
	tzProbeZone = "MEDIKUBE_DATE_TZ_PROBE_ZONE"
	tzProbeOut  = "MEDIKUBE_DATE_TZ_PROBE_OUT"
)

type tzProbeResult struct {
	Zone string `json:"zone"`
	// The date, marshalled by this package.
	Date string `json:"date"`
	// The same calendar day as a time.Time, rendered the way a naive
	// implementation would. This one is expected to disagree between zones —
	// it is the negative control that proves the probe really changed the
	// process zone rather than running three identical processes.
	Control string `json:"control"`
}

// FR-019 and research D-27: "a value read in one process-level TZ and rendered
// in another is byte-identical". TZ is read once per process, so the only
// honest way to assert it is to run the value in three processes.
func TestDateIsTheSameCalendarDayInEveryProcessTimeZone(t *testing.T) {
	t.Parallel()

	if os.Getenv(tzProbeZone) != "" {
		t.Skip("this process is a probe child")
	}

	zones := []string{"UTC", "Pacific/Auckland", "America/Los_Angeles"}
	results := make([]tzProbeResult, 0, len(zones))

	for _, zone := range zones {
		result := runDateTimeZoneProbe(t, zone)
		assert.Equalf(t, "2026-03-01", result.Date, "the calendar date moved under TZ=%s", zone)
		results = append(results, result)
	}

	require.Len(t, results, len(zones))
	for _, result := range results[1:] {
		assert.Equal(t, results[0].Date, result.Date,
			"two processes in different zones disagree about the same calendar date")
	}

	controls := map[string]bool{}
	for _, result := range results {
		controls[result.Control] = true
	}
	assert.Greater(t, len(controls), 1,
		"the naive control rendered the same day in every zone, so the probe never changed the process zone and this test proves nothing")
}

// The probe half: skipped in an ordinary run, and the whole test when the
// parent re-executes this binary with TZ set.
func TestDateTimeZoneProbe(t *testing.T) {
	zone := os.Getenv(tzProbeZone)
	if zone == "" {
		t.Skip("not a probe run")
	}

	require.Equal(t, zone, time.Local.String(), "the child process did not adopt TZ")

	d, err := ParseDate("2026-03-01")
	require.NoError(t, err)

	raw, err := json.Marshal(tzProbeResult{
		Zone:    zone,
		Date:    d.String(),
		Control: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC).In(time.Local).Format(time.DateOnly),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(os.Getenv(tzProbeOut), raw, 0o600))
}

func runDateTimeZoneProbe(t *testing.T, zone string) tzProbeResult {
	t.Helper()

	out := filepath.Join(t.TempDir(), "probe.json")

	cmd := exec.Command(os.Args[0], "-test.run=^TestDateTimeZoneProbe$", "-test.count=1")
	cmd.Env = append(os.Environ(), "TZ="+zone, tzProbeZone+"="+zone, tzProbeOut+"="+out)

	combined, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "probe in TZ=%s failed:\n%s", zone, combined)

	raw, err := os.ReadFile(out)
	require.NoErrorf(t, err, "probe in TZ=%s wrote no result:\n%s", zone, combined)

	var result tzProbeResult
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}
