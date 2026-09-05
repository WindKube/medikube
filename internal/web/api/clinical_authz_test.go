package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
)

// T067's FR-092 matrix for US2's three kinds, at the size the shared
// TestEveryRecordOperationIsOwnerScoped (records_authz_test.go) already proves
// byte-identical refusals at: this file only asks the three questions that
// test does not already ask of every kind through one hardcoded url —
// does the owner reach their own record, is a stranger refused, is nobody
// refused — once per kind, on both a read and a list.
func TestEncounterProcedureTreatmentAreOwnerScoped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind kind.Kind
		id   string
	}{
		{kind: kind.Encounter, id: seed.EncounterNameOnlyID},
		{kind: kind.Procedure, id: seed.ProcedureNameOnlyID},
		{kind: kind.Treatment, id: seed.TreatmentNameOnlyID},
	}

	for _, test := range tests {
		t.Run(test.kind.Segment(), func(t *testing.T) {
			t.Parallel()

			owner := newCaller(t)
			collection := "/api/v1/records/" + test.kind.Segment()
			record := collection + "/" + test.id

			t.Run("the owner reads their own record", func(t *testing.T) {
				t.Parallel()

				answer := owner.get(record)
				assert.Equal(t, http.StatusOK, answer.Status, answer.Body)
			})

			t.Run("the owner lists their own patient", func(t *testing.T) {
				t.Parallel()

				answer := owner.get(collection + "?patient=" + testsupport.AccountAPatientSelfID)
				assert.Equal(t, http.StatusOK, answer.Status, answer.Body)
			})

			t.Run("a signed-in stranger is refused with a miss", func(t *testing.T) {
				t.Parallel()

				answer := owner.as(testsupport.AccountBEmail).get(record)
				assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
			})

			t.Run("nobody is refused before any record is looked up", func(t *testing.T) {
				t.Parallel()

				answer := owner.anonymous().get(record)
				assert.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
			})
		})
	}
}

// One create-then-read round trip per kind, proving the DTO carries what was
// submitted all the way back out — T065's DTO round-trip, at the size a kind
// already proven against recordstest's shared contracts
// (internal/web/api/records_contract_test.go) still needs by hand: the
// literal request body a person's browser would send.
func TestEncounterProcedureTreatmentCreateAnswersWhatWasSubmitted(t *testing.T) {
	t.Parallel()

	patient := testsupport.AccountAPatientSelfID

	tests := []struct {
		kind kind.Kind
		body string
		want string
	}{
		{
			kind: kind.Encounter,
			body: `{"patient":"` + patient + `","reason":"Twisted ankle","occurred_on":"2026-05-01"}`,
			want: `"reason":"Twisted ankle"`,
		},
		{
			kind: kind.Procedure,
			body: `{"patient":"` + patient + `","name":"X-ray","occurred_on":"2026-05-01","status":"completed"}`,
			want: `"name":"X-ray"`,
		},
		{
			kind: kind.Treatment,
			body: `{"patient":"` + patient + `","name":"Splinting","started_on":"2026-05-01"}`,
			want: `"name":"Splinting"`,
		},
	}

	for _, test := range tests {
		t.Run(test.kind.Segment(), func(t *testing.T) {
			t.Parallel()

			owner := newCaller(t)
			collection := "/api/v1/records/" + test.kind.Segment()

			created := owner.post(collection, test.body)
			require.Equal(t, http.StatusCreated, created.Status, created.Body)
			assert.Contains(t, created.Body, test.want)

			var location struct {
				ID string `json:"id"`
			}
			require.NoError(t, json.Unmarshal([]byte(created.Body), &location))

			read := owner.get(collection + "/" + location.ID)
			require.Equal(t, http.StatusOK, read.Status, read.Body)
			assert.Contains(t, read.Body, test.want)
		})
	}
}
