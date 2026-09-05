package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/openapi"
)

// T276: `medikube routes` and `medikube openapi` need no MEDIKUBE_DATA_DIR at
// all. This runs dispatchMediKube in-process with the environment variable
// unset, which is the same boot config.Load would otherwise refuse
// (internal/config/validate.go).
func TestDispatchRoutesNeedsNoDataDirectory(t *testing.T) {
	t.Setenv("MEDIKUBE_DATA_DIR", "")

	handled, err := dispatchMediKube([]string{"routes", "--json"})
	require.True(t, handled)
	assert.NoError(t, err)
}

func TestDispatchOpenAPINeedsNoDataDirectory(t *testing.T) {
	t.Setenv("MEDIKUBE_DATA_DIR", "")

	handled, err := dispatchMediKube([]string{"openapi"})
	require.True(t, handled)
	assert.NoError(t, err)
}

// dispatchMediKube falls through to PocketBase's RootCmd for everything it
// does not recognise, which is what lets serve, superuser and migrate reach
// config.Load and app.Execute in run().
func TestDispatchMediKubeLeavesEverythingElseUnhandled(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"serve"}, {"superuser"}, {"migrate"}, {}} {
		handled, err := dispatchMediKube(args)
		assert.Falsef(t, handled, "%v was handled by dispatchMediKube; it belongs to PocketBase's RootCmd", args)
		assert.NoError(t, err)
	}
}

// openAPIInput is a pure function of the compiled binary — the route
// inventory and the record kinds' DTOs, both facts about the code rather than
// about a running instance (research.md's own description of what this
// command needs). This is the proof: Generate, RoundTrip and Marshal all
// succeed from it with no app, no config and no database anywhere in call.
func TestOpenAPIInputProducesAValidDocument(t *testing.T) {
	t.Parallel()

	in, err := openAPIInput()
	require.NoError(t, err)

	document, err := openapi.Generate(in)
	require.NoError(t, err)

	loaded, _, err := openapi.RoundTrip(context.Background(), document)
	require.NoError(t, err)

	encoded, err := openapi.Marshal(loaded)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)
}
