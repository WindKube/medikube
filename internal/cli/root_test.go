package cli_test

import (
	"bytes"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
)

// T282: MediKube's subcommands are dispatched by name, and every MediKube flag
// is defined on its own subcommand — never globally. --help is what proves
// dispatch happened without needing a working Deps func for every command:
// flag.ContinueOnError's --help returns flag.ErrHelp before runRoutes,
// runOpenAPI, runHealthcheck or runSeed ever calls into Deps.
func TestDispatchRecognisesEveryMediKubeCommand(t *testing.T) {
	t.Parallel()

	for _, name := range cli.Names() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			handled, err := cli.Dispatch([]string{name, "--help"}, cli.Deps{Stdout: &stdout, Stderr: &stderr})

			assert.True(t, handled, "%s was not recognised as a MediKube command", name)
			assert.ErrorIs(t, err, flag.ErrHelp)
		})
	}
}

func TestDispatchLeavesPocketBasesOwnCommandsUnhandled(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"serve"},
		{"superuser"},
		{"migrate"},
		{"migrate", "up"},
		{},
	} {
		handled, err := cli.Dispatch(args, cli.Deps{})

		assert.Falsef(t, handled, "%v was handled; it belongs to PocketBase's RootCmd", args)
		assert.NoError(t, err)
	}
}

// Every MediKube flag is defined on a FlagSet built inside Dispatch and never
// on flag.CommandLine — the global set PocketBase's own pre-parse of --dir,
// --encryptionEnv and --dev reads from (contracts/cli.md trap 2). A flag
// registered there would collide with a later command's flag of the same name
// and would survive across Dispatch calls, which a fresh FlagSet per call
// cannot.
func TestNoMediKubeFlagIsRegisteredGlobally(t *testing.T) {
	before := 0
	flag.CommandLine.VisitAll(func(*flag.Flag) { before++ })

	var stdout, stderr bytes.Buffer

	handled, err := cli.Dispatch([]string{"routes", "--json"}, cli.Deps{
		Stdout: &stdout, Stderr: &stderr, Routes: fixtureRoutes,
	})
	require.True(t, handled)
	require.NoError(t, err)

	after := 0
	flag.CommandLine.VisitAll(func(*flag.Flag) { after++ })

	assert.Equal(t, before, after, "routes' --json flag leaked onto flag.CommandLine")
	assert.Nil(t, flag.CommandLine.Lookup("json"))
}

func TestUsageListsEveryMediKubeCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	require.NoError(t, cli.Usage(&out))

	for _, name := range cli.Names() {
		assert.Contains(t, out.String(), name)
	}
}
