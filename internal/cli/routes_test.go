package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
	"medikube/internal/httproute"
)

func fixtureRoutes() []httproute.Route {
	return []httproute.Route{
		{
			OpID:    "getRecord",
			Method:  "GET",
			Path:    "/api/v1/records/{kind}/{id}",
			Kind:    httproute.KindAPI,
			Auth:    httproute.AuthUser,
			Summary: "One record.",
		},
		{
			OpID:     "overview",
			Method:   "GET",
			Path:     "/",
			Kind:     httproute.KindPage,
			Auth:     httproute.AuthUser,
			Summary:  "The signed-in account's overview.",
			Landmark: "[role=main]",
			SmokeURL: "/",
		},
	}
}

// T276: `medikube routes --json` lists exactly the registry's routes, needs no
// database and binds no port. deps here carries nothing else — no Bootstrap,
// no OpenAPI builder — which is the proof: if runRoutes needed either, this
// test would panic on a nil func rather than pass.
func TestRoutesJSONListsExactlyTheRegistry(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{
		Stdout: &stdout,
		Stderr: &stderr,
		Routes: fixtureRoutes,
	}

	handled, err := cli.Dispatch([]string{"routes", "--json"}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	var rows []cli.RouteRow
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))

	require.Len(t, rows, 2)
	assert.Equal(t, cli.RouteRow{
		OpID: "getRecord", Method: "GET", Path: "/api/v1/records/{kind}/{id}",
		Kind: "api", Auth: "user", Summary: "One record.",
	}, rows[0])
	assert.Equal(t, cli.RouteRow{
		OpID: "overview", Method: "GET", Path: "/",
		Kind: "page", Auth: "user", Landmark: "[role=main]", SmokeURL: "/",
		Summary: "The signed-in account's overview.",
	}, rows[1])
}

func TestRoutesHumanTableListsMethodPathAuthLandmarkSummary(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, Routes: fixtureRoutes}

	handled, err := cli.Dispatch([]string{"routes"}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "METHOD")
	assert.Contains(t, out, "GET")
	assert.Contains(t, out, "/api/v1/records/{kind}/{id}")
	assert.Contains(t, out, "user")
	assert.Contains(t, out, "[role=main]")
	assert.Contains(t, out, "One record.")
}

func TestRoutesRejectsAnUnknownFlag(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, Routes: fixtureRoutes}

	handled, err := cli.Dispatch([]string{"routes", "--nope"}, deps)
	assert.True(t, handled)
	assert.Error(t, err)
}
