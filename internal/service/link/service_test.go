package link_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/link"
)

type fakeResolver struct {
	byID map[string]clinical.PatientRef
}

func (f fakeResolver) Resolve(_ context.Context, _ kind.Kind, ids []string) ([]clinical.PatientRef, error) {
	refs := make([]clinical.PatientRef, 0, len(ids))
	for _, id := range ids {
		ref, found := f.byID[id]
		if !found {
			ref = clinical.PatientRef{ID: id, Found: false}
		}

		refs = append(refs, ref)
	}

	return refs, nil
}

type fakeAuthorizer struct {
	denied map[string]bool
	calls  []string
}

func (f *fakeAuthorizer) Record(
	_ context.Context, _ access.Actor, _ kind.Kind, id string, _ access.Permission,
) (access.Grant, error) {
	f.calls = append(f.calls, id)

	if f.denied[id] {
		return access.Grant{}, nil
	}

	return access.Grant{Level: access.PermOwn}, nil
}

const subjectPatient = "patient1"

func TestValidateSetIsAReplaceSet(t *testing.T) {
	t.Parallel()

	resolver := fakeResolver{byID: map[string]clinical.PatientRef{
		"m1": {ID: "m1", PatientID: subjectPatient, Found: true},
		"m2": {ID: "m2", PatientID: subjectPatient, Found: true},
	}}
	authorizer := &fakeAuthorizer{}

	ids, err := link.ValidateSet(t.Context(), resolver, authorizer, access.Actor{}, subjectPatient, kind.Medication, []string{"m1", "m2", "m1"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"m1", "m2"}, ids, "a repeated id is one member")
}

func TestValidateSetReAddingAnExistingMemberIsIdempotent(t *testing.T) {
	t.Parallel()

	resolver := fakeResolver{byID: map[string]clinical.PatientRef{
		"m1": {ID: "m1", PatientID: subjectPatient, Found: true},
	}}
	authorizer := &fakeAuthorizer{}

	first, err := link.ValidateSet(t.Context(), resolver, authorizer, access.Actor{}, subjectPatient, kind.Medication, []string{"m1"})
	require.NoError(t, err)

	second, err := link.ValidateSet(t.Context(), resolver, authorizer, access.Actor{}, subjectPatient, kind.Medication, []string{"m1", "m1"})
	require.NoError(t, err)

	assert.ElementsMatch(t, first, second, "re-adding an existing member is a no-op, not an error")
}

func TestValidateSetRefusalsAreByteIdentical(t *testing.T) {
	t.Parallel()

	resolver := fakeResolver{byID: map[string]clinical.PatientRef{
		"crossPatient": {ID: "crossPatient", PatientID: "patient2", Found: true},
		"unreachable":  {ID: "unreachable", PatientID: subjectPatient, Found: false},
	}}
	authorizer := &fakeAuthorizer{}

	cases := []struct {
		name string
		id   string
	}{
		{"cross-patient member", "crossPatient"},
		{"non-existent member", "doesNotExist"},
		{"unreachable member", "unreachable"},
	}

	var refusals []error

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := link.ValidateSet(t.Context(), resolver, authorizer, access.Actor{}, subjectPatient, kind.Medication, []string{tt.id})
			require.ErrorIs(t, err, domain.ErrNotFound)
		})

		_, err := link.ValidateSet(context.Background(), resolver, authorizer, access.Actor{}, subjectPatient, kind.Medication, []string{tt.id})
		refusals = append(refusals, err)
	}

	for _, err := range refusals {
		assert.Equal(t, refusals[0].Error(), err.Error(), "every refusal must be byte-identical (FR-057, SC-004)")
	}
}

func TestValidateSetChecksTheAuthorizerOnEveryTarget(t *testing.T) {
	t.Parallel()

	resolver := fakeResolver{byID: map[string]clinical.PatientRef{
		"m1": {ID: "m1", PatientID: subjectPatient, Found: true},
		"m2": {ID: "m2", PatientID: subjectPatient, Found: true},
	}}
	authorizer := &fakeAuthorizer{denied: map[string]bool{"m2": true}}

	_, err := link.ValidateSet(t.Context(), resolver, authorizer, access.Actor{}, subjectPatient, kind.Medication, []string{"m1", "m2"})
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Contains(t, authorizer.calls, "m1")
	assert.Contains(t, authorizer.calls, "m2")
}

func TestValidateSetOfNoIDsIsANoOp(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}

	ids, err := link.ValidateSet(t.Context(), fakeResolver{}, authorizer, access.Actor{}, subjectPatient, kind.Medication, nil)
	require.NoError(t, err)
	assert.Empty(t, ids)
	assert.Empty(t, authorizer.calls)
}
