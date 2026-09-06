package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
	"medikube/internal/httproute"
	"medikube/internal/openapi"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// productionOpenAPIDocumentVersion must track cmd/medikube/cli.go's constant
// of the same name: this is the version the committed api/openapi.json
// actually carries, and internal/openapi/staleness_test.go carries its own
// copy of the same duplication for the same reason — neither package may
// import cmd/medikube, and the alternative is a third package that exists
// only to hold one string.
const productionOpenAPIDocumentVersion = "0.1.0"

// validOpenAPIInput mirrors cmd/medikube's own openAPIInput: the production
// route inventory and the one real kind's DTOs. It is what proves runOpenAPI
// actually drives openapi.Generate → RoundTrip → Marshal end to end, rather
// than a fixture invented for this package alone.
func validOpenAPIInput() (openapi.Input, error) {
	return openapi.Input{
		Version:        "test",
		Routes:         httproute.Inventory().Routes(),
		Kinds:          api.OpenAPIKinds(),
		Envelope:       web.Envelope{},
		SearchResponse: api.SearchResponse{},
	}, nil
}

// T279: the command writes a document byte-identical to the committed
// api/openapi.json. This test proves the mechanism against a fixture input;
// internal/openapi/staleness_test.go is what asserts the committed file agrees
// with the real one.
func TestOpenAPIWritesAValidDocumentToStdout(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, OpenAPI: validOpenAPIInput}

	handled, err := cli.Dispatch([]string{"openapi"}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	var document map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &document))
	assert.Equal(t, "3.1.0", document["openapi"])
	assert.NotEmpty(t, document["paths"])
}

func TestOpenAPIIsByteIdenticalBetweenTwoRuns(t *testing.T) {
	t.Parallel()

	var first, second bytes.Buffer

	deps := cli.Deps{Stdout: &first, Stderr: &bytes.Buffer{}, OpenAPI: validOpenAPIInput}
	handled, err := cli.Dispatch([]string{"openapi"}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	deps.Stdout = &second
	handled, err = cli.Dispatch([]string{"openapi"}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	assert.Equal(t, first.Bytes(), second.Bytes())
}

func TestOpenAPIWritesToOutFile(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "openapi.json")

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, OpenAPI: validOpenAPIInput}

	handled, err := cli.Dispatch([]string{"openapi", "--out", out}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	assert.Empty(t, stdout.Bytes(), "the document was written to --out, so nothing belongs on stdout")

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.NotEmpty(t, written)
}

// T279: the command writes a document byte-identical to the committed
// api/openapi.json.
func TestOpenAPIOutputIsByteIdenticalToTheCommittedDocument(t *testing.T) {
	t.Parallel()

	deps := cli.Deps{
		Stdout: nil, // overwritten below
		Stderr: &bytes.Buffer{},
		OpenAPI: func() (openapi.Input, error) {
			return openapi.Input{
				Version:        productionOpenAPIDocumentVersion,
				Routes:         httproute.Inventory().Routes(),
				Kinds:          api.OpenAPIKinds(),
				Envelope:       web.Envelope{},
				SearchResponse: api.SearchResponse{},
			}, nil
		},
	}

	var stdout bytes.Buffer
	deps.Stdout = &stdout

	handled, err := cli.Dispatch([]string{"openapi"}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	committed, err := os.ReadFile(filepath.Join(repoRoot(t), "api", "openapi.json"))
	require.NoError(t, err, "api/openapi.json is not committed; run `task openapi`")

	assert.Equal(t, string(committed), stdout.String(),
		"api/openapi.json is stale: run `task openapi` and commit the result")
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller could not resolve this test file's own path")

	return filepath.Join(filepath.Dir(file), "..", "..")
}

func TestOpenAPIPropagatesTheInputBuilderError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	boom := errors.New("boom")
	deps := cli.Deps{
		Stdout:  &stdout,
		Stderr:  &stderr,
		OpenAPI: func() (openapi.Input, error) { return openapi.Input{}, boom },
	}

	handled, err := cli.Dispatch([]string{"openapi"}, deps)
	assert.True(t, handled)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}
