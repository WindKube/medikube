package search_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/search"
)

var registered = []kind.Kind{kind.Medication, kind.Allergy, kind.Condition}

func TestNewQuery(t *testing.T) {
	t.Parallel()

	t.Run("a valid query resolves with no kind narrowing meaning every registered kind", func(t *testing.T) {
		t.Parallel()

		q, err := search.NewQuery("warfarin", "patient1", nil, registered)
		require.NoError(t, err)
		assert.Equal(t, "warfarin", q.Term)
		assert.Equal(t, "patient1", q.PatientID)
		assert.Equal(t, registered, q.Kinds)
	})

	t.Run("an empty term is refused", func(t *testing.T) {
		t.Parallel()

		_, err := search.NewQuery("", "patient1", nil, registered)
		requireField(t, err, "q", domain.CodeRequired)
	})

	t.Run("a term over 200 characters is refused", func(t *testing.T) {
		t.Parallel()

		_, err := search.NewQuery(strings.Repeat("a", 201), "patient1", nil, registered)
		requireField(t, err, "q", domain.CodeTooLong)
	})

	t.Run("a term of exactly 200 characters is accepted", func(t *testing.T) {
		t.Parallel()

		_, err := search.NewQuery(strings.Repeat("a", 200), "patient1", nil, registered)
		require.NoError(t, err)
	})

	t.Run("an absent patient is refused", func(t *testing.T) {
		t.Parallel()

		_, err := search.NewQuery("warfarin", "", nil, registered)
		requireField(t, err, "patient", domain.CodeRequired)
	})

	t.Run("a registered kind narrows to exactly what was named, in the order named", func(t *testing.T) {
		t.Parallel()

		q, err := search.NewQuery("warfarin", "patient1", []string{kind.Condition.Segment(), kind.Medication.Segment()}, registered)
		require.NoError(t, err)
		assert.Equal(t, []kind.Kind{kind.Condition, kind.Medication}, q.Kinds)
	})

	t.Run("an unregistered kind is 400 bad_request and never echoes the value", func(t *testing.T) {
		t.Parallel()

		const poison = "a-kind-nobody-serves-sentinel"

		_, err := search.NewQuery("warfarin", "patient1", []string{poison}, registered)
		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrBadRequest))
		assert.NotContains(t, err.Error(), poison)
	})

	t.Run("a kind declared in the kind table but not registered on this build is refused the same way", func(t *testing.T) {
		t.Parallel()

		_, err := search.NewQuery("warfarin", "patient1", []string{string(kind.Vitals)}, registered)
		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrBadRequest))
	})
}

func requireField(t *testing.T, err error, field, code string) {
	t.Helper()

	require.Error(t, err)

	var invalid *domain.ValidationError
	require.True(t, errors.As(err, &invalid))

	for _, f := range invalid.Fields {
		if f.Field == field && f.Code == code {
			return
		}
	}

	t.Fatalf("no field error for %s/%s in %+v", field, code, invalid.Fields)
}
