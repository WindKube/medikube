package api_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/equipment"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestEquipmentSatisfiesTheSharedRecordContracts is US5's T125/T127: the same
// shared repositorycontract and kindcontract suites medication runs, this
// time against equipment through a fully wired instance.
func TestEquipmentSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.EquipmentCreate{Patient: patientID, Name: "CPAP machine", Type: "cpap"}
		},
		Full: func(patientID string) any {
			prescribedOn := "2024-01-01"
			servicedOn := "2024-06-01"
			serviceDueOn := "2025-06-01"

			return &api.EquipmentCreate{
				Patient:      patientID,
				Name:         "CPAP machine",
				Type:         "cpap",
				Manufacturer: "ResMed",
				Model:        "AirSense 10",
				Serial:       "SN-001",
				PrescribedOn: &prescribedOn,
				ServicedOn:   &servicedOn,
				ServiceDueOn: &serviceDueOn,
				Instructions: "clean weekly",
				Status:       "active",
				Notes:        "on loan",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   equipmentEntryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.EquipmentPatch{} },
			Sort:       equipment.Sorts(),
			HasPrimaryDate: func(body any) bool {
				detail, ok := body.(*api.Equipment)
				return ok && detail.PrescribedOn != nil
			},
			CascadeSkip: "equipment's cascade-on-patient-delete is proven against a real instance " +
				"by internal/store/equipment/repo_integration_test.go; this harness has no patient " +
				"to delete without reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := equipmentEntryFor(t, instance)

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
			DefaultSort: equipment.Sorts()[0],
			NoPatientSkip: "equipment.Patient is a plain string FR-048 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := searchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func equipmentEntryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.Equipment.Segment())
	require.NoError(t, err)

	return entry
}
