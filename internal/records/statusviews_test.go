package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/records"
)

// T179, T183a. contracts/pages.md §3.5's seven-entry catalogue is the single
// source both the per-kind filters (already declared on each kind's own
// registration) and the SmokeVariants read from, so a status view added here
// with no route to carry it fails at boot (internal/httproute's
// describePage), and a status view with a malformed URL fails right here.
func TestTheCatalogueIsExactlySevenEntries(t *testing.T) {
	t.Parallel()

	require.Len(t, records.StatusViews, 7)
}

func TestEveryEntryDeclaresAValidKindAndQuery(t *testing.T) {
	t.Parallel()

	seenNames := map[string]bool{}
	seenKinds := map[string]bool{}

	for _, view := range records.StatusViews {
		t.Run(view.Name, func(t *testing.T) {
			assert.True(t, view.Kind.Valid(), "the catalogue names a kind internal/domain/kind does not declare")
			assert.NotEmpty(t, view.Query)
			assert.NotEmpty(t, view.Name)
		})

		assert.False(t, seenNames[view.Name], "duplicate status view name %q", view.Name)
		seenNames[view.Name] = true

		assert.False(t, seenKinds[view.Kind.Enum()], "two status views narrow the same kind's list: %s", view.Kind)
		seenKinds[view.Kind.Enum()] = true
	}
}

func TestSmokeURLIsAConcreteURLCarryingTheQueryAndThePatient(t *testing.T) {
	t.Parallel()

	for _, view := range records.StatusViews {
		t.Run(view.Name, func(t *testing.T) {
			url := view.SmokeURL("mkpatamara00001")

			assert.Contains(t, url, "/"+view.Kind.Segment()+"?")
			assert.Contains(t, url, view.Query)
			assert.Contains(t, url, "patient=mkpatamara00001")
			assert.NotContains(t, url, "{")
			assert.NotContains(t, url, "}")
		})
	}
}

func TestStatusViewForFindsAndMisses(t *testing.T) {
	t.Parallel()

	view, found := records.StatusViewFor(records.StatusViews[0].Kind)
	require.True(t, found)
	assert.Equal(t, records.StatusViews[0], view)

	_, found = records.StatusViewFor("no-such-kind")
	assert.False(t, found)
}
