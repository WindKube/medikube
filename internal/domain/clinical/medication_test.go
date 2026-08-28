package clinical

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

// The only two fields the marshaller may emit. Everything else on a Medication
// is either patient data outright or a value that describes a person's therapy,
// and FR-038 keeps all of it out of the operational record. This is an
// allowlist by name, so a field added later is redacted by default.
var loggableMedicationFields = map[string]bool{
	"ID":      true,
	"OwnerID": true,
}

// Recognisable values for the field types a Medication is built from. A date
// and an instant cannot be filled with a sentinel string, so they are filled
// with a sentinel calendar day instead and searched for by their rendering.
var (
	sentinelDate    = time.Date(2031, time.July, 19, 0, 0, 0, 0, time.UTC)
	sentinelInstant = time.Date(2032, time.August, 20, 4, 5, 6, 0, time.UTC)
)

type sentinelField struct {
	name     string
	loggable bool
	// The strings that would appear in the log line if this field reached it.
	tokens []string
}

// fillWithSentinels writes a value into every field of a Medication that could
// only have come from that field, and reports what to search the rendered line
// for. Reflection rather than a list somebody maintains: a field added to the
// struct is covered the moment it exists, which is the whole point.
func fillWithSentinels(t *testing.T, m *Medication) []sentinelField {
	t.Helper()

	value := reflect.ValueOf(m).Elem()
	structure := value.Type()
	require.NotZero(t, structure.NumField(), "Medication has no fields at all")

	fields := make([]sentinelField, 0, structure.NumField())
	for i := range structure.NumField() {
		field := structure.Field(i)
		target := value.Field(i)

		found := sentinelField{name: field.Name, loggable: loggableMedicationFields[field.Name]}

		switch {
		// The trailing Z keeps one sentinel from being a substring of another.
		case target.Kind() == reflect.String:
			sentinel := fmt.Sprintf("SENTINEL%dZ", i)
			target.SetString(sentinel)
			found.tokens = []string{sentinel}
		case field.Type == reflect.TypeOf(domain.Date{}):
			date, err := domain.NewDate(sentinelDate.Year(), sentinelDate.Month(), sentinelDate.Day())
			require.NoError(t, err)
			target.Set(reflect.ValueOf(date))
			found.tokens = []string{date.String(), fmt.Sprint(sentinelDate.Unix())}
		case field.Type == reflect.TypeOf(time.Time{}):
			target.Set(reflect.ValueOf(sentinelInstant))
			found.tokens = []string{
				sentinelInstant.Format(time.RFC3339),
				sentinelInstant.Format(time.DateOnly),
				fmt.Sprint(sentinelInstant.Unix()),
			}
		default:
			t.Fatalf("Medication.%s is a %s, which this test does not know how to fill — "+
				"teach it, then decide deliberately whether MarshalZerologObject may emit it (FR-038)",
				field.Name, field.Type)
		}

		fields = append(fields, found)
	}

	return fields
}

func renderLogLine(t *testing.T, m Medication) string {
	t.Helper()

	var buf bytes.Buffer
	// Log() and Send() so the line carries nothing but the marshaller's own
	// output — no level, no message to read the assertions past.
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(m).Send()

	return buf.String()
}

// FR-038 and SC-005. Not "the fields we remembered to check": every field of
// the struct, filled with a value that could only have come from it, then
// looked for in the rendered line. A field added without a thought about the
// log stream fails here rather than leaking in production.
func TestNoMedicationFieldReachesTheLogStreamExceptItsIdentifiers(t *testing.T) {
	t.Parallel()

	var medication Medication
	fields := fillWithSentinels(t, &medication)
	line := renderLogLine(t, medication)

	emitted := 0
	for _, field := range fields {
		for _, token := range field.tokens {
			if field.loggable {
				continue
			}

			assert.NotContains(t, line, token,
				"Medication.%s reached the log stream; a medication is patient data and "+
					"MarshalZerologObject emits identifiers only (FR-038)", field.name)
		}

		if field.loggable {
			emitted++
			assert.Contains(t, line, field.tokens[0],
				"Medication.%s is an identifier and the log line needs it to be addressable", field.name)
		}
	}

	assert.Equal(t, len(loggableMedicationFields), emitted,
		"the allowlist names a field the struct does not have, so the assertions above pass vacuously")
}

// The named half of the same guarantee: the rendered line is exactly these two
// keys. A marshaller that started emitting a third — however harmless it looked
// to whoever added it — fails here.
func TestTheMedicationLogLineIsExactlyItsTwoIdentifiers(t *testing.T) {
	t.Parallel()

	line := renderLogLine(t, Medication{
		ID:      "med0000000001",
		OwnerID: "usr0000000001",
		Name:    "Levothyroxine",
		Dosage:  "75 mcg",
		Notes:   "take on an empty stomach",
	})

	assert.JSONEq(t, `{"medication_id":"med0000000001","owner_id":"usr0000000001"}`, line)
}

// The three fields FR-038 names in so many words, asserted by their values and
// not only by the shape of the line above.
func TestTheNameTheDoseAndTheNotesNeverReachTheLogStream(t *testing.T) {
	t.Parallel()

	medication := Medication{
		ID:              "med0000000002",
		OwnerID:         "usr0000000002",
		Name:            "Sertraline",
		AlternativeName: "Zoloft",
		Dosage:          "50 mg",
		Frequency:       "once daily",
		Indication:      "major depressive disorder",
		SideEffects:     "nausea in the first week",
		Notes:           "started after the March appointment",
	}

	line := renderLogLine(t, medication)

	for _, secret := range []string{
		"Sertraline", "Zoloft", "50 mg", "once daily",
		"major depressive disorder", "nausea in the first week",
		"started after the March appointment",
	} {
		assert.NotContains(t, line, secret)
	}
}

// data-model §2, "derived, never stored". is_current is a function of status,
// which is why it is a method and not a column somebody has to keep in step.
func TestIsCurrentIsTrueOnlyWhileTheCourseIsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status TherapyStatus
		want   bool
	}{
		{status: TherapyStatusActive, want: true},
		{status: TherapyStatusOnHold, want: false},
		{status: TherapyStatusCompleted, want: false},
		{status: TherapyStatusStopped, want: false},
		{status: TherapyStatusCancelled, want: false},
		{status: "", want: false},
	}

	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, Medication{Status: test.status}.IsCurrent())
		})
	}
}
