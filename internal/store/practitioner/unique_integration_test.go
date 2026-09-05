package practitioner_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
)

// T124. Two practitioners, same owner, same name, and — the case that
// matters — no specialty at all on either one (both left at directory.
// Specialty's Go zero value, "").
//
// SQLite treats NULLs as distinct in a unique index, so this test only proves
// anything because the select field stores ” and never NULL for an unset
// specialty (research D-25). A future migration that let the column go back
// to NULL on empty would make this pass for the wrong reason — a second
// create silently succeeding — and this is the test that catches it.
func TestCreateRefusesTheSameNameWithNoSpecialtyAtAllTwice(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	first := directory.Practitioner{OwnerID: h.owner, Name: "Dr. No Specialty"}
	_, err := h.repo.Create(ctx, first)
	require.NoError(t, err)

	second := directory.Practitioner{OwnerID: h.owner, Name: "dr. no specialty"}
	_, err = h.repo.Create(ctx, second)

	assert.ErrorIs(t, err, domain.ErrConflict)
}
