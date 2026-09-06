package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/familymember"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestFamilyMemberSatisfiesTheSharedRecordContracts is US10's T195/T197: the
// same shared repositorycontract and kindcontract suites every other kind
// runs, this time against family_member through a fully wired instance.
func TestFamilyMemberSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.FamilyMemberCreate{Patient: patientID, Name: "Chiamaka Okonkwo", Relationship: "aunt"}
		},
		Full: func(patientID string) any {
			birthYear := 1970
			deathYear := 2020
			diagnosedAge := 55

			return &api.FamilyMemberCreate{
				Patient:      patientID,
				Name:         "Chiamaka Okonkwo",
				Relationship: "aunt",
				Sex:          "female",
				BirthYear:    &birthYear,
				DeathYear:    &deathYear,
				IsDeceased:   true,
				Conditions: []api.FamilyCondition{
					{
						Name:         "Hypertension",
						ICD10Code:    "I10",
						DiagnosedAge: &diagnosedAge,
						Severity:     "moderate",
						Status:       "chronic",
						Notes:        "managed with medication",
					},
				},
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   familyMemberEntryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.FamilyMemberPatch{} },
			Sort:       familymember.Sorts(),
			NullPrimaryDateSkip: "family_member's default sort is relationship, then name, then id " +
				"(data-model §4.13) — there is no date field for it to order by",
			CascadeSkip: "family_member's cascade-on-patient-delete is proven against a real instance " +
				"by internal/store/familymember/repo_integration_test.go; this harness has no patient " +
				"to delete without reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := familyMemberEntryFor(t, instance)

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
			DefaultSort: []domain.SortKey{familymember.Sorts()[0]},
			NoPatientSkip: "family_member.Patient is a plain string FR-052 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := searchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func familyMemberEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.FamilyMember.Segment())
	require.NoError(t, err)

	return entry
}
