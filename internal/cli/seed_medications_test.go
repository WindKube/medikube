package cli_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
)

// T185. internal/testsupport/fixtures_test.go asserts the committed fixture is
// what the seed writes; this file asserts what the seed writes is the mixed set
// data-model §6 and contracts/pages.md describe, and that `medikube seed` is
// the thing that writes it rather than a second table of demo rows.
//
// The two halves are both needed and neither implies the other. A seed that
// drifted into a uniform set would still match its own fixture perfectly, and
// every one of the browser gate's cases — the populated list, the account with
// fewer, the empty state, the row with nothing but a name — would go on passing
// against data that no longer exercises them.

// development is a seedable environment. Spelled once here because every case
// below that is not about the refusal needs one.
const development = "development"

func TestTheSeededMedicationSetIsTheMixedSetTheContractsDescribe(t *testing.T) {
	t.Parallel()

	medications := seed.Medications()
	require.NotEmpty(t, medications, "the seed writes no %s rows at all", kind.Medication.Collection())

	owners := patientOwners()

	byOwner := make(map[string][]clinical.Medication)
	for _, medication := range medications {
		byOwner[owners[medication.PatientID]] = append(byOwner[owners[medication.PatientID]], medication)
	}

	t.Run("the counts are the ones every test is pinned to", func(t *testing.T) {
		t.Parallel()

		// Read back from internal/testsupport rather than restated, because
		// those constants are what five phases of ownership tests address and
		// a seed that moved without them would make all of those pass against
		// records that are not there.
		assert.Len(t, byOwner[testsupport.AccountAID], testsupport.AccountAMedicationCount)
		assert.Len(t, byOwner[testsupport.AccountBID], testsupport.AccountBMedicationCount)
		assert.Len(t, byOwner[testsupport.AccountCID], testsupport.AccountCMedicationCount)
	})

	t.Run("one account holds none, and it is the empty state's account", func(t *testing.T) {
		t.Parallel()

		// research D-39: the empty branch is exercised on every smoke run
		// rather than asserted once, and that only works while some account
		// genuinely has nothing.
		assert.Empty(t, byOwner[testsupport.AccountCID],
			"account C holds records, so the browser gate's empty-state case navigates to a populated page")
	})

	t.Run("a second account holds fewer, and none of the first account's", func(t *testing.T) {
		t.Parallel()

		counterparty := byOwner[testsupport.AccountBID]
		require.NotEmpty(t, counterparty,
			"account B holds nothing, so every stranger-refused test addresses records nobody owns")
		assert.Less(t, len(counterparty), len(byOwner[testsupport.AccountAID]),
			"the two populated accounts hold the same number of rows, so a list that ignored the owner would look correct")

		owned := make(map[string]string, len(medications))
		for _, medication := range medications {
			previous, duplicate := owned[medication.ID]
			require.Falsef(t, duplicate, "%s is seeded twice, for %s and %s", medication.ID, previous, medication.PatientID)
			owned[medication.ID] = medication.PatientID
		}
	})

	t.Run("account A spans every published state and every published kind", func(t *testing.T) {
		t.Parallel()

		// Over the domain's published vocabulary and not over a list written
		// here: a state added to clinical without a seeded row is a filter, a
		// badge and a sort order that nothing ever renders, and this is what
		// says so.
		primary := byOwner[testsupport.AccountAID]

		for _, status := range clinical.TherapyStatuses() {
			assert.Truef(t, slices.ContainsFunc(primary, func(m clinical.Medication) bool {
				return m.Status == status
			}), "no seeded row of account A is %s", status)
		}

		for _, value := range clinical.MedicationTypes() {
			assert.Truef(t, slices.ContainsFunc(primary, func(m clinical.Medication) bool {
				return m.Type == value
			}), "no seeded row of account A is a %s", value)
		}
	})

	t.Run("exactly one row has every optional field empty", func(t *testing.T) {
		t.Parallel()

		var bare []string

		for _, medication := range medications {
			if optionalsEmpty(medication) {
				bare = append(bare, medication.ID)
			}
		}

		// Exactly one, not at least one: the partial-data case is the row a
		// renderer that assumes a dose or a date breaks on, and a fixture where
		// half the rows were bare would make the populated cases stop covering
		// the fields they exist for.
		require.Len(t, bare, 1, "the partial-data edge case is not exactly one row: %v", bare)
		assert.Equal(t, testsupport.NameOnlyMedicationID, bare[0],
			"the bare row is not the one internal/testsupport names, so every test that asks for it by name gets a populated row")
	})

	t.Run("every seeded row would be accepted from a person", func(t *testing.T) {
		t.Parallel()

		for _, medication := range medications {
			assert.NoErrorf(t, medication.Validate(),
				"%s is a row the application would refuse, which makes it a trap rather than a fixture", medication.ID)
		}
	})
}

// patientOwners maps every seeded patient to the account that owns it. A
// medication names a patient and not an account (research D-13), so every
// assertion in this file that is stated per account reaches the account
// through the patient a row belongs to.
func patientOwners() map[string]string {
	owners := make(map[string]string)
	for _, patient := range seed.Patients() {
		owners[patient.ID] = patient.OwnerID
	}

	return owners
}

// patientsOf is every patient id data-model §9 seeds for one account, boxed
// for dbx.HashExp: only []interface{} gets its IN-clause treatment
// (dbx/expression.go's HashExp.Build) — a []string falls through to the
// single-value branch and dbx refuses to bind a slice as one parameter.
func patientsOf(accountID string) []interface{} {
	var ids []interface{}

	for _, patient := range seed.Patients() {
		if patient.OwnerID == accountID {
			ids = append(ids, patient.ID)
		}
	}

	return ids
}

// optionalsEmpty reports whether every field FR-014 leaves optional is unset.
// Name and Status are absent from the check because they are the two the
// partial row does carry.
func optionalsEmpty(medication clinical.Medication) bool {
	return medication.AlternativeName == "" &&
		medication.Type == "" &&
		medication.Dosage == "" &&
		medication.Frequency == "" &&
		medication.Route == "" &&
		medication.Indication == "" &&
		medication.SideEffects == "" &&
		medication.Notes == "" &&
		medication.StartedOn == domain.Date{} &&
		medication.EndedOn == domain.Date{}
}

func TestSeedWritesTheMixedSetIntoTheDatabase(t *testing.T) {
	t.Parallel()

	app := migrated(t)
	require.NoError(t, cli.Seed(app, development, nil))

	for _, account := range []struct {
		id    string
		count int
	}{
		{testsupport.AccountAID, testsupport.AccountAMedicationCount},
		{testsupport.AccountBID, testsupport.AccountBMedicationCount},
		{testsupport.AccountCID, testsupport.AccountCMedicationCount},
	} {
		owned, err := app.CountRecords(kind.Medication.Collection(), dbx.HashExp{"patient": patientsOf(account.id)})
		require.NoError(t, err)
		assert.Equalf(t, int64(account.count), owned, "%s holds a different number of records than the seed table declares", account.id)
	}

	// The rows in the database are the rows in the table, id for id. A command
	// that wrote its own set would satisfy every count above and fail here.
	records, err := app.FindAllRecords(kind.Medication.Collection())
	require.NoError(t, err)

	written := make([]string, 0, len(records))
	for _, record := range records {
		written = append(written, record.Id)
	}

	declared := make([]string, 0, len(seed.Medications()))
	for _, medication := range seed.Medications() {
		declared = append(declared, medication.ID)
	}

	assert.ElementsMatch(t, declared, written)
}

func TestSeedRefusesEveryEnvironmentThatIsNotDeclaredSeedable(t *testing.T) {
	t.Parallel()

	for _, env := range []string{"production", "prod", "PRODUCTION", "", "Development", "test"} {
		t.Run("MEDIKUBE_ENV="+env, func(t *testing.T) {
			t.Parallel()

			app := migrated(t)

			var out bytes.Buffer
			err := cli.Seed(app, env, &out)

			require.ErrorIs(t, err, cli.ErrProduction, "FR-060: %q is not a seedable environment", env)
			assert.Empty(t, out.String(), "the refusal still printed a report")

			// The refusal has to come before the write, not merely alongside
			// it. A command that seeded and then complained would have put
			// demo rows in the database it was refusing to touch.
			written, countErr := app.CountRecords(kind.Medication.Collection())
			require.NoError(t, countErr)
			assert.Zero(t, written, "the refused run wrote records anyway")
		})
	}

	for _, env := range []string{"staging", development} {
		t.Run("MEDIKUBE_ENV="+env+" is allowed", func(t *testing.T) {
			t.Parallel()

			assert.NoError(t, cli.Seed(migrated(t), env, nil))
		})
	}
}

func TestSeedRefusesADatabaseTheMigrationsHaveNotReached(t *testing.T) {
	t.Parallel()

	app := bootstrapped(t)

	err := cli.Seed(app, development, nil)
	require.ErrorIs(t, err, cli.ErrNotMigrated)
	assert.Contains(t, err.Error(), kind.Medication.Collection())
}

func TestSeedReportsEachAccountOnceAndTheSecondRunSkipsThemAll(t *testing.T) {
	t.Parallel()

	app := migrated(t)

	var first bytes.Buffer
	require.NoError(t, cli.Seed(app, development, &first))

	created := lines(first.String())
	require.Len(t, created, len(seed.Accounts()), "one line per account (contracts/cli.md)")

	for i, account := range seed.Accounts() {
		assert.Contains(t, created[i], "created")
		assert.Contains(t, created[i], account.Email)
	}

	// The counts are on the line because the shape of the fixture is what an
	// operator is checking for: A populated, B fewer, C none.
	assert.Contains(t, created[0], "12 "+kind.Medication.Segment())
	assert.Contains(t, created[1], "3 "+kind.Medication.Segment())
	assert.Contains(t, created[2], "0 "+kind.Medication.Segment())

	var second bytes.Buffer
	require.NoError(t, cli.Seed(app, development, &second))

	for _, line := range lines(second.String()) {
		assert.Contains(t, line, "skipped", "the second run reported a creation, so the seed is not idempotent (FR-060)")
	}
}

func lines(raw string) []string {
	return strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
}

// bootstrapped is an instance with PocketBase's own tables and none of
// MediKube's. It is what `medikube seed` meets when somebody forgets
// `medikube migrate`.
func bootstrapped(t *testing.T) core.App {
	t.Helper()

	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	require.NoError(t, app.Bootstrap())

	return app
}

func migrated(t *testing.T) core.App {
	t.Helper()

	app := bootstrapped(t)
	require.NoError(t, app.RunAllMigrations())

	return app
}
