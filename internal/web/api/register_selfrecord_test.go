package api_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/person"
	"medikube/internal/store"
)

// T053, FR-005, US1-1. Registration provisions exactly one patient for the new
// account, marked as theirs and as themself — the guarantee patients.md's
// "never empty in practice" line and person.RelationshipToOwnerSelf both rest
// on, and the one the account's own row carries no member for.
func TestRegisteringProvisionsExactlyOneSelfRecordPatient(t *testing.T) {
	t.Parallel()

	instance := openRig(t)

	answer := instance.anonymous().post(registerURL, body(
		"email", quoted(newAccountEmail),
		"name", quoted(newAccountName),
		"password", quoted(newAccountPassword),
	))

	require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

	session := answer.session(t)

	built, err := store.PatientsSchema().Build(store.Query{
		Conditions: []store.Condition{store.Equal(store.PatientOwner, session.User.ID)},
	})
	require.NoError(t, err)

	var patients []*core.Record
	require.NoError(t, built.Apply(instance.instance.App.RecordQuery(store.PatientCollection)).All(&patients))

	require.Len(t, patients, 1, "registration must provision exactly one patient")

	provisioned, err := store.PatientFromRecord(patients[0])
	require.NoError(t, err)

	assert.Equal(t, session.User.ID, provisioned.OwnerID)
	assert.True(t, provisioned.IsSelfRecord)
	assert.Equal(t, person.RelationshipSelf, provisioned.RelationshipToOwner)
	assert.Equal(t, "Dara", provisioned.FirstName)
	assert.Equal(t, "Ferreira", provisioned.LastName)
}
