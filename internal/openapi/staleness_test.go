package openapi_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/openapi"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// openAPIDocumentVersion must be kept equal to cmd/medikube/cli.go's own
// constant of the same name: this test is what regenerates the document the
// exact way `medikube openapi` does, and a mismatch here would fail this test
// for a reason that has nothing to do with staleness.
const openAPIDocumentVersion = "0.1.0"

// productionInput mirrors cmd/medikube's openAPIInput exactly — the real
// route inventory and the one shipped kind's DTOs — because this is FR-064's
// gate: regenerating the document CI's way must produce no diff against the
// committed api/openapi.json, and a fixture input would prove that of a
// document nobody ships.
func productionInput() openapi.Input {
	return openapi.Input{
		Version: openAPIDocumentVersion,
		Routes:  httproute.Inventory().Routes(),
		Kinds: []openapi.Kind{
			{
				Enum:    kind.Medication.Enum(),
				Segment: kind.Medication.Segment(),
				Summary: api.MedicationSchema().NewSummary(),
				Detail:  api.MedicationSchema().NewDetail(),
				Create:  api.MedicationSchema().NewCreate(),
				Patch:   api.MedicationSchema().NewPatch(),
			},
			{
				Enum:    kind.Allergy.Enum(),
				Segment: kind.Allergy.Segment(),
				Summary: api.AllergySchema().NewSummary(),
				Detail:  api.AllergySchema().NewDetail(),
				Create:  api.AllergySchema().NewCreate(),
				Patch:   api.AllergySchema().NewPatch(),
			},
			{
				Enum:    kind.Condition.Enum(),
				Segment: kind.Condition.Segment(),
				Summary: api.ConditionSchema().NewSummary(),
				Detail:  api.ConditionSchema().NewDetail(),
				Create:  api.ConditionSchema().NewCreate(),
				Patch:   api.ConditionSchema().NewPatch(),
			},
			{
				Enum:    kind.EmergencyContact.Enum(),
				Segment: kind.EmergencyContact.Segment(),
				Summary: api.EmergencyContactSchema().NewSummary(),
				Detail:  api.EmergencyContactSchema().NewDetail(),
				Create:  api.EmergencyContactSchema().NewCreate(),
				Patch:   api.EmergencyContactSchema().NewPatch(),
			},
			{
				Enum:    kind.Immunization.Enum(),
				Segment: kind.Immunization.Segment(),
				Summary: api.ImmunizationSchema().NewSummary(),
				Detail:  api.ImmunizationSchema().NewDetail(),
				Create:  api.ImmunizationSchema().NewCreate(),
				Patch:   api.ImmunizationSchema().NewPatch(),
			},
			{
				Enum:    kind.Injury.Enum(),
				Segment: kind.Injury.Segment(),
				Summary: api.InjurySchema().NewSummary(),
				Detail:  api.InjurySchema().NewDetail(),
				Create:  api.InjurySchema().NewCreate(),
				Patch:   api.InjurySchema().NewPatch(),
			},
			{
				Enum:    kind.Insurance.Enum(),
				Segment: kind.Insurance.Segment(),
				Summary: api.InsuranceSchema().NewSummary(),
				Detail:  api.InsuranceSchema().NewDetail(),
				Create:  api.InsuranceSchema().NewCreate(),
				Patch:   api.InsuranceSchema().NewPatch(),
			},
			{
				Enum:    kind.Equipment.Enum(),
				Segment: kind.Equipment.Segment(),
				Summary: api.EquipmentSchema().NewSummary(),
				Detail:  api.EquipmentSchema().NewDetail(),
				Create:  api.EquipmentSchema().NewCreate(),
				Patch:   api.EquipmentSchema().NewPatch(),
			},
			{
				Enum:    kind.Encounter.Enum(),
				Segment: kind.Encounter.Segment(),
				Summary: api.EncounterSchema().NewSummary(),
				Detail:  api.EncounterSchema().NewDetail(),
				Create:  api.EncounterSchema().NewCreate(),
				Patch:   api.EncounterSchema().NewPatch(),
			},
			{
				Enum:    kind.Procedure.Enum(),
				Segment: kind.Procedure.Segment(),
				Summary: api.ProcedureSchema().NewSummary(),
				Detail:  api.ProcedureSchema().NewDetail(),
				Create:  api.ProcedureSchema().NewCreate(),
				Patch:   api.ProcedureSchema().NewPatch(),
			},
			{
				Enum:    kind.Treatment.Enum(),
				Segment: kind.Treatment.Segment(),
				Summary: api.TreatmentSchema().NewSummary(),
				Detail:  api.TreatmentSchema().NewDetail(),
				Create:  api.TreatmentSchema().NewCreate(),
				Patch:   api.TreatmentSchema().NewPatch(),
			},
		},
		Envelope: web.Envelope{},
	}
}

// repoRoot walks up from this test file to the module root, the same way
// internal/architecture's tests locate api/openapi.json without a hardcoded
// absolute path.
func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller could not resolve this test file's own path")

	return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestRegeneratingTheDocumentProducesNoDiff is the gate `task openapi:check`
// and the openapi-diff CI job exist to run. If someone changes a route
// without regenerating api/openapi.json, this fails for exactly that reason
// (FR-064).
func TestRegeneratingTheDocumentProducesNoDiff(t *testing.T) {
	document, err := openapi.Generate(productionInput())
	require.NoError(t, err)

	loaded, _, err := openapi.RoundTrip(t.Context(), document)
	require.NoError(t, err)

	regenerated, err := openapi.Marshal(loaded)
	require.NoError(t, err)

	committed, err := os.ReadFile(filepath.Join(repoRoot(t), "api", "openapi.json"))
	require.NoError(t, err, "api/openapi.json is not committed; run `task openapi`")

	assert.Equal(t, string(committed), string(regenerated),
		"api/openapi.json is stale: run `task openapi` and commit the result")
}
