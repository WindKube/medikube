package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// request_id is the one member of the envelope the domain cannot supply: it is
// minted per request at the HTTP edge, which is also the layer that assembles
// the outer {"error": …} object. Everything else in the envelope comes from
// here, and this is the seam where the two halves are checked against the
// document rather than against each other.
const envelopeMemberOwnedByTheEdge = "request_id"

// T036. The contract document is the input, not a paraphrase of it: a field
// renamed in contracts/README.md and not here fails, and so does the reverse.
func TestValidationErrorMatchesTheContractEnvelope(t *testing.T) {
	t.Parallel()

	documented := documentedErrorEnvelope(t)

	var v ValidationError
	v.Add("ended_on", "end_before_start", "the end date is before the start date")

	var produced map[string]any
	require.NoError(t, json.Unmarshal(mustMarshal(t, v.OrNil()), &produced))

	t.Run("members", func(t *testing.T) {
		t.Parallel()

		want := keysOf(documented)
		want = without(want, envelopeMemberOwnedByTheEdge)

		assert.Equal(t, want, keysOf(produced),
			"the *ValidationError envelope and contracts/README.md disagree on members")
		assert.NotContains(t, keysOf(produced), envelopeMemberOwnedByTheEdge,
			"the domain cannot know a request id and must not invent one")
	})

	t.Run("field entries", func(t *testing.T) {
		t.Parallel()

		documentedFields, ok := documented["fields"].([]any)
		require.True(t, ok, "contracts/README.md's envelope has no fields array")
		require.NotEmpty(t, documentedFields)

		documentedEntry, ok := documentedFields[0].(map[string]any)
		require.True(t, ok)

		producedFields, ok := produced["fields"].([]any)
		require.True(t, ok, "fields did not marshal as an array")
		require.Len(t, producedFields, 1)

		producedEntry, ok := producedFields[0].(map[string]any)
		require.True(t, ok)

		assert.Equal(t, keysOf(documentedEntry), keysOf(producedEntry),
			"a fields[] entry and contracts/README.md disagree on members")
		assert.Equal(t, "ended_on", producedEntry["field"])
		assert.Equal(t, "end_before_start", producedEntry["code"])
		assert.NotEmpty(t, producedEntry["message"])
	})

	t.Run("code is the one the taxonomy maps to 422", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, documented["code"], produced["code"])
		assert.Equal(t, CodeValidationFailed, produced["code"])
		assert.NotEmpty(t, produced["message"])
	})
}

// Go 1.27's encoding/json/v2 retrofit is not fully backward compatible around
// nil-versus-empty slices, and the contract says slices marshal as [] and never
// null (research D-28). An empty fields array is only reachable through a
// hand-built value, but a client that branches on Array.isArray must never see
// null.
func TestValidationErrorMarshalsAnEmptyFieldsArrayAsAnArray(t *testing.T) {
	t.Parallel()

	assert.JSONEq(t,
		`{"code":"validation_failed","message":"`+ValidationMessage+`","fields":[]}`,
		string(mustMarshal(t, &ValidationError{})))
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

// The first fenced JSON block under the envelope heading. Deliberately not a
// copy of the document: a copy drifts, and drift between the domain type and
// the published contract is exactly what this test exists to catch.
func documentedErrorEnvelope(t *testing.T) map[string]any {
	t.Helper()

	path := filepath.Join(repoRoot(t), "specs", "001-walking-skeleton", "contracts", "README.md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "the contract document this type is written against is missing")

	block := regexp.MustCompile("(?s)## The error envelope.*?```json\n(.*?)```").FindSubmatch(raw)
	require.Len(t, block, 2, "no fenced JSON envelope found under the envelope heading in %s", path)

	var envelope struct {
		Error map[string]any `json:"error"`
	}
	require.NoError(t, json.Unmarshal(block[1], &envelope), "the documented envelope is not valid JSON")
	require.NotEmpty(t, envelope.Error)
	return envelope.Error
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "walked to the filesystem root without finding go.mod")
		dir = parent
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func without(keys []string, drop string) []string {
	kept := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != drop {
			kept = append(kept, key)
		}
	}
	return kept
}
