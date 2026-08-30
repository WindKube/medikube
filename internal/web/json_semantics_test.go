package web

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// research D-28, asserted directly rather than relied upon. Go 1.27 retrofits
// encoding/json onto v2 and the release notes say the result is not fully
// backward compatible; the DTO layer above is built on the four behaviours
// below. A toolchain that relaxed any of them would otherwise change what
// MediKube accepts with nothing going red.

type semanticsFixture struct {
	Name  string   `json:"name"`
	Notes string   `json:"notes,omitempty"`
	Tags  []string `json:"tags"`
}

func TestDuplicateMemberNamesAreRejectedWithNoOptionAskedFor(t *testing.T) {
	t.Parallel()

	var fixture semanticsFixture
	err := json.Unmarshal([]byte(`{"name":"a","name":"b"}`), &fixture)

	require.Error(t, err, "the last value silently won, so a request can smuggle a field past a proxy that read the first")

	var syntactic *jsontext.SyntacticError
	require.ErrorAs(t, err, &syntactic)
	assert.Equal(t, "name", syntactic.JSONPointer.LastToken(),
		"the offending member is not machine-recoverable, so the field error would have to be parsed out of prose")
}

// Two halves, and both matter. A case-mismatched member is not an error by
// itself — it is simply unknown — and it becomes one only in combination with
// RejectUnknownMembers. A DTO layer that assumed the first half alone would
// accept "Name" silently.
func TestACaseInsensitiveMatchNoLongerSilentlySucceeds(t *testing.T) {
	t.Parallel()

	t.Run("it does not fill the field", func(t *testing.T) {
		t.Parallel()

		var fixture semanticsFixture
		require.NoError(t, json.Unmarshal([]byte(`{"Name":"a"}`), &fixture))
		assert.Empty(t, fixture.Name, "a case-insensitive match filled the field, which v1 used to do")
	})

	t.Run("and with RejectUnknownMembers it is a hard error", func(t *testing.T) {
		t.Parallel()

		var fixture semanticsFixture
		err := json.Unmarshal([]byte(`{"Name":"a"}`), &fixture, json.RejectUnknownMembers(true))
		require.Error(t, err)

		var semantic *json.SemanticError
		require.ErrorAs(t, err, &semantic)
		assert.True(t, errors.Is(semantic.Err, json.ErrUnknownName))
		assert.Equal(t, "Name", semantic.JSONPointer.LastToken())
	})
}

func TestANilSliceMarshalsAsAnEmptyArray(t *testing.T) {
	t.Parallel()

	out, err := json.Marshal(semanticsFixture{Name: "a"})
	require.NoError(t, err)

	assert.Equal(t, `{"name":"a","tags":[]}`, string(out),
		"a list came back null, and a client that iterates it breaks on the empty case")
}

// The other half of the same fact: null is still reachable, deliberately, and a
// DTO that wanted it would have to ask. Asserted so that "slices are []" is
// known to be the default rather than an accident of this fixture.
func TestNullIsOnlyReachableByAskingForIt(t *testing.T) {
	t.Parallel()

	out, err := json.Marshal(semanticsFixture{Name: "a"}, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	assert.Equal(t, `{"name":"a","tags":null}`, string(out))
}

// contracts/records.md spells the two patch dates **string and says the
// mechanism "marshals correctly under encoding/json/v2". It does not work: an
// explicit null zeroes the whole pointer chain, so a cleared field and an
// omitted one arrive identical. internal/web/dto.go carries Optional[T]
// instead, and this is the guard that pins the reason.
//
// If a future toolchain makes **T carry the distinction, this test goes red and
// the deviation can be reconsidered — which is the only honest way to hold a
// decision like this.
func TestAPointerToAPointerCannotCarryAbsentVersusExplicitNull(t *testing.T) {
	t.Parallel()

	type doomed struct {
		StartedOn **string `json:"started_on,omitempty"`
	}

	var absent, null doomed
	require.NoError(t, json.Unmarshal([]byte(`{}`), &absent))
	require.NoError(t, json.Unmarshal([]byte(`{"started_on":null}`), &null))

	assert.Nil(t, absent.StartedOn)
	assert.Nil(t, null.StartedOn,
		"**string now distinguishes absent from null; contracts/records.md's mechanism can be restored")

	// Not even pre-populating the outer pointer rescues it: the decoder zeroes
	// the whole chain rather than the value at the end of it.
	inner := "2026-01-01"
	outer := &inner
	preset := doomed{StartedOn: &outer}
	require.NoError(t, json.Unmarshal([]byte(`{"started_on":null}`), &preset))
	assert.Nil(t, preset.StartedOn)
}

// omitzero is what carries the absent state on the way out, and it reads
// IsZero() rather than comparing to the zero value — which is why Optional[T]
// implements it.
func TestOmitzeroReadsIsZero(t *testing.T) {
	t.Parallel()

	type carrier struct {
		Field Optional[string] `json:"field,omitzero"`
	}

	absent, err := json.Marshal(carrier{})
	require.NoError(t, err)
	assert.Equal(t, `{}`, string(absent))

	cleared, err := json.Marshal(carrier{Field: Cleared[string]()})
	require.NoError(t, err)
	assert.Equal(t, `{"field":null}`, string(cleared))
}

// The unknown-member rejection reaches inside a custom unmarshaler, which is
// what makes Optional[T] safe to use on a nested DTO in a later phase.
func TestRejectUnknownMembersReachesInsideAnOptional(t *testing.T) {
	t.Parallel()

	type inner struct {
		A string `json:"a"`
	}
	type outer struct {
		Nested Optional[inner] `json:"nested,omitzero"`
	}

	var accepted outer
	require.NoError(t, json.Unmarshal([]byte(`{"nested":{"a":"1"}}`), &accepted, json.RejectUnknownMembers(true)))

	var refused outer
	err := json.Unmarshal([]byte(`{"nested":{"b":"1"}}`), &refused, json.RejectUnknownMembers(true))
	require.Error(t, err, "the option stopped at the custom unmarshaler, so a nested DTO accepts anything")
}
