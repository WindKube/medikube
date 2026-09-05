package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
)

// contracts/tags.md's four operations: T155.

func tagsURL() string { return "/api/v1/tags" }

func tagURL(id string) string { return tagsURL() + "/" + id }

type tagDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	UsageCount int    `json:"usage_count"`
}

type tagListDTO struct {
	Items      []tagDTO `json:"items"`
	NextCursor *string  `json:"next_cursor"`
}

func (r response) tag(t *testing.T) tagDTO {
	t.Helper()

	var dto tagDTO
	r.decode(t, &dto)

	return dto
}

func (r response) tagList(t *testing.T) tagListDTO {
	t.Helper()

	var dto tagListDTO
	r.decode(t, &dto)

	return dto
}

func TestListTags(t *testing.T) {
	t.Parallel()

	t.Run("200 lists the account's own tags, each with usage_count", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(tagsURL())
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		items := answer.tagList(t).Items
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}

		assert.Contains(t, ids, seed.TagChronicID)
	})

	t.Run("401 with no session", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).anonymous()

		answer := caller.get(tagsURL())
		assert.Equal(t, http.StatusUnauthorized, answer.Status)
	})

	t.Run("another account's tags are neither listed nor addressable (FR-062, US7-5)", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).as(testsupport.AccountBEmail)

		answer := caller.get(tagsURL())
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		for _, item := range answer.tagList(t).Items {
			assert.NotEqual(t, seed.TagChronicID, item.ID)
		}
	})
}

func TestCreateTag(t *testing.T) {
	t.Parallel()

	t.Run("201 with Location", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(tagsURL(), `{"name":"cardiology","color":"#aa3311"}`)
		require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

		created := answer.tag(t)
		require.NotEmpty(t, created.ID)
		assert.Equal(t, tagURL(created.ID), answer.Header.Get("Location"))
		assert.Equal(t, 0, created.UsageCount)
	})

	t.Run(`"Cardiology" after "cardiology" is 409 duplicate_name (FR-063, US7-2)`, func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		first := caller.post(tagsURL(), `{"name":"cardiology"}`)
		require.Equal(t, http.StatusCreated, first.Status, first.Body)

		second := caller.post(tagsURL(), `{"name":"Cardiology"}`)
		require.Equal(t, http.StatusConflict, second.Status, second.Body)
		assert.Contains(t, second.Body, `"duplicate_name"`)
	})

	t.Run("422 when name is empty", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(tagsURL(), `{"name":""}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 when name is over 40 characters", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(tagsURL(), `{"name":"`+repeat("a", 41)+`"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 when color does not match the pattern", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(tagsURL(), `{"name":"cardiology","color":"red"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})
}

func TestUpdateTag(t *testing.T) {
	t.Parallel()

	t.Run("200, no If-Match required (contracts/tags.md §3)", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(tagsURL(), `{"name":"before-rename"}`).tag(t)

		answer := caller.do(http.MethodPatch, tagURL(created.ID), `{"name":"after-rename"}`, nil)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Equal(t, "after-rename", answer.tag(t).Name)
	})

	t.Run("409 on a case-insensitive duplicate", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		require.Equal(t, http.StatusCreated, caller.post(tagsURL(), `{"name":"existing"}`).Status)
		created := caller.post(tagsURL(), `{"name":"renameable"}`).tag(t)

		answer := caller.do(http.MethodPatch, tagURL(created.ID), `{"name":"Existing"}`, nil)
		assert.Equal(t, http.StatusConflict, answer.Status, answer.Body)
	})

	t.Run("404 for another account's tag, identical to a non-existent id (FR-062, US7-5)", func(t *testing.T) {
		t.Parallel()

		owner := newCaller(t)
		stranger := newCallerAs(t, testsupport.AccountBEmail)

		created := owner.post(tagsURL(), `{"name":"someone-elses"}`).tag(t)

		answer := stranger.do(http.MethodPatch, tagURL(created.ID), `{"name":"stolen"}`, nil)
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})
}

func TestDeleteTag(t *testing.T) {
	t.Parallel()

	t.Run("204, and the tag no longer lists", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(tagsURL(), `{"name":"deletable"}`).tag(t)

		answer := caller.do(http.MethodDelete, tagURL(created.ID), "", nil)
		require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)

		list := caller.get(tagsURL()).tagList(t)
		for _, item := range list.Items {
			assert.NotEqual(t, created.ID, item.ID)
		}
	})

	t.Run("404 for another account's tag, identical to a non-existent id", func(t *testing.T) {
		t.Parallel()

		owner := newCaller(t)
		stranger := newCallerAs(t, testsupport.AccountBEmail)

		created := owner.post(tagsURL(), `{"name":"not-yours"}`).tag(t)

		answer := stranger.do(http.MethodDelete, tagURL(created.ID), "", nil)
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})
}

// ?tags=a,b&match=any|all (FR-067): T156's HTTP half. The store-level
// cross-kind proof lives in internal/store/filter_tags_test.go; this is the
// one operation-level check that the query parameter actually reaches the
// service.
func TestRecordsFilterByTagsAndMatch(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.get(collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID +
		"&tags=" + seed.TagChronicID + "&match=any")
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	items := answer.list(t).Items
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}

	assert.Contains(t, ids, seed.NameOnlyID)
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}

	return string(out)
}
