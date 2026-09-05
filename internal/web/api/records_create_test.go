package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/views/ids"
)

// T143, FR-015. A create answers 201 with the address of what it made and the
// version to change it with, the representation it answers is the one that was
// stored, and a name alone is sufficient.

// everyField is a create that fills in all twelve recorded members, so that the
// comparison against stored data has something to disagree about in each.
const everyField = `{
  "patient": "` + testsupport.AccountAPatientSelfID + `",
  "name": "Amoxicillin",
  "alternative_name": "Amoxil",
  "type": "prescription",
  "dosage": "500 mg",
  "frequency": "three times daily",
  "route": "oral",
  "indication": "chest infection",
  "started_on": "2025-02-01",
  "ended_on": "2025-02-08",
  "status": "completed",
  "side_effects": "mild nausea",
  "notes": "finish the course"
}`

func TestACreateAnswersTheAddressAndTheVersionOfWhatItMade(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.post(collectionURL(), everyField)
	require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

	created := answer.medication(t)
	require.NotEmpty(t, created.ID)

	assert.Equal(t, recordURL(created.ID), answer.Header.Get("Location"),
		"the Location is not the address this instance serves the record at")

	// The ETag is on the create, not only on the read. Without it the first
	// change to a record somebody just made would need a second request for a
	// version it was already entitled to (FR-026).
	assert.Equal(t, web.ETag(storedVersion(t, caller, created.ID)), answer.Header.Get("ETag"))

	// The address answers, with the same representation. A Location pointing
	// at something that 404s is worse than none.
	followed := caller.get(answer.Header.Get("Location"))
	require.Equal(t, http.StatusOK, followed.Status, followed.Body)
	assert.Equal(t, created, followed.medication(t))
}

func TestTheCreatedRepresentationIsWhatWasStored(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.post(collectionURL(), everyField)
	require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

	created := answer.medication(t)

	record, err := caller.stored(created.ID)
	require.NoError(t, err, "the create answered 201 for a record that is not in the database")

	stored, err := storeMedication(record)
	require.NoError(t, err)

	assert.Equal(t, stored.Name, created.Name)
	assert.Equal(t, stored.AlternativeName, created.AlternativeName)
	assert.Equal(t, string(stored.Type), created.Type)
	assert.Equal(t, stored.Dosage, created.Dosage)
	assert.Equal(t, stored.Frequency, created.Frequency)
	assert.Equal(t, string(stored.Route), created.Route)
	assert.Equal(t, stored.Indication, created.Indication)
	assert.Equal(t, stored.StartedOn.String(), derefOr(created.StartedOn))
	assert.Equal(t, stored.EndedOn.String(), derefOr(created.EndedOn))
	assert.Equal(t, string(stored.Status), created.Status)
	assert.Equal(t, stored.SideEffects, created.SideEffects)
	assert.Equal(t, stored.Notes, created.Notes)

	// FR-032, and the one that matters: the row is filed against the patient
	// the body named, and that patient belongs to the account that sent it.
	assert.Equal(t, testsupport.AccountAPatientSelfID, record.GetString("patient"))
	assert.Equal(t, kind.Medication.Enum(), created.Kind)
}

func TestANameAloneIsSufficientAndEverythingElseIsOptional(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.post(collectionURL(), `{"patient":"`+testsupport.AccountAPatientSelfID+`","name":"Paracetamol"}`)
	require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

	created := answer.medication(t)

	// data-model §2's default, applied before validation so the entity the
	// rules ran against is the entity that was stored.
	assert.Equal(t, string(clinical.TherapyStatusActive), created.Status)

	// FR-024 on the way out: what was never filled in is absent from the
	// response, not present and empty. Only the two dates are explicit nulls.
	assert.NotContains(t, answer.Body, `"dosage"`)
	assert.NotContains(t, answer.Body, `"notes"`)
	assert.Contains(t, answer.Body, `"started_on":null`)
	assert.Contains(t, answer.Body, `"ended_on":null`)
}

func TestACreateWithoutANameIsRefusedAndStoresNothing(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	before := storedCount(t, caller)

	answer := caller.post(collectionURL(), `{"patient":"`+testsupport.AccountAPatientSelfID+`","dosage":"500 mg"}`)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Contains(t, answer.envelope(t).fieldCodes(), [2]string{"name", domain.CodeRequired})
	assert.Equal(t, before, storedCount(t, caller), "a refused create wrote a row")
}

// TestABodyThatIsNotAnObjectIsRefusedWithoutNamingWhatWasSent covers the shapes
// a client gets wrong that are not fields: an empty body, an array, a
// duplicated member. None of the refusals may echo the submission, because the
// submission is medical data and the message reaches the log (research D-28).
func TestABodyThatIsNotAnObjectIsRefusedWithoutNamingWhatWasSent(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	cases := []struct {
		name string
		body string
		leak string
	}{
		{"an empty body", ``, ""},
		{"an array", `[{"name":"Amoxicillin"}]`, "Amoxicillin"},
		{"a name sent twice", `{"name":"Amoxicillin","name":"Ibuprofen"}`, "Ibuprofen"},
		{"a number where a string belongs", `{"name":12345678901234567890}`, "12345678901234567890"},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			answer := caller.post(collectionURL(), one.body)

			require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
			require.NotEmpty(t, answer.envelope(t).Error.Fields, "the refusal names no field")

			if one.leak != "" {
				assert.NotContains(t, answer.Body, one.leak, "the refusal echoes what was submitted")
			}
		})
	}
}

// TestACreateRefusesEveryServerOwnedMember is FR-032 by shape. None of the four
// is a member of the create DTO, so each is refused by the decoder rather than
// by a check somebody could forget to write.
func TestACreateRefusesEveryServerOwnedMember(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	for _, member := range []string{"owner", "id", "created", "updated", "created_at", "updated_at"} {
		t.Run(member, func(t *testing.T) {
			before := storedCount(t, caller)

			answer := caller.post(collectionURL(),
				`{"name":"Amoxicillin","`+member+`":"`+testsupport.AccountBID+`"}`)

			require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
			assert.Contains(t, answer.envelope(t).fieldCodes(), [2]string{member, domain.CodeUnknownField})
			assert.Equal(t, before, storedCount(t, caller), "a refused create wrote a row")
		})
	}
}

func derefOr(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

// TestACreateOverDatastarAnswersHTML mirrors patients_test.go's own: a
// Datastar submit gets the form back as text/html on 200 (the list already
// refreshes through the record stream for this kind), and every other caller
// keeps today's JSON exactly (422/201).
func TestACreateOverDatastarAnswersHTML(t *testing.T) {
	t.Parallel()

	datastar := map[string]string{"Datastar-Request": "true"}

	t.Run("an invalid create over Datastar answers 200 text/html with the form and the field error", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.do(http.MethodPost, collectionURL(),
			`{"patient":"`+testsupport.AccountAPatientSelfID+`"}`, datastar)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, ids.RecordForm(kind.Medication, ""))
		assert.Contains(t, answer.Body, "a name is required")
	})

	t.Run("the same invalid create with no Datastar-Request header still answers 422 JSON", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(collectionURL(), `{"patient":"`+testsupport.AccountAPatientSelfID+`"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, `"code":"`+domain.CodeValidationFailed+`"`)
	})

	t.Run("a valid create over Datastar answers 200 text/html with the blank form", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.do(http.MethodPost, collectionURL(),
			`{"patient":"`+testsupport.AccountAPatientSelfID+`","name":"Datastar Amoxicillin"}`, datastar)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, ids.RecordForm(kind.Medication, ""))
	})

	t.Run("the same valid create with no Datastar-Request header still answers 201 JSON", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(collectionURL(),
			`{"patient":"`+testsupport.AccountAPatientSelfID+`","name":"Plain Amoxicillin"}`)
		assert.Equal(t, http.StatusCreated, answer.Status, answer.Body)
	})
}
