package cli_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
	"medikube/internal/domain/identity"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
)

// T222. The medication half of the fixture is asserted next door; this is the
// account half — the three accounts every US2 test, the browser gate and
// quickstart.md address by name.
//
// What is being defended is that `medikube seed` produces accounts somebody can
// actually sign in as and a settings page that has something to render: an id
// the constants publish, the published credential, the profile columns filled,
// and both confirmation states present. A seed that wrote three
// indistinguishable confirmed accounts would satisfy every count in this
// package and leave FR-075's "not confirmed, send it again" state as a branch
// nothing ever reaches.

func TestSeedWritesTheThreeAccountsEveryTestAddressesByName(t *testing.T) {
	t.Parallel()

	app := migrated(t)
	require.NoError(t, cli.Seed(app, development, nil))

	for _, account := range []struct {
		id, email, name string
		confirmed       bool
	}{
		{testsupport.AccountAID, testsupport.AccountAEmail, testsupport.AccountAName, testsupport.AccountAConfirmed},
		{testsupport.AccountBID, testsupport.AccountBEmail, testsupport.AccountBName, testsupport.AccountBConfirmed},
		{testsupport.AccountCID, testsupport.AccountCEmail, testsupport.AccountCName, testsupport.AccountCConfirmed},
	} {
		t.Run(account.email, func(t *testing.T) {
			t.Parallel()

			record, err := app.FindRecordById(store.AccountCollection, account.id)
			require.NoError(t, err, "the seed wrote no account %s", account.id)

			assert.Equal(t, account.email, record.Email())
			assert.Equal(t, account.name, record.GetString("name"))
			assert.Equal(t, string(identity.DefaultRole), record.GetString("role"))

			// quickstart.md publishes one credential for all three. An account
			// that cannot sign in is an account the browser gate cannot use.
			assert.True(t, record.ValidatePassword(testsupport.Password),
				"%s does not accept the published demo credential", account.email)

			assert.Equal(t, account.confirmed, record.Verified(),
				"%s is in the other confirmation state (FR-075)", account.email)

			// The settings form renders a selection per preference, so a blank
			// column is a control with nothing selected on first sight.
			for column, want := range map[string]string{
				"unit_system": string(identity.DefaultUnitSystem),
				"locale":      identity.DefaultLocale,
				"date_format": string(identity.DefaultDateFormat),
				"theme":       string(identity.DefaultTheme),
			} {
				assert.Equalf(t, want, record.GetString(column),
					"%s has no %s, so the settings page renders an unset control", account.email, column)
			}
		})
	}
}

// FR-075 and research D-39, at the level the seed decides it: the smoke run
// needs one account whose address is settled and one whose is not, and it is
// the seed rather than a test fixture that has to keep supplying both.
func TestSeedLeavesOneAddressUnconfirmedForTheSettingsPageToOffer(t *testing.T) {
	t.Parallel()

	app := migrated(t)
	require.NoError(t, cli.Seed(app, development, nil))

	confirmed, unconfirmed := confirmationStates(t, app)

	assert.NotEmpty(t, confirmed, "no seeded address is confirmed, so the settled state renders nowhere")
	assert.NotEmpty(t, unconfirmed,
		"every seeded address is confirmed, so the settings page's \"not confirmed, send it again\" "+
			"state is a branch the browser gate never reaches")
}

// The demo instance is re-seeded, and somebody working through the confirmation
// flow on it confirms the very address the next run needs unconfirmed. FR-060's
// idempotence is what puts it back: the seed declares the state rather than
// merely creating the row, so the smoke case survives being exercised.
func TestReSeedingRestoresTheUnconfirmedAddressSomebodyConfirmed(t *testing.T) {
	t.Parallel()

	app := migrated(t)
	require.NoError(t, cli.Seed(app, development, nil))

	_, unconfirmed := confirmationStates(t, app)
	require.NotEmpty(t, unconfirmed)

	for _, id := range unconfirmed {
		record, err := app.FindRecordById(store.AccountCollection, id)
		require.NoError(t, err)

		record.SetVerified(true)
		require.NoError(t, app.Save(record))
	}

	require.NoError(t, cli.Seed(app, development, nil))

	_, again := confirmationStates(t, app)
	assert.Equal(t, unconfirmed, again,
		"a demo instance whose confirmation flow was walked through keeps every address confirmed after re-seeding")
}

// confirmationStates is the seeded accounts split by the state of their address,
// read from the database rather than from the table that wrote it.
func confirmationStates(t *testing.T, app core.App) (confirmed, unconfirmed []string) {
	t.Helper()

	for _, account := range seed.Accounts() {
		record, err := app.FindRecordById(store.AccountCollection, account.ID)
		require.NoError(t, err)

		if record.Verified() {
			confirmed = append(confirmed, account.ID)

			continue
		}

		unconfirmed = append(unconfirmed, account.ID)
	}

	return confirmed, unconfirmed
}
