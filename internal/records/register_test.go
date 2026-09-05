package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
)

// T019. SearchFields, Basis and SeedFixtureID are as mandatory as the seven
// published consumers: a kind with no SearchFields is invisible to US8's
// search and a kind with no Basis silently drops US9's "why does this row
// match". Both are refused here, at Register — the composition root panics on
// whatever this returns, so the failure is a boot failure and never a
// request's.
func TestARegistrationMissingSearchFieldsBasisOrSeedFixtureIDIsRefused(t *testing.T) {
	t.Parallel()

	for name, remove := range map[string]func(*records.Registration){
		"search fields":   func(r *records.Registration) { r.SearchFields = nil },
		"basis":           func(r *records.Registration) { r.Basis = nil },
		"seed fixture id": func(r *records.Registration) { r.SeedFixtureID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
			remove(&registration)

			registry := records.NewRegistry()
			err := registry.Register(registration)

			require.Errorf(t, err, "the registration was accepted with %s unwired", name)
			assert.Containsf(t, err.Error(), name,
				"the refusal does not name what is missing, so nobody reading it knows what to add")
			assert.Empty(t, registry.Kinds(), "a refused registration left the kind half-registered")
		})
	}
}

// A complete registration carries all three, and Register accepts it — the
// control that keeps the test above from passing vacuously.
func TestACompleteRegistrationCarriesSearchFieldsBasisAndSeedFixtureID(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))

	entry, found := registry.FromKind(kind.Medication)
	require.True(t, found)

	require.NotNil(t, entry.SearchFields)
	require.NotNil(t, entry.Basis)
	assert.NotEmpty(t, entry.SeedFixtureID)

	title, body := entry.SearchFields(&recordstest.Detail{Name: "n", Note: "note"})
	assert.Equal(t, "n", title)
	assert.Equal(t, "note", body)

	assert.Nil(t, entry.Basis(&recordstest.Detail{}, records.Criteria{}))
}
