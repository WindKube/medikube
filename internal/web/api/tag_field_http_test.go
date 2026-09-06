package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport/seed"
)

// FR-064, US7: a Datastar form submit carrying `tags` persists them on the
// record and the re-rendered form shows the applied tag as a chip — proving
// the picker mounted on every kind's form (internal/web/page/tagfield.go)
// actually reaches the record it is on, for one representative kind.
func TestTagFieldSubmitPersistsAndReRendersChip(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	target := allergyURL(seed.CriticalAllergyID)

	before := owner.get(target)
	require.Equal(t, http.StatusOK, before.Status, before.Body)
	version := before.etag(t)

	datastar := map[string]string{"Datastar-Request": "true", "If-Match": version}

	answer := owner.do(http.MethodPatch, target, `{"tags":["`+seed.TagChronicID+`"]}`, datastar)
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)
	assert.Contains(t, answer.Header.Get("Content-Type"), "text/html")
	assert.Contains(t, answer.Body, "chronic", "the re-rendered form does not show the applied tag as a chip")
	assert.Contains(t, answer.Body, "Remove chronic", "the applied tag does not render as a removable chip")

	after := owner.get(target)
	require.Equal(t, http.StatusOK, after.Status, after.Body)
	assert.Contains(t, after.Body, `"tags":["`+seed.TagChronicID+`"]`,
		"the tag was not persisted onto the record (replace-set, FR-064)")
	assert.NotContains(t, after.Body, seed.TagFlaggedID,
		"replace-set semantics should have dropped the tag no longer submitted")
}
