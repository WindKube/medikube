package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/insurance"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestInsuranceSatisfiesTheSharedRecordContracts is US5's T125/T127: the same
// shared repositorycontract and kindcontract suites medication runs, this
// time against insurance through a fully wired instance.
func TestInsuranceSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.InsuranceCreate{
				Patient:     patientID,
				Type:        "medical",
				Company:     "Acme Health",
				MemberName:  "Amara Okafor",
				MemberID:    "MEM-001",
				EffectiveOn: "2024-01-01",
			}
		},
		Full: func(patientID string) any {
			expiresOn := "2030-01-01"

			return &api.InsuranceCreate{
				Patient:       patientID,
				Type:          "medical",
				Company:       "Acme Health",
				PlanName:      "Gold PPO",
				EmployerGroup: "Acme Corp",
				MemberName:    "Amara Okafor",
				MemberID:      "MEM-001",
				GroupNumber:   "GRP-9",
				HolderName:    "Amara Okafor",
				Relationship:  "self",
				EffectiveOn:   "2024-01-01",
				ExpiresOn:     &expiresOn,
				Status:        "active",
				IsPrimary:     true,
				Notes:         "primary policy",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   insuranceEntryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.InsurancePatch{} },
			Sort:       insurance.Sorts(),
			CascadeSkip: "insurance's cascade-on-patient-delete is proven against a real instance " +
				"by internal/store/insurance/repo_integration_test.go; this harness has no patient " +
				"to delete without reaching past records.Service into the store directly",
			NullPrimaryDateSkip: "insurance.EffectiveOn is required (FR-043) and Insurance.Validate " +
				"refuses a zero date, so there is no undated record this fixture can create",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := insuranceEntryFor(t, instance)

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
			DefaultSort: []domain.SortKey{insurance.Sorts()[0]},
			NoPatientSkip: "insurance.Patient is a plain string FR-043 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := searchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func insuranceEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.Insurance.Segment())
	require.NoError(t, err)

	return entry
}
