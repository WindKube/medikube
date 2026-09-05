package kinds_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/condition"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestConditionSatisfiesTheSharedRecordContracts is T045's proof for
// condition, mirroring the allergy and medication contract test files.
func TestConditionSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.ConditionCreate{Patient: patientID, Diagnosis: "Seasonal allergy", Status: "active"}
		},
		Full: func(patientID string) any {
			onsetOn := "2023-01-05"
			resolvedOn := "2023-01-26"

			return &api.ConditionCreate{
				Patient:    patientID,
				Diagnosis:  "Bacterial pneumonia",
				Status:     "resolved",
				Severity:   "moderate",
				OnsetOn:    &onsetOn,
				ResolvedOn: &resolvedOn,
				ICD10Code:  "J15.9",
				Notes:      "treated with antibiotics",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   conditionEntryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.ConditionPatch{} },
			Sort:       condition.Sorts(),
			HasPrimaryDate: func(body any) bool {
				detail, ok := body.(*api.Condition)
				return ok && detail.OnsetOn != nil
			},
			CascadeSkip: "condition's cascade-on-patient-delete needs a real instance this harness " +
				"cannot reach past records.Service into the store directly to prove; not yet covered " +
				"by an internal/store/condition integration test",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := conditionEntryFor(t, instance)

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
			DefaultSort: []domain.SortKey{condition.Sorts()[0]},
			NoPatientSkip: "condition.Patient is a plain string FR-002 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := conditionSearchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func conditionEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.Condition.Segment())
	require.NoError(t, err)

	return entry
}

func conditionSearchRowFor(t *testing.T, instance *apitest.Instance, k kind.Kind, recordID string) (title string, found bool) {
	t.Helper()

	require.NotNil(t, instance.Search, "the instance has no search repository wired")

	row, found, err := instance.Search.Find(context.Background(), k, recordID)
	require.NoError(t, err)

	return row.Title, found
}
