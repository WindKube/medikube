package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/medication"
	"medikube/internal/service/symptom"
	"medikube/internal/service/vitals"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// TestMedicationSatisfiesTheSharedRecordContracts is T022-T024's proof: the
// shared repositorycontract and kindcontract suites, run against medication —
// the one real kind this phase ships — through a fully wired instance
// (migrations, the registry, the search indexer and all), rather than against
// a fake standing in for one.
//
// Two clauses are documented skips rather than silent omissions: medication's
// patient is server-resolved from the actor's own account and never something
// a create body could omit and still decode (Patient is a plain, non-pointer
// string FR-002 requires structurally), so there is no "no patient" body to
// build without editing the wire type by hand; and this phase's fake
// repository tier has no patient to delete, so the cascade is proven at
// internal/store/medication/repo_integration_test.go instead.
func TestMedicationSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.MedicationCreate{Patient: patientID, Name: "Ibuprofen"}
		},
		Full: func(patientID string) any {
			startedOn := "2024-01-01"

			return &api.MedicationCreate{
				Patient:   patientID,
				Name:      "Ibuprofen",
				Dosage:    "200mg",
				Frequency: "twice daily",
				StartedOn: &startedOn,
				Notes:     "as needed for headache",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   entryFor(t, instance).Service,
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
			NewPatch:   func() any { return &api.MedicationPatch{} },
			Sort:       medication.Sorts(),
			HasPrimaryDate: func(body any) bool {
				detail, ok := body.(*api.Medication)
				return ok && detail.StartedOn != nil
			},
			CascadeSkip: "medication's cascade-on-patient-delete is proven against a real instance " +
				"by internal/store/medication/repo_integration_test.go; this harness has no patient " +
				"to delete without reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := entryFor(t, instance)

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
			DefaultSort: []domain.SortKey{medication.Sorts()[0]},
			NoPatientSkip: "medication.Patient is a plain string FR-002 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not " +
				"decode at all; internal/web/api/medication_http_test.go proves the empty-string case",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := searchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

func entryFor(t *testing.T, instance *apitest.Instance) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(kind.Medication.Segment())
	require.NoError(t, err)

	return entry
}

func entryForKind(t *testing.T, instance *apitest.Instance, k kind.Kind) records.Entry {
	t.Helper()

	entry, err := instance.Records.Dispatch(k.Segment())
	require.NoError(t, err)

	return entry
}

// TestSymptomSatisfiesTheSharedRecordContracts is T091's symptom half.
func TestSymptomSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			return &api.SymptomCreate{
				Patient: patientID, Name: "Headache", Severity: "moderate", OccurredAt: "2026-01-01T09:00:00Z",
			}
		},
		Full: func(patientID string) any {
			return &api.SymptomCreate{
				Patient: patientID, Name: "Headache", Category: "pain", Severity: "moderate",
				OccurredAt: "2026-01-01T09:00:00Z", BodySite: "temple", Impact: "moderate",
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   entryForKind(t, instance, kind.Symptom).Service,
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
			NewPatch:   func() any { return &api.SymptomPatch{} },
			Sort:       symptom.Sorts(),
			NullPrimaryDateSkip: "occurred_at is required (FR-029): every symptom episode names when it " +
				"happened, so there is no undated episode to construct",
			CascadeSkip: "symptom's cascade-on-patient-delete is asserted against the real migrated " +
				"schema by internal/store/migrations/assertions_test.go; this harness has no patient " +
				"to delete without reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := entryForKind(t, instance, kind.Symptom)

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
			DefaultSort: []domain.SortKey{symptom.Sorts()[0]},
			NoPatientSkip: "symptom.Patient is a plain string FR-029 requires structurally (there is " +
				"no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := searchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

// TestVitalsSatisfiesTheSharedRecordContracts is T091's vitals half.
func TestVitalsSatisfiesTheSharedRecordContracts(t *testing.T) {
	t.Parallel()

	fixture := recordstest.Fixture{
		Minimal: func(patientID string) any {
			weight := 70.0

			return &api.VitalsCreate{Patient: patientID, RecordedAt: "2026-01-01T09:00:00Z", WeightKg: &weight}
		},
		Full: func(patientID string) any {
			weight, height, systolic, diastolic := 70.0, 175.0, 120.0, 80.0

			return &api.VitalsCreate{
				Patient: patientID, RecordedAt: "2026-01-01T09:00:00Z",
				WeightKg: &weight, HeightCm: &height,
				SystolicMmHg: &systolic, DiastolicMmHg: &diastolic,
			}
		},
	}

	newHarness := func(t *testing.T) recordstest.RepositoryHarness {
		t.Helper()

		instance := apitest.New(t)

		return recordstest.RepositoryHarness{
			Service:   entryForKind(t, instance, kind.Vitals).Service,
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
			NewPatch:   func() any { return &api.VitalsPatch{} },
			Sort:       vitals.Sorts(),
			NullPrimaryDateSkip: "recorded_at is required (FR-033): every measurement set names when " +
				"it was recorded, so there is no undated set to construct",
			CascadeSkip: "measurements' cascade-on-patient-delete is asserted against the real migrated " +
				"schema by internal/store/migrations/assertions_test.go; this harness has no patient " +
				"to delete without reaching past records.Service into the store directly",
		})
	})

	t.Run("KindContract", func(t *testing.T) {
		t.Parallel()

		instance := apitest.New(t)
		entry := entryForKind(t, instance, kind.Vitals)

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
			DefaultSort: []domain.SortKey{vitals.Sorts()[0]},
			NoPatientSkip: "the measurements kind's Patient is a plain string FR-033 requires structurally " +
				"(there is no pointer to omit it with), so a body naming no patient does not decode at all",
			SearchIndex: func(t *testing.T, k kind.Kind, recordID string) (bool, string) {
				t.Helper()

				title, found := searchRowFor(t, instance, k, recordID)

				return found, title
			},
		})
	})
}

// searchRowFor reads the search_index row back through the same real
// repository T030's wiring writes through, proving the write side end to end
// rather than trusting the Indexer's own unit tests to speak for the
// registration that binds it.
func searchRowFor(t *testing.T, instance *apitest.Instance, k kind.Kind, recordID string) (title string, found bool) {
	t.Helper()

	require.NotNil(t, instance.Search, "the instance has no search repository wired")

	row, found, err := instance.Search.Find(context.Background(), k, recordID)
	require.NoError(t, err)

	return row.Title, found
}
