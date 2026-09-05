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
	"medikube/internal/service/injury"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestInjurySatisfiesTheSharedRecordContracts is T109's proof: the shared
// repositorycontract and kindcontract suites, run against injury through a
// fully wired instance (migrations, the registry, the search indexer and all),
// mirroring internal/web/api/records_contract_test.go's medication suite.
func TestInjurySatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	occurredOn := "2024-01-01"

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.InjuryCreate{Patient: patientID, Name: "Twisted ankle", BodyPart: "ankle"}
		},
		Full: func(patientID string) any {
			return &api.InjuryCreate{
				Patient:       patientID,
				Name:          "Twisted ankle",
				Type:          "sprain",
				BodyPart:      "ankle",
				Laterality:    "left",
				OccurredOn:    &occurredOn,
				Mechanism:     "rolled it stepping off a curb",
				Severity:      "moderate",
				Status:        "active",
				RecoveryNotes: "icing twice a day",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   injuryEntryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.InjuryPatch{} },
			Sort:       injury.Sorts(),
			HasPrimaryDate: func(body any) bool {
				detail, ok := body.(*api.Injury)
				return ok && detail.OccurredOn != nil
			},
			CascadeSkip: "injury's cascade-on-patient-delete is a store-tier concern proven by " +
				"internal/store/injury/repo_integration_test.go; this harness has no patient to " +
				"delete without reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := injuryEntryFor(t, instance)

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
			DefaultSort: []domain.SortKey{injury.Sorts()[0]},
			NoPatient: func() any {
				return &api.InjuryCreate{Name: "Twisted ankle", BodyPart: "ankle"}
			},
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := injurySearchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func injuryEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.Injury.Segment())
	require.NoError(t, err)

	return entry
}

func injurySearchRowFor(t *testing.T, instance *apitest.Instance, k kind.Kind, recordID string) (title string, found bool) {
	t.Helper()

	require.NotNil(t, instance.Search, "the instance has no search repository wired")

	row, found, err := instance.Search.Find(context.Background(), k, recordID)
	require.NoError(t, err)

	return row.Title, found
}
