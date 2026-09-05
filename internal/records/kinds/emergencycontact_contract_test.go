package kinds_test

import (
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
// files, save for KindContract: FR-051 fixes ONE compound default ordering
// (is_active, is_primary, name) rather than several single-field
// alternatives, and recordstest.KindContractOptions.DefaultSort — one
// domain.SortKey — can only ever exercise Entry.Schema.Sorts[0] alone. That
// single term is not this kind's ordering, so the generic KindContract's
// internal List calls would be refused by the very query it exists to
// prove — TestEmergencyContactRegistrationIsComplete below asserts the same
// registration completeness by hand instead.
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
			CascadeSkip: "emergency contact's cascade-on-patient-delete needs a real instance this harness " +
				"cannot reach past records.Service into the store directly to prove; not yet covered " +
				"by an internal/store/emergencycontact integration test",
		})
	})
}

// TestEmergencyContactRegistrationIsComplete replaces KindContract's coverage
// for the reason documented on the test above: it exercises the same six
// generic operations and the same registration-completeness assertion, each
// List call carrying the kind's own full compound default rather than the
// single term the generic suite would build.
func TestEmergencyContactRegistrationIsComplete(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	entry := contactEntryFor(t, instance)

	recordstest.AssertRegistrationComplete(t, entry)

	owner := access.Actor{UserID: testsupport.AccountAID, RequestID: "req-1"}
	patientID := testsupport.AccountAPatientChildID

	created, err := entry.Service.Create(t.Context(), owner, &api.EmergencyContactCreate{
		Patient: patientID, Name: "Ngozi Okonkwo", Relationship: "spouse", Phone: "+1-555-0100",
	})
	require.NoError(t, err)

	listed, err := entry.Service.List(t.Context(), owner, records.Query{
		PatientID: patientID, Sort: emergencycontact.Sorts(), Limit: 100,
	})
	require.NoError(t, err)

	var found bool
	for _, item := range listed.Items {
		if item.ID == created.ID {
			found = true
		}
	}
	require.True(t, found, "the created contact is not in its own patient's list")

	found2, err := entry.Service.Get(t.Context(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, found2.ID)

	updated, err := entry.Service.Update(t.Context(), owner, created.ID, created.Version,
		&api.EmergencyContactPatch{})
	require.NoError(t, err)

	require.NoError(t, entry.Service.Delete(t.Context(), owner, created.ID, updated.Version))

	_, err = entry.Service.Get(t.Context(), owner, created.ID)
	require.Error(t, err)
}

func contactEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.EmergencyContact.Segment())
	require.NoError(t, err)

	return entry
}
