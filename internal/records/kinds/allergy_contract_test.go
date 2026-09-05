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
	"medikube/internal/service/allergy"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestAllergySatisfiesTheSharedRecordContracts is T045's proof for allergy,
// mirroring internal/web/api/records_contract_test.go's own for medication:
// the shared repositorycontract and kindcontract suites, run through a fully
// wired instance rather than against a fake standing in for one.
func TestAllergySatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.AllergyCreate{Patient: patientID, Allergen: "Penicillin", Severity: "mild"}
		},
		Full: func(patientID string) any {
			onsetOn := "2024-01-01"

			return &api.AllergyCreate{
				Patient:  patientID,
				Allergen: "Penicillin",
				Reaction: "hives",
				Severity: "severe",
				Status:   "active",
				OnsetOn:  &onsetOn,
				Notes:    "avoid entirely",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   allergyEntryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.AllergyPatch{} },
			Sort:       allergy.Sorts(),
			HasPrimaryDate: func(body any) bool {
				detail, ok := body.(*api.Allergy)
				return ok && detail.OnsetOn != nil
			},
			CascadeSkip: "allergy's cascade-on-patient-delete needs a real instance this harness " +
				"cannot reach past records.Service into the store directly to prove; not yet covered " +
				"by an internal/store/allergy integration test",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := allergyEntryFor(t, instance)

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
			DefaultSort: []domain.SortKey{allergy.Sorts()[0]},
			NoPatientSkip: "allergy.Patient is a plain string FR-002 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := allergySearchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func allergyEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.Allergy.Segment())
	require.NoError(t, err)

	return entry
}

func allergySearchRowFor(t *testing.T, instance *apitest.Instance, k kind.Kind, recordID string) (title string, found bool) {
	t.Helper()

	require.NotNil(t, instance.Search, "the instance has no search repository wired")

	row, found, err := instance.Search.Find(context.Background(), k, recordID)
	require.NoError(t, err)

	return row.Title, found
}
