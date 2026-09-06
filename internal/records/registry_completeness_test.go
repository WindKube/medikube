package records_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/web/api"
)

// expectedKinds is every kind this build's registry is expected to carry a
// complete registration for. It is the one list phase 003's stories extend as
// they register a kind, and it is deliberately not `kind.Kinds()`: the kind
// table declares fourteen spellings before any of the thirteen new ones has a
// registration to go with it, and this test has to fail loudly for the gap
// rather than pass by asserting nothing.
var expectedKinds = []kind.Kind{
	kind.Medication,
	kind.Allergy,
	kind.Condition,
	kind.EmergencyContact,
	kind.Immunization,
	kind.Injury,
	kind.Insurance,
	kind.Equipment,
	kind.Symptom,
	kind.Vitals,
	kind.Encounter,
	kind.Procedure,
	kind.Treatment,
	kind.FamilyMember,
}

// TestEveryExpectedKindHasACompleteRegistration is T021: it walks
// expectedKinds against a real, composed registry and fails when any of them
// lacks any piece a registration owes — the same seven consumers T104 checks
// one registration at a time, plus the three that are just as mandatory
// (T019) — so that a kind added to expectedKinds with no registration to match
// fails here rather than shipping a route nobody can reach.
func TestEveryExpectedKindHasACompleteRegistration(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Allergy, audit.TargetKindAllergy)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Condition, audit.TargetKindCondition)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.EmergencyContact, audit.TargetKindEmergencyContact)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Immunization, audit.TargetKindImmunization)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Injury, audit.TargetKindInjury)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Insurance, audit.TargetKindInsurance)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Equipment, audit.TargetKindEquipment)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Symptom, audit.TargetKindSymptom)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Vitals, audit.TargetKindVitals)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Encounter, audit.TargetKindEncounter)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Procedure, audit.TargetKindProcedure)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Treatment, audit.TargetKindTreatment)))
	require.NoError(t, registry.Register(recordstest.Registration(kind.FamilyMember, audit.TargetKindFamilyMember)))

	require.NotEmpty(t, expectedKinds, "the expectation list itself is empty, so this test asserts nothing")

	for _, k := range expectedKinds {
		t.Run(string(k), func(t *testing.T) {
			t.Parallel()

			entry, registered := registry.FromKind(k)
			require.Truef(t, registered, "%s is expected but not registered on this build's registry", k)

			recordstest.AssertRegistrationComplete(t, entry)
		})
	}
}

func TestEveryExpectedKindIsDocumentedInTheOpenAPIInput(t *testing.T) {
	t.Parallel()

	documented := map[string]bool{}
	for _, k := range api.OpenAPIKinds() {
		documented[k.Enum] = true
	}

	for _, k := range expectedKinds {
		require.True(t, documented[k.Enum()], "%s has no OpenAPI DTO set", k)
	}
}
