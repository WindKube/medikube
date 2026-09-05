package kinds_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/emergencycontact"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestEmergencyContactSatisfiesTheSharedRecordContracts is T045's proof for
// emergency contacts, mirroring the allergy and condition contract test
// files. KindContractOptions.DefaultSort is []domain.SortKey (generalised
// for this kind's own FR-051 case), so RunKindContract exercises it with
// the full three-term compound (is_active, is_primary, name) rather than
// Entry.Schema.Sorts[0] alone.
func TestEmergencyContactSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.EmergencyContactCreate{Patient: patientID, Name: "Ngozi Okonkwo", Relationship: "spouse", Phone: "+1-555-0100"}
		},
		Full: func(patientID string) any {
			return &api.EmergencyContactCreate{
				Patient:      patientID,
				Name:         "Ngozi Okonkwo",
				Relationship: "spouse",
				Phone:        "+1-555-0100",
				PhoneAlt:     "+1-555-0101",
				Email:        "ngozi@example.test",
				Address:      "1 Example Street",
				IsPrimary:    true,
				Notes:        "prefers text over calls",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   contactEntryFor(t, instance).Service,
			Owner:     access.Actor{UserID: testsupport.AccountAID, RequestID: "req-1"},
			PatientID: testsupport.AccountAPatientChildID,
			Stranger:  access.Actor{UserID: testsupport.AccountBID, RequestID: "req-2"},
		}
	}

	t.Run("RepositoryContract", func(t *testing.T) {
		t.Parallel()

		recordstest.RunRepositoryContract(t, recordstest.RepositoryContractOptions{
			NewHarness:          newHarness,
			Fixture:             fixture,
			NewPatch:            func() any { return &api.EmergencyContactPatch{} },
			Sort:                emergencycontact.Sorts(),
			NullPrimaryDateSkip: "emergency contacts have no primary date column (FR-051's ordering is name/flags, not a date)",
			CascadeSkip: "emergency contact's cascade-on-patient-delete is proven directly against a real " +
				"instance by internal/store/emergencycontact's own TestDeletingAPatientDestroysItsEmergencyContacts",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := contactEntryFor(t, instance)

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
			DefaultSort: emergencycontact.Sorts(),
			NoPatientSkip: "emergencycontact.Patient is a plain string FR-002 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := contactSearchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func contactEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.EmergencyContact.Segment())
	require.NoError(t, err)

	return entry
}

func contactSearchRowFor(t *testing.T, instance *apitest.Instance, k kind.Kind, recordID string) (title string, found bool) {
	t.Helper()

	require.NotNil(t, instance.Search, "the instance has no search repository wired")

	row, found, err := instance.Search.Find(context.Background(), k, recordID)
	require.NoError(t, err)

	return row.Title, found
}
