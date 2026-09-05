package testsupport

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/seed"
)

// T092. Two things can drift apart here and both of them break five phases
// quietly rather than loudly.
//
// The first is the seed moving away from the constants above: every ownership
// test in phases 002-005 addresses a record by one of those constants, and a
// seed that renumbered an account would make every one of them pass against a
// record that does not exist. The second is the committed fixture going stale:
// tests.NewTestApp reapplies missing migrations to the clone, so a schema
// change self-heals and the *rows* do not — the fixture keeps yesterday's data
// in today's schema and nothing says so.
//
// So the assertions are written against both: the constants against the
// fixture, and the fixture against a freshly seeded instance.

func TestTheCommittedFixtureIsExactlyWhatTheSeedWrites(t *testing.T) {
	t.Parallel()

	committed := snapshot(t, NewApp(t))
	fresh := snapshot(t, freshlySeeded(t))

	require.Equal(t, fresh, committed,
		"the committed fixture is stale. Regenerate it:\n"+
			"    %s=1 go test -count=1 -run TestRegenerateTheCommittedFixture ./internal/testsupport/\n"+
			"and commit %s", regenEnv, FixtureDir())
}

// patientsOfAccount is every patient id data-model §9 seeds for one account,
// boxed for dbx.HashExp: only []interface{} gets its IN-clause treatment
// (dbx/expression.go's HashExp.Build) — a []string falls through to the
// single-value branch and dbx refuses to bind a slice as one parameter.
func patientsOfAccount(accountID string) []interface{} {
	var ids []interface{}

	for _, patient := range seed.Patients() {
		if patient.OwnerID == accountID {
			ids = append(ids, patient.ID)
		}
	}

	return ids
}

func TestEveryExportedFixtureIdentifierNamesASeededRecord(t *testing.T) {
	t.Parallel()

	app := NewApp(t)

	accounts := []struct {
		id, email, name string
		medications     int
		confirmed       bool
	}{
		{AccountAID, AccountAEmail, AccountAName, AccountAMedicationCount, AccountAConfirmed},
		{AccountBID, AccountBEmail, AccountBName, AccountBMedicationCount, AccountBConfirmed},
		{AccountCID, AccountCEmail, AccountCName, AccountCMedicationCount, AccountCConfirmed},
	}

	for _, account := range accounts {
		t.Run(account.email, func(t *testing.T) {
			record, err := app.FindRecordById(usersCollection, account.id)
			require.NoError(t, err, "no account %s: the seed and the constants have drifted", account.id)

			assert.Equal(t, account.email, record.Email())
			assert.Equal(t, account.name, record.GetString("name"))
			assert.Equal(t, string(identity.DefaultRole), record.GetString("role"))

			assert.True(t, record.ValidatePassword(Password),
				"%s cannot sign in with the published password", account.email)

			assert.Equal(t, account.confirmed, record.Verified(),
				"%s is in the other confirmation state (FR-075, T222)", account.email)

			owned, err := app.CountRecords(kind.Medication.Collection(),
				dbx.HashExp{"patient": patientsOfAccount(account.id)})
			require.NoError(t, err)
			assert.Equal(t, int64(account.medications), owned,
				"%s holds a different number of records than the constants say", account.email)
		})
	}
}

// T036/data-model §9's cast. Every id the constants publish must resolve to a
// real row, the same way the account ids above are checked against the seed.
func TestEveryPhase002FixtureIdentifierNamesASeededRecord(t *testing.T) {
	t.Parallel()

	app := NewApp(t)

	facilities := []string{AccountAFacilityPracticeID, AccountAFacilityPharmacyID}
	for _, id := range facilities {
		_, err := app.FindRecordById("facilities", id)
		require.NoError(t, err, "no facility %s: the seed and the constants have drifted", id)
	}

	practitioners := []string{AccountAPractitionerID, AccountBPractitionerID}
	for _, id := range practitioners {
		_, err := app.FindRecordById("practitioners", id)
		require.NoError(t, err, "no practitioner %s: the seed and the constants have drifted", id)
	}

	patients := []string{
		AccountAPatientSelfID, AccountAPatientChildID, AccountAPatientParentID, AccountBPatientSelfID,
	}
	for _, id := range patients {
		_, err := app.FindRecordById("patients", id)
		require.NoError(t, err, "no patient %s: the seed and the constants have drifted", id)
	}

	for accountID, patientID := range map[string]string{
		AccountAID: AccountAPatientSelfID,
		AccountBID: AccountBPatientSelfID,
	} {
		record, err := app.FindRecordById(usersCollection, accountID)
		require.NoError(t, err)
		assert.Equal(t, patientID, record.GetString("active_patient"),
			"%s's active_patient does not point at its own self-record", accountID)
	}
}

// T222 and FR-075. One seeded account has an unconfirmed address and at least
// one has a confirmed one, so BOTH states of the settings page are reachable
// from the fixture and neither is a branch nothing ever renders.
//
// It is asserted as a shape rather than by naming account C, because what the
// smoke run needs is one of each: a seed that moved the unconfirmed address to
// another account would still be a fixture that covers both branches, and one
// that confirmed everybody would not.
func TestTheFixtureHoldsBothConfirmationStates(t *testing.T) {
	t.Parallel()

	app := NewApp(t)

	var confirmed, unconfirmed int

	for _, account := range seed.Accounts() {
		record, err := app.FindRecordById(usersCollection, account.ID)
		require.NoError(t, err)

		if record.Verified() {
			confirmed++

			continue
		}

		unconfirmed++
	}

	assert.Positive(t, confirmed, "no seeded address is confirmed, so the settled state renders nowhere")
	assert.Positive(t, unconfirmed,
		"every seeded address is confirmed, so the settings page's \"not confirmed, send it again\" state "+
			"is a branch the browser gate never reaches (FR-075, research D-39)")
}

func TestTheSeededSuperuserIsTheAdminCredentialQuickstartPublishes(t *testing.T) {
	t.Parallel()

	app := NewApp(t)

	record, err := app.FindRecordById(core.CollectionNameSuperusers, SuperuserID)
	require.NoError(t, err)

	assert.Equal(t, SuperuserEmail, record.Email())
	assert.True(t, record.ValidatePassword(Password))

	// The two tiers are different things and FR-040 turns on them staying
	// different: the superuser bypasses every API rule, an identity.RoleAdmin
	// account does not.
	_, err = app.FindRecordById(usersCollection, SuperuserID)
	assert.Error(t, err, "the superuser must not also be an ordinary account")
}

func TestTheFourNamedRowsAreTheEdgeCasesTheyAreNamedFor(t *testing.T) {
	t.Parallel()

	app := NewApp(t)
	collection := kind.Medication.Collection()

	t.Run("only a name", func(t *testing.T) {
		record, err := app.FindRecordById(collection, NameOnlyMedicationID)
		require.NoError(t, err)

		assert.NotEmpty(t, record.GetString("name"))
		assert.Equal(t, string(clinical.TherapyStatusActive), record.GetString("status"))

		// Every optional column, so a renderer that assumes a dose or a date is
		// present has something to break on.
		for _, column := range []string{
			"alternative_name", "type", "dosage", "frequency",
			"route", "indication", "side_effects", "notes",
		} {
			assert.Empty(t, record.GetString(column), "%s should be empty on the partial row", column)
		}

		assert.True(t, record.GetDateTime("started_on").IsZero())
		assert.True(t, record.GetDateTime("ended_on").IsZero())
	})

	t.Run("right-to-left text and markup characters", func(t *testing.T) {
		record, err := app.FindRecordById(collection, ScriptedMedicationID)
		require.NoError(t, err)

		name := record.GetString("name")
		assert.Contains(t, name, "<b>", "an unescaped template should have an element to render")
		assert.Contains(t, name, "&")
		assert.True(t, strings.ContainsFunc(name, func(r rune) bool {
			return r >= 0x0600 && r <= 0x06FF // Arabic
		}), "the row should carry right-to-left text")
	})

	t.Run("a single-day course", func(t *testing.T) {
		record, err := app.FindRecordById(collection, SingleDayMedicationID)
		require.NoError(t, err)

		started := record.GetDateTime("started_on")
		ended := record.GetDateTime("ended_on")

		require.False(t, started.IsZero())
		assert.Equal(t, started.String(), ended.String(),
			"FR-018 accepts a course that starts and ends on the same day")
	})

	t.Run("a start date in the future", func(t *testing.T) {
		record, err := app.FindRecordById(collection, FutureStartMedicationID)
		require.NoError(t, err)

		started := record.GetDateTime("started_on")
		require.False(t, started.IsZero())
		assert.True(t, started.Time().After(time.Now()),
			"the entity has no clock, so a future start is accepted and the fixture proves it")
	})
}

func TestTheSeededSetSpansEveryStateAndEveryKind(t *testing.T) {
	t.Parallel()

	app := NewApp(t)

	records, err := app.FindAllRecords(kind.Medication.Collection())
	require.NoError(t, err)
	require.Len(t, records, AccountAMedicationCount+AccountBMedicationCount)

	states := make([]string, 0, len(records))
	kinds := make([]string, 0, len(records))

	for _, record := range records {
		states = append(states, record.GetString("status"))
		if value := record.GetString("type"); value != "" {
			kinds = append(kinds, value)
		}
	}

	// A fixture missing a state is a list filter, a badge and a sort order that
	// nothing ever renders.
	for _, state := range clinical.TherapyStatuses() {
		assert.Contains(t, states, string(state), "no seeded row is %s", state)
	}

	for _, value := range clinical.MedicationTypes() {
		assert.Contains(t, kinds, string(value), "no seeded row is a %s", value)
	}
}

func TestSeedingTwiceChangesNothing(t *testing.T) {
	t.Parallel()

	app := freshlySeeded(t)
	before := snapshot(t, app)

	require.NoError(t, seed.Apply(app))

	assert.Equal(t, before, snapshot(t, app), "the seed is not idempotent (FR-060)")
}

// freshlySeeded is the seed applied to an empty storage location: PocketBase's
// own system migrations, then MediKube's, then the rows. It is what the
// committed fixture is compared against.
func freshlySeeded(t *testing.T) core.App {
	t.Helper()

	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())
	require.NoError(t, seed.Apply(app))

	return app
}

// snapshot is every seeded row rendered as one sorted line each, over exactly
// the columns the seed writes.
//
// created and updated are excluded and nothing else is: they are wall-clock
// values that differ between the machine that generated the fixture and the one
// running the test, and comparing them would make the assertion fail always
// rather than when something drifted.
func snapshot(t *testing.T, app core.App) []string {
	t.Helper()

	lines := make([]string, 0, 20)

	for _, collection := range []struct {
		name    string
		columns []string
	}{
		{usersCollection, []string{
			"email", "verified", "name", "role",
			"unit_system", "locale", "date_format", "theme", "active_patient",
		}},
		{kind.Medication.Collection(), []string{
			"owner", "name", "alternative_name", "type", "dosage", "frequency",
			"route", "indication", "started_on", "ended_on", "status",
			"side_effects", "notes",
		}},
		{"facilities", []string{
			"owner", "kind", "name", "brand", "city", "country", "phone",
		}},
		{"practitioners", []string{
			"owner", "name", "specialty", "facility", "phone", "email", "website", "notes",
		}},
		{"patients", []string{
			"owner", "first_name", "last_name", "birth_date", "sex", "blood_type",
			"relationship_to_owner", "primary_practitioner", "is_self_record",
		}},
	} {
		records, err := app.FindAllRecords(collection.name)
		require.NoError(t, err)

		for _, record := range records {
			values := make([]string, 0, len(collection.columns))
			for _, column := range collection.columns {
				values = append(values, column+"="+record.GetString(column))
			}

			lines = append(lines, fmt.Sprintf("%s/%s %s",
				collection.name, record.Id, strings.Join(values, " ")))
		}
	}

	sort.Strings(lines)

	return lines
}
