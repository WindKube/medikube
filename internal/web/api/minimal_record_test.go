package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
)

// T213, spec Edge Case. A record carrying only its required fields lists,
// opens, corrects, deletes, is searched, is tagged and appears on the
// timeline exactly like a fully populated one, and every screen renders an
// absent optional as absent rather than as a blank, a dash or a zero, for
// every registered kind.
//
// auditKindCases (audit_coverage_test.go) already carries one minimal
// create-body builder per kind — the same shape each kind's own contract
// test builds its recordstest.Fixture.Minimal from — so a kind added there is
// covered here too.
type minimalTagsDTO struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags"`
}

func TestAMinimalRecordBehavesLikeAFullOneAcrossEveryKind(t *testing.T) {
	t.Parallel()

	cases := auditKindCases()
	require.Len(t, cases, len(kind.Kinds()), "every registered kind must have a case in auditKindCases")

	for _, k := range kind.Kinds() {
		kase, ok := cases[k]
		require.Truef(t, ok, "%s has no case in auditKindCases", k)

		t.Run(k.Segment(), func(t *testing.T) {
			t.Parallel()

			caller := newCaller(t)
			patientID := testsupport.AccountAPatientChildID
			collection := "/api/v1/records/" + k.Segment()

			term := "Minimal" + strings.ReplaceAll(k.Segment(), "-", "")

			tag := caller.post(tagsURL(), fmt.Sprintf(`{"name":"minimal-%s"}`, k.Segment()))
			require.Equalf(t, http.StatusCreated, tag.Status, "%s: creating a tag: %s", k, tag.Body)
			tagID := tag.tag(t).ID

			// Create: only what auditKindCases builds, which is each kind's
			// own contract Minimal fixture.
			created := caller.post(collection, kase.create(patientID, term))
			require.Equalf(t, http.StatusCreated, created.Status, "%s: minimal create: %s", k, created.Body)

			id := created.items1(t).ID
			etag := created.etag(t)
			recordURL := collection + "/" + id

			// Lists.
			listed := caller.get(collection + "?patient=" + patientID)
			require.Equalf(t, http.StatusOK, listed.Status, "%s: list: %s", k, listed.Body)
			assert.Containsf(t, listed.Body, id, "%s: the minimal record is not in its own list", k)

			// Opens: the JSON record and the detail page both.
			opened := caller.get(recordURL)
			require.Equalf(t, http.StatusOK, opened.Status, "%s: open: %s", k, opened.Body)

			detailPage := caller.get("/" + k.Segment() + "/" + id)
			require.Equalf(t, http.StatusOK, detailPage.Status, "%s: detail page: %s", k, detailPage.Body)

			landmarkName := k.DetailLandmark()[len(`article[name="`) : len(k.DetailLandmark())-2]
			assert.Containsf(t, detailPage.Body, `aria-label="`+landmarkName+`"`,
				"%s: the detail page does not carry its own landmark", k)

			// A screen renders an absent optional as absent, never as a
			// Go zero value leaking through a templ %v.
			for _, artifact := range []string{"0001-01-01", "<nil>", "NaN"} {
				assert.NotContainsf(t, detailPage.Body, artifact,
					"%s: the detail page rendered a zero-value artifact (%q) for an absent optional", k, artifact)
			}

			// Corrects.
			corrected := caller.patch(recordURL, fmt.Sprintf(`{%q:%q}`, kase.correctionField, term+"-corrected"), etag)
			require.Equalf(t, http.StatusOK, corrected.Status, "%s: correct: %s", k, corrected.Body)
			etag = corrected.etag(t)

			// Is searched.
			found := caller.get(searchURL("patient=" + patientID + "&q=" + term + "-corrected"))
			require.Equalf(t, http.StatusOK, found.Status, "%s: search: %s", k, found.Body)

			result := decodedSearch(t, found)

			var matched bool
			for _, group := range result.Groups {
				for _, item := range group.Items {
					if item.ID == id {
						matched = true
					}
				}
			}
			assert.Truef(t, matched, "%s: the minimal record was not found by its own corrected text", k)

			// Is tagged.
			tagged := caller.patch(recordURL, fmt.Sprintf(`{"tags":[%q]}`, tagID), etag)
			require.Equalf(t, http.StatusOK, tagged.Status, "%s: tag: %s", k, tagged.Body)
			etag = tagged.etag(t)

			afterTag := caller.get(recordURL)
			require.Equal(t, http.StatusOK, afterTag.Status)

			var withTags minimalTagsDTO
			afterTag.decode(t, &withTags)
			assert.Containsf(t, withTags.Tags, tagID, "%s: the applied tag is not on the record", k)

			// Appears on the timeline.
			timeline := caller.get("/timeline?patient=" + patientID)
			require.Equalf(t, http.StatusOK, timeline.Status, "%s: timeline: %s", k, timeline.Body)
			assert.Containsf(t, timeline.Body, "timeline-entry-"+id,
				"%s: the minimal record does not appear on the timeline", k)

			// Deletes.
			deleted := caller.delete(recordURL, etag)
			require.Equalf(t, http.StatusNoContent, deleted.Status, "%s: delete: %s", k, deleted.Body)

			gone := caller.get(recordURL)
			assert.Equalf(t, http.StatusNotFound, gone.Status, "%s: the deleted record is still reachable", k)
		})
	}
}
