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
	"medikube/internal/service/immunization"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestImmunizationSatisfiesTheSharedRecordContracts is T109's proof: the
// shared repositorycontract and kindcontract suites, run against
// immunization through a fully wired instance, mirroring
// internal/web/api/records_contract_test.go's proof for medication.
func TestImmunizationSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.ImmunizationCreate{Patient: patientID, VaccineName: "Influenza", AdministeredOn: strPtr("2024-01-01")}
		},
		Full: func(patientID string) any {
			return &api.ImmunizationCreate{
				Patient:        patientID,
				VaccineName:    "Influenza",
				TradeName:      "Fluzone",
				AdministeredOn: strPtr("2024-01-01"),
				LotNumber:      "AB1234",
				Manufacturer:   "Sanofi",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   immunizationEntryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.ImmunizationPatch{} },
			Sort:       immunization.Sorts(),
			HasPrimaryDate: func(body any) bool {
				detail, ok := body.(*api.Immunization)
				return ok && detail.AdministeredOn != nil
			},
			NullPrimaryDateSkip: "immunization.AdministeredOn is required (FR-039, data-model §4.8), " +
				"so Fixture.Minimal cannot omit it and there is no unset-primary-date row to sort last",
			CascadeSkip: "immunization's cascade-on-patient-delete is proven against a real instance " +
				"by internal/store/immunization/repo_integration_test.go; this harness has no patient " +
				"to delete without reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := immunizationEntryFor(t, instance)

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
			DefaultSort: []domain.SortKey{immunization.Sorts()[0]},
			NoPatient: func() any {
				return &api.ImmunizationCreate{VaccineName: "Influenza", AdministeredOn: strPtr("2024-01-01")}
			},
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := immunizationSearchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func immunizationEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.Immunization.Segment())
	require.NoError(t, err)

	return entry
}

func immunizationSearchRowFor(t *testing.T, instance *apitest.Instance, k kind.Kind, recordID string) (title string, found bool) {
	t.Helper()

	require.NotNil(t, instance.Search, "the instance has no search repository wired")

	row, found, err := instance.Search.Find(context.Background(), k, recordID)
	require.NoError(t, err)

	return row.Title, found
}

func strPtr(value string) *string { return &value }
