package person

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

// The only two fields the marshaller may emit. Everything else on a Patient is
// PHI outright — a name, a birth date, an address, a body measurement, a
// relationship — and FR-046 keeps all of it out of the operational record.
// This is an allowlist by name, so a field added later is redacted by default.
var loggablePatientFields = map[string]bool{
	"ID":      true,
	"OwnerID": true,
}

var sentinelDate = time.Date(2031, time.July, 19, 0, 0, 0, 0, time.UTC)
var sentinelInstant = time.Date(2032, time.August, 20, 4, 5, 6, 0, time.UTC)

type sentinelField struct {
	name     string
	loggable bool
	tokens   []string
}

// fillWithSentinels writes a value into every field of a Patient that could
// only have come from that field, and reports what to search the rendered
// line for. Reflection rather than a list somebody maintains: a field added to
// the struct is covered the moment it exists.
func fillWithSentinels(t *testing.T, p *Patient) []sentinelField {
	t.Helper()

	value := reflect.ValueOf(p).Elem()
	structure := value.Type()
	require.NotZero(t, structure.NumField(), "Patient has no fields at all")

	fields := make([]sentinelField, 0, structure.NumField())
	for i := range structure.NumField() {
		field := structure.Field(i)
		target := value.Field(i)

		found := sentinelField{name: field.Name, loggable: loggablePatientFields[field.Name]}

		switch {
		case target.Kind() == reflect.String:
			sentinel := fmt.Sprintf("SENTINEL%dZ", i)
			target.SetString(sentinel)
			found.tokens = []string{sentinel}
		case target.Kind() == reflect.Bool:
			target.SetBool(true)
			found.tokens = nil // no rendering of a bool could disclose anything
		case target.Kind() == reflect.Float64:
			sentinel := 123.0 + float64(i)
			target.SetFloat(sentinel)
			found.tokens = []string{fmt.Sprintf("%v", sentinel)}
		case field.Type == reflect.TypeOf(domain.Date{}):
			date, err := domain.NewDate(sentinelDate.Year(), sentinelDate.Month(), sentinelDate.Day())
			require.NoError(t, err)
			target.Set(reflect.ValueOf(date))
			found.tokens = []string{date.String()}
		case field.Type == reflect.TypeOf(time.Time{}):
			target.Set(reflect.ValueOf(sentinelInstant))
			found.tokens = []string{
				sentinelInstant.Format(time.RFC3339),
				sentinelInstant.Format(time.DateOnly),
				fmt.Sprint(sentinelInstant.Unix()),
			}
		default:
			t.Fatalf("Patient.%s is a %s, which this test does not know how to fill — "+
				"teach it, then decide deliberately whether MarshalZerologObject may emit it (FR-046)",
				field.Name, field.Type)
		}

		fields = append(fields, found)
	}

	return fields
}

func renderPatientLogLine(t *testing.T, p Patient) string {
	t.Helper()

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(p).Send()

	return buf.String()
}

// FR-046: every field of the struct, filled with a value that could only have
// come from it, then looked for in the rendered line. A field added without a
// thought about the log stream fails here rather than leaking in production.
func TestNoPatientFieldReachesTheLogStreamExceptItsIdentifiers(t *testing.T) {
	t.Parallel()

	var patient Patient
	fields := fillWithSentinels(t, &patient)
	line := renderPatientLogLine(t, patient)

	emitted := 0
	for _, field := range fields {
		for _, token := range field.tokens {
			if field.loggable {
				continue
			}

			assert.NotContains(t, line, token,
				"Patient.%s reached the log stream; a patient is PHI and "+
					"MarshalZerologObject emits identifiers only (FR-046)", field.name)
		}

		if field.loggable {
			emitted++
			assert.Contains(t, line, field.tokens[0],
				"Patient.%s is an identifier and the log line needs it to be addressable", field.name)
		}
	}

	assert.Equal(t, len(loggablePatientFields), emitted,
		"the allowlist names a field the struct does not have, so the assertions above pass vacuously")
}

// The named half of the same guarantee: the rendered line is exactly these two
// keys.
func TestThePatientLogLineIsExactlyItsTwoIdentifiers(t *testing.T) {
	t.Parallel()

	line := renderPatientLogLine(t, Patient{
		ID:        "pat0000000001",
		OwnerID:   "usr0000000001",
		FirstName: "Amara",
		LastName:  "Okafor",
		Address:   "12 Rowan Street",
	})

	assert.JSONEq(t, `{"id":"pat0000000001","owner_id":"usr0000000001"}`, line)
}

// The fields FR-046 names in so many words, asserted by their values and not
// only by the shape of the line above.
func TestNameBirthDateAndAddressNeverReachTheLogStream(t *testing.T) {
	t.Parallel()

	birthDate, err := domain.NewDate(1990, time.March, 12)
	require.NoError(t, err)

	patient := Patient{
		ID:        "pat0000000002",
		OwnerID:   "usr0000000002",
		FirstName: "Kwame",
		LastName:  "Mensah",
		BirthDate: birthDate,
		Address:   "4 Baobab Close",
	}

	line := renderPatientLogLine(t, patient)

	for _, secret := range []string{"Kwame", "Mensah", "1990-03-12", "Baobab"} {
		assert.NotContains(t, line, secret)
	}
}
