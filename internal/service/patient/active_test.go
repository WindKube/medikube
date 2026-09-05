package patient_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
)

// FR-020: the target is authorized before the pointer is written, and the
// write is audited as switch_patient.
func TestSetActivePatientAuthorizesBeforeWriting(t *testing.T) {
	t.Parallel()

	svc, _, auditor := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	active, err := svc.SetActivePatient(t.Context(), owner(), &created.ID)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, created.ID, active.ID)

	resolved, err := svc.ResolveActivePatient(t.Context(), owner())
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, created.ID, resolved.ID)

	events := auditor.Events()
	require.Len(t, events, 1)
	assert.Equal(t, audit.ActionSwitchPatient, events[0].Action)
	assert.Equal(t, audit.TargetKindPatient, events[0].TargetKind)
	assert.Equal(t, created.ID, events[0].TargetID)
	assert.Equal(t, created.ID, events[0].PatientID)
}

// FR-020, US3-5: another account's patient is refused, and the pointer is
// left unchanged.
func TestSetActivePatientRefusesAnotherAccountsPatient(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	_, err = svc.SetActivePatient(t.Context(), owner(), &created.ID)
	require.NoError(t, err)

	_, err = svc.SetActivePatient(t.Context(), stranger(), &created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	resolved, err := svc.ResolveActivePatient(t.Context(), owner())
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, created.ID, resolved.ID, "the stranger's refused attempt must not have touched the owner's pointer")

	strangerPointer, err := svc.ResolveActivePatient(t.Context(), stranger())
	require.NoError(t, err)
	assert.Nil(t, strangerPointer)
}

// FR-017: a pointer at a patient the actor cannot reach resolves to null.
func TestResolveActivePatientNullsAnUnreachablePointer(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	_, err = svc.SetActivePatient(t.Context(), owner(), &created.ID)
	require.NoError(t, err)

	// The stranger owns nothing, so its own pointer stays null no matter
	// what the owner's names.
	resolved, err := svc.ResolveActivePatient(t.Context(), stranger())
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

// FR-018, US3-4: exactly one reachable patient is auto-selected and the
// selection is persisted, so a second resolve finds the same one without a
// second choice to make.
func TestResolveActivePatientAutoSelectsExactlyOne(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	resolved, err := svc.ResolveActivePatient(t.Context(), owner())
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, created.ID, resolved.ID)

	again, err := svc.ResolveActivePatient(t.Context(), owner())
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, created.ID, again.ID)
}

// FR-018: two or more reachable patients leave the pointer null.
func TestResolveActivePatientStaysNullWithSeveralPatients(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	_, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	second := personDraft()
	second.FirstName = "Chidinma"
	_, err = svc.Create(t.Context(), owner(), second)
	require.NoError(t, err)

	resolved, err := svc.ResolveActivePatient(t.Context(), owner())
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

// FR-013: clearing the pointer with a nil patient answers no active patient.
func TestSetActivePatientClearsWithNil(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	second := personDraft()
	second.FirstName = "Chidinma"
	_, err = svc.Create(t.Context(), owner(), second)
	require.NoError(t, err)

	_, err = svc.SetActivePatient(t.Context(), owner(), &created.ID)
	require.NoError(t, err)

	cleared, err := svc.SetActivePatient(t.Context(), owner(), nil)
	require.NoError(t, err)
	assert.Nil(t, cleared)

	// Two patients are reachable, so this is not auto-selection's job to
	// mask the clear.
	resolved, err := svc.ResolveActivePatient(t.Context(), owner())
	require.NoError(t, err)
	assert.Nil(t, resolved)
}
