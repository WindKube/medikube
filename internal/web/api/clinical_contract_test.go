package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/encounter"
	"medikube/internal/service/procedure"
	"medikube/internal/service/treatment"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestEncounterProcedureTreatmentSatisfyTheSharedRecordContracts is T064 and
// T068 for US2's three kinds: the same shared repositorycontract and
// kindcontract suites TestMedicationSatisfiesTheSharedRecordContracts runs
// (records_contract_test.go), run once per kind, against a fully wired
// instance rather than a fake standing in for one.
//
// Each kind's own Patient member is a plain, non-pointer string the same way
// medication's is (FR-002 requires it structurally), so NoPatientSkip carries
// the identical reason; each kind's own cascade-on-patient-delete is likewise
// left to an internal/store integration test, for the identical reason
// CascadeSkip does on medication's own entry.
func TestEncounterProcedureTreatmentSatisfyTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	t.Run(kind.Encounter.Segment(), func(t *testing.T) {
		t.Parallel()

		runClinicalContract(t, kind.Encounter, encounter.Sorts(), recordstest.Fixture{
			Minimal: func(patientID string) any {
				occurredOn := "2026-01-10"
				return &api.EncounterCreate{Patient: patientID, Reason: "Annual check-up", OccurredOn: &occurredOn}
			},
			Full: func(patientID string) any {
				occurredOn := "2026-01-10"

				return &api.EncounterCreate{
					Patient: patientID, Reason: "Follow-up", OccurredOn: &occurredOn,
					VisitType: "office", Priority: "routine", Assessment: "stable", Notes: "none",
				}
			},
		}, func() any { return &api.EncounterPatch{} })
	})

	t.Run(kind.Procedure.Segment(), func(t *testing.T) {
		t.Parallel()

		runClinicalContract(t, kind.Procedure, procedure.Sorts(), recordstest.Fixture{
			Minimal: func(patientID string) any {
				occurredOn := "2026-01-10"

				return &api.ProcedureCreate{
					Patient: patientID, Name: "Skin biopsy", OccurredOn: &occurredOn, Status: "completed",
				}
			},
			Full: func(patientID string) any {
				occurredOn := "2026-01-10"

				return &api.ProcedureCreate{
					Patient: patientID, Name: "Colonoscopy", OccurredOn: &occurredOn,
					Status: "completed", Type: "diagnostic", Notes: "none",
				}
			},
		}, func() any { return &api.ProcedurePatch{} })
	})

	t.Run(kind.Treatment.Segment(), func(t *testing.T) {
		t.Parallel()

		runClinicalContract(t, kind.Treatment, treatment.Sorts(), recordstest.Fixture{
			Minimal: func(patientID string) any {
				startedOn := "2026-01-10"
				return &api.TreatmentCreate{Patient: patientID, Name: "Physical therapy", StartedOn: &startedOn}
			},
			Full: func(patientID string) any {
				startedOn := "2026-01-10"

				return &api.TreatmentCreate{
					Patient: patientID, Name: "Cardiac rehabilitation", StartedOn: &startedOn,
					Setting: "outpatient", Notes: "none",
				}
			},
		}, func() any { return &api.TreatmentPatch{} })
	})
}

func runClinicalContract(
	t *testing.T, k kind.Kind, sort []domain.SortKey, fixture recordstest.Fixture, newPatch func() any,
) {
	t.Helper()

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   clinicalEntryFor(t, instance, k).Service,
			Owner:     access.Actor{UserID: testsupport.AccountAID, RequestID: "req-1"},
			PatientID: testsupport.AccountAPatientChildID,
			Stranger:  access.Actor{UserID: testsupport.AccountBID, RequestID: "req-2"},
		}
	}

	t.Run("RepositoryContract", func(t *testing.T) {
		t.Parallel()

		recordstest.RunRepositoryContract(t, recordstest.RepositoryContractOptions{
			NewHarness: newHarness,
			Fixture:    fixture,
			NewPatch:   newPatch,
			Sort:       sort,
			NullPrimaryDateSkip: k.Segment() + "'s primary date is a required field (FR-025/FR-026 et al.), " +
				"so Fixture.Minimal cannot legally omit it",
			CascadeSkip: k.Segment() + "'s cascade-on-patient-delete is proven against a real instance " +
				"by an internal/store integration test; this harness has no patient to delete without " +
				"reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := clinicalEntryFor(t, instance, k)

		recordstest.RunKindContract(t, recordstest.KindContractOptions{
			NewHarness: func(t *testing.T) recordstest.RepositoryHarness {
				t.Helper()

				return recordstest.RepositoryHarness{
					Service:   entry.Service,
					Owner:     access.Actor{UserID: testsupport.AccountAID, RequestID: "req-1"},
					PatientID: testsupport.AccountAPatientChildID,
					Stranger:  access.Actor{UserID: testsupport.AccountBID, RequestID: "req-2"},
				}
			},
			Entry:       entry,
			Fixture:     fixture,
			DefaultSort: sort[0],
			NoPatientSkip: k.Segment() + ".Patient is a plain string FR-002 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := searchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func clinicalEntryFor(t *testing.T, instance *apitest.Instance, k kind.Kind) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(k.Segment())
	require.NoError(t, err)

	return entry
}
