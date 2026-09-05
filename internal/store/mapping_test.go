package store

import (
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
)

// T079. The round trip goes through the database rather than through two
// function calls, because the interesting failures are all in the column: a
// value PocketBase normalised on write, a column that holds the empty string
// where the entity holds a zero value, and a field name that never existed and
// reads back as "" without complaining.
func TestAMedicationRoundTripsThroughARecordAndTheDatabase(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "owner@example.test")
	patient := seedPatient(t, app, owner.Id)

	original := sampleMedication(t, patient.Id)
	saved := seedMedication(t, app, original)

	reloaded, err := app.FindRecordById(kind.Medication.Collection(), saved.Id)
	require.NoError(t, err)

	got, err := MedicationFromRecord(reloaded)
	require.NoError(t, err)

	assert.Equal(t, saved.Id, got.ID)
	assert.Equal(t, original.PatientID, got.PatientID)
	assert.Equal(t, original.Name, got.Name)
	assert.Equal(t, original.AlternativeName, got.AlternativeName)
	assert.Equal(t, original.Type, got.Type)
	assert.Equal(t, original.Dosage, got.Dosage)
	assert.Equal(t, original.Frequency, got.Frequency)
	assert.Equal(t, original.Route, got.Route)
	assert.Equal(t, original.Indication, got.Indication)
	assert.Equal(t, original.StartedOn, got.StartedOn)
	assert.Equal(t, original.EndedOn, got.EndedOn)
	assert.Equal(t, original.Status, got.Status)
	assert.Equal(t, original.SideEffects, got.SideEffects)
	assert.Equal(t, original.Notes, got.Notes)

	withinRecently(t, got.CreatedAt)
	withinRecently(t, got.UpdatedAt)
	assert.NotEmpty(t, got.Version)

	// And the entity that comes back is one the domain would have accepted, so
	// the mapper cannot be producing something only it can read.
	assert.NoError(t, got.Validate())
}

// The bug this exists for is one line long and completely silent. An unset date
// column holds the empty string, which reads back as the zero types.DateTime,
// whose Time() is time.Time{} — year 1, month 1, day 1, all clock components
// zero, offset zero. That is a *valid* calendar date as far as a naive
// conversion is concerned, so "no start date recorded" becomes 1 January of the
// year 1 and renders as one.
func TestTheAbsentDateStaysAbsentInBothDirections(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "absent@example.test")
	patient := seedPatient(t, app, owner.Id)

	bare := clinical.Medication{
		PatientID: patient.Id,
		Name:      "Paracetamol",
		Status:    clinical.TherapyStatusActive,
	}

	saved := seedMedication(t, app, bare)

	for _, field := range []string{medicationFieldStartedOn, medicationFieldEndedOn} {
		assert.Emptyf(t, saved.GetString(field),
			"%s holds a value for a date that was never recorded", field)
	}

	reloaded, err := app.FindRecordById(kind.Medication.Collection(), saved.Id)
	require.NoError(t, err)

	got, err := MedicationFromRecord(reloaded)
	require.NoError(t, err)

	assert.True(t, got.StartedOn.IsZero(), "the absent start date came back as %s", got.StartedOn)
	assert.True(t, got.EndedOn.IsZero(), "the absent end date came back as %s", got.EndedOn)
	assert.Empty(t, got.StartedOn.String())
	assert.Empty(t, got.EndedOn.String())
}

// FR-019 and research D-27: a date is the same calendar date to every viewer.
// The column has to hold midnight UTC on exactly the day that was recorded, and
// the assertion is on the stored text rather than on a converted value, because
// a conversion is what would hide the fault.
func TestACalendarDateIsStoredAsMidnightUTCOnTheDayItNames(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "dates@example.test")
	patient := seedPatient(t, app, owner.Id)

	cases := []struct {
		date   string
		stored string
	}{
		{"2026-03-01", "2026-03-01 00:00:00.000Z"},
		{"2026-01-01", "2026-01-01 00:00:00.000Z"},
		{"2026-12-31", "2026-12-31 00:00:00.000Z"},
		// The day either side of a DST transition in a western zone, which is
		// where a local-zone conversion shows up as an off-by-one.
		{"2026-03-08", "2026-03-08 00:00:00.000Z"},
		{"2026-11-01", "2026-11-01 00:00:00.000Z"},
	}

	for _, testCase := range cases {
		t.Run(testCase.date, func(t *testing.T) {
			t.Parallel()

			saved := seedMedication(t, app, clinical.Medication{
				PatientID: patient.Id,
				Name:      "Ibuprofen",
				Status:    clinical.TherapyStatusActive,
				StartedOn: mustDate(t, testCase.date),
			})

			var stored string
			require.NoError(t, app.DB().
				Select(medicationFieldStartedOn).
				From(kind.Medication.Collection()).
				Where(dbxID(saved.Id)).
				Row(&stored))
			assert.Equal(t, testCase.stored, stored)

			reloaded, err := app.FindRecordById(kind.Medication.Collection(), saved.Id)
			require.NoError(t, err)

			got, err := MedicationFromRecord(reloaded)
			require.NoError(t, err)
			assert.Equal(t, testCase.date, got.StartedOn.String())
		})
	}
}

// A date column holding a real time of day is a schema fault, and truncating it
// is how the off-by-one-day comes back: the load succeeds, the record moves,
// and nothing reports it. The mapper refuses instead, which is what makes the
// fault visible on the first read rather than on somebody's medication history.
func TestADateColumnCarryingATimeOfDayIsRefusedRatherThanTruncated(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "corrupt@example.test")
	patient := seedPatient(t, app, owner.Id)

	saved := seedMedication(t, app, clinical.Medication{
		PatientID: patient.Id,
		Name:      "Metformin",
		Status:    clinical.TherapyStatusActive,
		StartedOn: mustDate(t, "2026-03-01"),
	})

	// Written past the mapper on purpose: this is data that should not exist,
	// and the point is what happens when it does.
	_, err := app.DB().
		NewQuery("UPDATE {{" + kind.Medication.Collection() + "}} SET [[" + medicationFieldStartedOn + "]] = '2026-03-01 23:45:00.000Z' WHERE [[id]] = {:id}").
		Bind(map[string]any{"id": saved.Id}).
		Execute()
	require.NoError(t, err)

	reloaded, err := app.FindRecordById(kind.Medication.Collection(), saved.Id)
	require.NoError(t, err)

	_, err = MedicationFromRecord(reloaded)
	require.ErrorIs(t, err, ErrMalformedRecord)
	assert.Contains(t, err.Error(), medicationFieldStartedOn,
		"the refusal has to name the column or nobody can act on it")
}

func TestAUserRoundTripsThroughARecordAndTheDatabase(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	record := seedUser(t, app, "profile@example.test")

	reloaded, err := app.FindRecordById(authCollection, record.Id)
	require.NoError(t, err)

	got, err := UserFromRecord(reloaded)
	require.NoError(t, err)

	assert.Equal(t, record.Id, got.ID)
	assert.Equal(t, "profile@example.test", got.Email)
	assert.False(t, got.EmailConfirmed)
	assert.Equal(t, "Test Person", got.Name)
	assert.Equal(t, identity.RoleUser, got.Role)
	assert.Equal(t, identity.UnitSystemMetric, got.UnitSystem)
	assert.Equal(t, "en", got.Locale)
	assert.Equal(t, identity.DateFormatISO, got.DateFormat)
	assert.Equal(t, identity.ThemeSystem, got.Theme)
	assert.True(t, got.DisabledAt.IsZero())
	assert.False(t, got.IsDisabled())

	withinRecently(t, got.CreatedAt)
	withinRecently(t, got.UpdatedAt)

	// The write direction, over a value the profile form can change, and the
	// two it cannot: the mapper must not be a way to set a password, a token
	// key, an address or a verification flag (research D-13, FR-011, FR-075).
	got.Name = "Renamed Person"
	got.Theme = identity.ThemeDark
	got.Email = "attacker@example.test"
	got.EmailConfirmed = true

	require.NoError(t, UserToRecord(reloaded, got))
	require.NoError(t, app.Save(reloaded))

	after, err := app.FindRecordById(authCollection, record.Id)
	require.NoError(t, err)

	written, err := UserFromRecord(after)
	require.NoError(t, err)

	assert.Equal(t, "Renamed Person", written.Name)
	assert.Equal(t, identity.ThemeDark, written.Theme)
	assert.Equal(t, "profile@example.test", written.Email, "the mapper rewrote the sign-in identity")
	assert.False(t, written.EmailConfirmed, "the mapper set the verified flag")
	assert.NotEmpty(t, after.TokenKey(), "the mapper cleared the token key a password change rotates")
}

// data-model §1: a disabled account carries an instant, and the column is a
// PocketBase date, so "not disabled" is the empty string rather than SQL NULL.
func TestADisabledAccountCarriesAnInstantAndAnEnabledOneCarriesNothing(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	record := seedUser(t, app, "disabled@example.test")

	user, err := UserFromRecord(record)
	require.NoError(t, err)
	require.False(t, user.IsDisabled())
	require.Empty(t, record.GetString(userFieldDisabledAt))

	user.DisabledAt = time.Date(2026, time.March, 1, 9, 30, 0, 0, time.UTC)
	require.NoError(t, UserToRecord(record, user))
	require.NoError(t, app.Save(record))

	reloaded, err := app.FindRecordById(authCollection, record.Id)
	require.NoError(t, err)

	disabled, err := UserFromRecord(reloaded)
	require.NoError(t, err)
	assert.True(t, disabled.IsDisabled())
	assert.Equal(t, user.DisabledAt, disabled.DisabledAt)
	assert.Equal(t, time.UTC, disabled.DisabledAt.Location())
}

func TestAnAuditEventRoundTripsThroughARecordAndTheDatabase(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	actor := seedUser(t, app, "actor@example.test")

	collection, err := app.FindCollectionByNameOrId(auditCollection)
	require.NoError(t, err)

	cases := []struct {
		name  string
		event audit.Event
	}{
		{
			name: "a person acting on their own record",
			event: audit.Event{
				OccurredAt: time.Date(2026, time.March, 1, 9, 30, 0, 0, time.UTC),
				ActorID:    actor.Id,
				ActorKind:  audit.ActorKindUser,
				Action:     audit.ActionCreate,
				TargetKind: audit.TargetKindMedication,
				TargetID:   "abcdefghij12345",
				RequestID:  "req-0123456789",
			},
		},
		{
			// research D-22: the actor is empty for a system action, and stays
			// empty rather than becoming a dangling reference.
			name: "a background run with no actor at all",
			event: audit.Event{
				OccurredAt: time.Date(2026, time.March, 2, 3, 0, 0, 0, time.UTC),
				ActorKind:  audit.ActorKindSystem,
				Action:     audit.ActionBackupCreate,
				TargetKind: audit.TargetKindBackup,
				TargetID:   "medikube_20260827120000.zip",
				RequestID:  "run-0123456789",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, testCase.event.Validate())

			record := core.NewRecord(collection)
			require.NoError(t, AuditEventToRecord(record, testCase.event))
			require.NoError(t, app.Save(record))

			reloaded, err := app.FindRecordById(auditCollection, record.Id)
			require.NoError(t, err)

			got, err := AuditEventFromRecord(reloaded)
			require.NoError(t, err)
			assert.Equal(t, testCase.event, got)
			assert.NoError(t, got.Validate())
		})
	}
}

// D-24. The ETag is derived from `updated` and is never a column, so the
// derivation is the only thing standing between two writes and a lost update.
func TestTheVersionIsDerivedFromUpdatedAndIsALegalETagToken(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "version@example.test")
	patient := seedPatient(t, app, owner.Id)

	saved := seedMedication(t, app, sampleMedication(t, patient.Id))

	first := Version(saved)
	require.NotEmpty(t, first)

	// RFC 9110's etagc: an ETag's opaque part may not carry a space, a double
	// quote or a control character, which rules out PocketBase's own date
	// layout — `2026-03-01 09:00:00.000Z` has a space in it.
	for _, r := range first {
		require.Truef(t, r == '!' || (r >= 0x23 && r <= 0x7E), "%q is not a legal ETag character", r)
	}

	// Reading the same row twice is the same version, or every conditional
	// request fails for no reason.
	reloaded, err := app.FindRecordById(kind.Medication.Collection(), saved.Id)
	require.NoError(t, err)
	assert.Equal(t, first, Version(reloaded))

	// A write moves it. `updated` has millisecond resolution, so the second
	// write is separated far enough to be a different instant.
	time.Sleep(2 * time.Millisecond)
	reloaded.Set(medicationFieldNotes, "changed")
	require.NoError(t, app.Save(reloaded))

	after, err := app.FindRecordById(kind.Medication.Collection(), saved.Id)
	require.NoError(t, err)
	assert.NotEqual(t, first, Version(after), "the version did not move when the record did")

	// A record with no `updated` yet has no version rather than a version
	// everybody shares.
	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)
	assert.Empty(t, Version(core.NewRecord(collection)))
}

// Every getter on core.Record is a cast that cannot fail: a misspelled column
// reads back as the zero value and the mapper carries on. This is the only
// thing between that and a field that silently stops being saved.
func TestEveryColumnTheMapperNamesExistsInTheSchema(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	require.NoError(t, AssertMappedFields(app))

	// And it is not vacuous: take a column away and it says so, by name.
	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	collection.Fields.RemoveByName(medicationFieldIndication)
	require.NoError(t, app.Save(collection))

	err = AssertMappedFields(app)
	require.Error(t, err)
	assert.Contains(t, err.Error(), medicationFieldIndication)
	assert.Contains(t, err.Error(), kind.Medication.Collection())
}

func TestAMapperRefusesARecordFromAnotherCollection(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "wrong@example.test")
	patient := seedPatient(t, app, owner.Id)

	medication := seedMedication(t, app, sampleMedication(t, patient.Id))

	auditRecords, err := app.FindCollectionByNameOrId(auditCollection)
	require.NoError(t, err)
	auditRecord := core.NewRecord(auditRecords)

	cases := []struct {
		name string
		call func() error
	}{
		{"a medication read out of an account", func() error {
			_, err := MedicationFromRecord(owner)
			return err
		}},
		{"an account read out of a medication", func() error {
			_, err := UserFromRecord(medication)
			return err
		}},
		{"an audit event read out of a medication", func() error {
			_, err := AuditEventFromRecord(medication)
			return err
		}},
		{"a medication written into an audit row", func() error {
			return MedicationToRecord(auditRecord, clinical.Medication{})
		}},
		{"an account written into a medication", func() error {
			return UserToRecord(medication, identity.User{})
		}},
		{"an audit event written into an account", func() error {
			return AuditEventToRecord(owner, audit.Event{})
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(t, testCase.call(), ErrUnexpectedCollection)
		})
	}
}

// D-28, and the correction this phase owes it. The decision says v2 makes
// unknown fields a rejection; it does not — that is an option, off by default,
// and phase 002's PATCH re-attribution defence rests on it being passed
// explicitly at every decode site. The rest of D-28 holds and is pinned here
// because the store is where a PocketBase value crosses into MediKube's own
// marshalling.
func TestTheJSONSemanticsTheStoreDependsOn(t *testing.T) {
	t.Parallel()

	t.Run("a calendar date marshals as YYYY-MM-DD under both packages", func(t *testing.T) {
		t.Parallel()

		date := mustDate(t, "2026-03-01")

		v1, err := jsonv1.Marshal(date)
		require.NoError(t, err)
		assert.JSONEq(t, `"2026-03-01"`, string(v1))

		v2, err := jsonv2.Marshal(date)
		require.NoError(t, err)
		assert.JSONEq(t, `"2026-03-01"`, string(v2))

		var back domain.Date
		require.NoError(t, jsonv2.Unmarshal(v2, &back))
		assert.Equal(t, date, back)
	})

	t.Run("the absent date marshals as the empty string, not as a year-one date", func(t *testing.T) {
		t.Parallel()

		encoded, err := jsonv2.Marshal(domain.Date{})
		require.NoError(t, err)
		assert.JSONEq(t, `""`, string(encoded))
	})

	t.Run("a PocketBase DateTime is not RFC3339 and must never reach a DTO", func(t *testing.T) {
		t.Parallel()

		instant, err := types.ParseDateTime(time.Date(2026, time.March, 1, 9, 30, 0, 0, time.UTC))
		require.NoError(t, err)

		encoded, err := jsonv1.Marshal(instant)
		require.NoError(t, err)
		assert.JSONEq(t, `"2026-03-01 09:30:00.000Z"`, string(encoded),
			"types.DateTime started marshalling as RFC3339; mapping.go's conversion is now redundant, not wrong")
	})

	t.Run("a nil slice is null under v1 and an empty list under v2", func(t *testing.T) {
		t.Parallel()

		payload := struct {
			Items []string `json:"items"`
		}{}

		v1, err := jsonv1.Marshal(payload)
		require.NoError(t, err)
		assert.JSONEq(t, `{"items":null}`, string(v1))

		v2, err := jsonv2.Marshal(payload)
		require.NoError(t, err)
		assert.JSONEq(t, `{"items":[]}`, string(v2))
	})

	t.Run("a duplicate key is accepted by v1 and refused by v2", func(t *testing.T) {
		t.Parallel()

		var payload struct {
			Name string `json:"name"`
		}

		require.NoError(t, jsonv1.Unmarshal([]byte(`{"name":"a","name":"b"}`), &payload))
		assert.Equal(t, "b", payload.Name)

		assert.Error(t, jsonv2.Unmarshal([]byte(`{"name":"a","name":"b"}`), &payload))
	})

	t.Run("an unknown member is ignored by v2 unless the option is passed", func(t *testing.T) {
		t.Parallel()

		var payload struct {
			Name string `json:"name"`
		}

		assert.NoError(t, jsonv2.Unmarshal([]byte(`{"name":"a","owner":"someone-else"}`), &payload),
			"v2 now rejects unknown members by default; D-28 can stop naming the option")

		assert.Error(t,
			jsonv2.Unmarshal([]byte(`{"name":"a","owner":"someone-else"}`), &payload,
				jsonv2.RejectUnknownMembers(true)),
			"this option is the whole of the PATCH re-attribution defence (D-28)")
	})
}
