package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/testsupport"
)

// T144, FR-027. Every problem in ONE response, each attached to its field and
// carrying the machine code a client branches on — not the first problem, and
// not a message a person has to read to find out which control to fix.
//
// The named case is US1-4: an end date earlier than the start date AND a blank
// name produce two entries. This asserts four, because a type that holds a
// single field makes the requirement unreachable however the handler is
// written, and two is close enough to one to pass by accident.

// fourViolationsAtOnce breaks four different rules, chosen so that no two share
// a code: a required member, two vocabularies and the cross-field date rule.
const fourViolationsAtOnce = `{
  "name": "   ",
  "type": "homeopathic",
  "started_on": "2025-06-10",
  "ended_on": "2025-06-01",
  "status": "discontinued"
}`

func TestARequestBreakingFourRulesReportsAllFourAtOnce(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.post(collectionURL(), fourViolationsAtOnce)
	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	envelope := answer.envelope(t)

	assert.Equal(t, domain.CodeValidationFailed, envelope.Error.Code)
	assert.NotEmpty(t, envelope.Error.RequestID, "a refusal with no request id correlates to nothing (FR-054)")

	assert.Equal(t, [][2]string{
		{"name", domain.CodeRequired},
		{"type", domain.CodeInvalidValue},
		{"ended_on", clinical.CodeEndBeforeStart},
		{"status", domain.CodeInvalidValue},
	}, envelope.fieldCodes(),
		"the refusal is not every violation, in the order the form renders the fields")
}

// TestUS1_4 is the acceptance scenario named in contracts/records.md, kept
// separate from the four-rule case so that a change to one cannot quietly
// remove the other.
func TestABlankNameAndAnEndBeforeTheStartAreTwoEntriesInOneResponse(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.post(collectionURL(),
		`{"name":"","dosage":"500 mg","started_on":"2025-06-10","ended_on":"2025-06-01"}`)
	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	assert.Equal(t, [][2]string{
		{"name", domain.CodeRequired},
		{"ended_on", clinical.CodeEndBeforeStart},
	}, answer.envelope(t).fieldCodes())
}

// TestNoRefusalCarriesWhatWasTyped is constitution VII at the boundary the
// refusals leave through. Every message here reaches the response, the one log
// stream and Sentry, and every field this application has is medical data.
func TestNoRefusalCarriesWhatWasTyped(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	// A drug name, a reason and a note, each of which discloses a condition.
	secret := "Methotrexate"
	reason := "rheumatoid arthritis"
	note := strings.Repeat("private-", 800)

	answer := caller.post(collectionURL(), `{
	  "name": "`+secret+`",
	  "indication": "`+reason+`",
	  "notes": "`+note+`",
	  "status": "discontinued"
	}`)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	for _, disclosed := range []string{secret, reason, "private-"} {
		assert.NotContainsf(t, answer.Body, disclosed,
			"the refusal echoes %q, which is patient data and which this message also writes to the log", disclosed)
	}
}

// TestEveryFreeTextMaximumIsReportedAgainstItsOwnField is FR-017's half: a
// refusal that named the wrong field would send the person to the wrong control.
func TestEveryFreeTextMaximumIsReportedAgainstItsOwnField(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	// Longer than every documented maximum, so one body trips all of them.
	tooLong := strings.Repeat("x", 5001)

	answer := caller.post(collectionURL(), `{
	  "name": "`+tooLong+`",
	  "alternative_name": "`+tooLong+`",
	  "dosage": "`+tooLong+`",
	  "frequency": "`+tooLong+`",
	  "indication": "`+tooLong+`",
	  "side_effects": "`+tooLong+`",
	  "notes": "`+tooLong+`"
	}`)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	assert.Equal(t, [][2]string{
		{"name", domain.CodeTooLong},
		{"alternative_name", domain.CodeTooLong},
		{"dosage", domain.CodeTooLong},
		{"frequency", domain.CodeTooLong},
		{"indication", domain.CodeTooLong},
		{"side_effects", domain.CodeTooLong},
		{"notes", domain.CodeTooLong},
	}, answer.envelope(t).fieldCodes())
}

// TestBothMalformedDatesAreReportedTogether is the calendar half. FR-018's
// "must be a real calendar date" cannot be a domain rule — a domain.Date has no
// representation for 30 February — so it is refused where the submitted text is
// read, and both dates are read before either is refused.
func TestBothMalformedDatesAreReportedTogether(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.post(collectionURL(), `{"name":"Amoxicillin","started_on":"2025-02-30","ended_on":"01/03/2025"}`)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Equal(t, [][2]string{
		{"started_on", domain.CodeInvalidDate},
		{"ended_on", domain.CodeInvalidDate},
	}, answer.envelope(t).fieldCodes())

	assert.NotContains(t, answer.Body, "2025-02-30", "the refusal echoes the submitted date")
}

// TestAChangeReportsEveryViolationToo is the same requirement on the other
// write. The rules run against the medication as it WOULD be after the patch,
// not against the patch: "the end date is before the start date" is a property
// of neither half alone.
func TestAChangeReportsEveryViolationToo(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	existing := caller.get(recordURL(testsupport.SingleDayMedicationID))
	require.Equal(t, http.StatusOK, existing.Status, existing.Body)

	answer := caller.patch(recordURL(testsupport.SingleDayMedicationID),
		`{"name":"","type":"homeopathic","ended_on":"2020-01-01"}`, existing.etag(t))

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	assert.Equal(t, [][2]string{
		{"name", domain.CodeRequired},
		{"type", domain.CodeInvalidValue},
		{"ended_on", clinical.CodeEndBeforeStart},
	}, answer.envelope(t).fieldCodes(),
		"an end date patched behind the STORED start date is the case a patch-only check cannot see")
}
