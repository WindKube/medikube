package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/testsupport"
)

// contracts/practitioners.md's five operations: T128.

func practitionersURL() string { return "/api/v1/practitioners" }

func practitionerURL(id string) string { return practitionersURL() + "/" + id }

type facilityRefDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type practitionerUsageDTO struct {
	Patients int `json:"patients"`
	Records  int `json:"records"`
}

type practitionerDTO struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Specialty string               `json:"specialty"`
	Facility  *facilityRefDTO      `json:"facility"`
	UpdatedAt string               `json:"updated_at"`
	Phone     string               `json:"phone"`
	Email     string               `json:"email"`
	Website   string               `json:"website"`
	Notes     string               `json:"notes"`
	Usage     practitionerUsageDTO `json:"usage"`
}

type practitionerListDTO struct {
	Items      []practitionerDTO `json:"items"`
	NextCursor *string           `json:"next_cursor"`
	Total      *int              `json:"total"`
}

func (r response) practitioner(t *testing.T) practitionerDTO {
	t.Helper()

	var dto practitionerDTO
	r.decode(t, &dto)

	return dto
}

func (r response) practitionerList(t *testing.T) practitionerListDTO {
	t.Helper()

	var dto practitionerListDTO
	r.decode(t, &dto)

	return dto
}

func TestListPractitioners(t *testing.T) {
	t.Parallel()

	t.Run("200 always, an empty directory answers items: []", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).as(testsupport.AccountCEmail)

		answer := caller.get(practitionersURL())
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		list := answer.practitionerList(t)
		assert.Empty(t, list.Items)
		assert.Contains(t, answer.Body, `"items":[]`)
	})

	t.Run("200 lists the account's own directory", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(practitionersURL())
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		list := answer.practitionerList(t)

		ids := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			ids = append(ids, item.ID)
		}

		assert.Contains(t, ids, testsupport.AccountAPractitionerID)
		assert.NotContains(t, ids, testsupport.AccountBPractitionerID)
	})

	t.Run("401 with no session", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).anonymous()

		answer := caller.get(practitionersURL())
		assert.Equal(t, http.StatusUnauthorized, answer.Status)
	})

	t.Run("?q= is a case-insensitive substring of name", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(practitionersURL() + "?q=ngozi")
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		list := answer.practitionerList(t)
		require.Len(t, list.Items, 1)
		assert.Equal(t, testsupport.AccountAPractitionerID, list.Items[0].ID)
	})

	t.Run("?q= never leaks another account's practitioner (FR-037, US5-6, SC-014)", func(t *testing.T) {
		t.Parallel()

		strangerCaller := newCaller(t).as(testsupport.AccountBEmail)

		answer := strangerCaller.get(practitionersURL() + "?q=ngozi")
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		list := answer.practitionerList(t)
		assert.Empty(t, list.Items)
		assert.Contains(t, answer.Body, `"items":[]`)
	})

	t.Run("?specialty= narrows the list", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(practitionersURL() + "?specialty=family_medicine")
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		list := answer.practitionerList(t)
		require.Len(t, list.Items, 1)
		assert.Equal(t, testsupport.AccountAPractitionerID, list.Items[0].ID)

		answer = caller.get(practitionersURL() + "?specialty=cardiology")
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Empty(t, answer.practitionerList(t).Items)
	})

	t.Run("?specialty= outside the vocabulary is 422 invalid_value", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(practitionersURL() + "?specialty=not-a-specialty")
		require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

		envelope := answer.envelope(t)
		assert.Equal(t, domain.CodeValidationFailed, envelope.Error.Code)
	})

	t.Run("?facility= narrows the list", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(practitionersURL() + "?facility=" + testsupport.AccountAFacilityPracticeID)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		list := answer.practitionerList(t)
		require.Len(t, list.Items, 1)
		assert.Equal(t, testsupport.AccountAPractitionerID, list.Items[0].ID)

		answer = caller.get(practitionersURL() + "?facility=" + testsupport.AccountAFacilityPharmacyID)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Empty(t, answer.practitionerList(t).Items)
	})
}

func TestCreatePractitioner(t *testing.T) {
	t.Parallel()

	t.Run("201 with Location and ETag", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(practitionersURL(), `{"name":"Dr. New Practitioner"}`)
		require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

		created := answer.practitioner(t)
		require.NotEmpty(t, created.ID)
		assert.Equal(t, practitionerURL(created.ID), answer.Header.Get("Location"))
		assert.NotEmpty(t, answer.Header.Get("ETag"))
		assert.Equal(t, practitionerUsageDTO{}, created.Usage)
	})

	t.Run("409 conflict on the same name and specialty, including when neither has one", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		first := caller.post(practitionersURL(), `{"name":"Dr. Duplicate"}`)
		require.Equal(t, http.StatusCreated, first.Status, first.Body)

		second := caller.post(practitionersURL(), `{"name":"Dr. Duplicate"}`)
		assert.Equal(t, http.StatusConflict, second.Status, second.Body)
	})

	t.Run("422 when name is missing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(practitionersURL(), `{}`)
		require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 when specialty is outside the vocabulary", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(practitionersURL(), `{"name":"Dr. Bad Specialty","specialty":"not-real"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 on malformed email", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(practitionersURL(), `{"name":"Dr. Bad Email","email":"not-an-email"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 on an unknown field", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(practitionersURL(), `{"name":"Dr. X","owner":"someone-else"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("404 when facility names a facility the actor does not own (FR-042)", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(practitionersURL(), `{"name":"Dr. Y","facility":"`+testsupport.AccountAFacilityPracticeID+`x"}`)
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})

	t.Run("201 the facility ref renders when the practitioner names one", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(practitionersURL(),
			`{"name":"Dr. With Facility","facility":"`+testsupport.AccountAFacilityPracticeID+`"}`)
		require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

		created := answer.practitioner(t)
		require.NotNil(t, created.Facility)
		assert.Equal(t, testsupport.AccountAFacilityPracticeID, created.Facility.ID)
		assert.Equal(t, "practice", created.Facility.Kind)
	})

	t.Run("401 with no session", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).anonymous()

		answer := caller.post(practitionersURL(), `{"name":"Nobody"}`)
		assert.Equal(t, http.StatusUnauthorized, answer.Status)
	})
}

func TestGetPractitioner(t *testing.T) {
	t.Parallel()

	t.Run("200 with usage", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(practitionerURL(testsupport.AccountAPractitionerID))
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		found := answer.practitioner(t)
		assert.Equal(t, testsupport.AccountAPractitionerID, found.ID)
		assert.NotEmpty(t, answer.Header.Get("ETag"))
	})

	t.Run("404 not owned or not existing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(practitionerURL(testsupport.AccountBPractitionerID))
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)

		answer = caller.get(practitionerURL("mkprcnobody00001"))
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})

	t.Run("401 with no session", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).anonymous()

		answer := caller.get(practitionerURL(testsupport.AccountAPractitionerID))
		assert.Equal(t, http.StatusUnauthorized, answer.Status)
	})
}

func TestUpdatePractitioner(t *testing.T) {
	t.Parallel()

	t.Run("200 with a new ETag", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(practitionersURL(), `{"name":"Dr. Update Me"}`).practitioner(t)
		version := caller.get(practitionerURL(created.ID)).etag(t)

		answer := caller.patch(practitionerURL(created.ID), `{"phone":"+1 555 0100"}`, version)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Equal(t, "+1 555 0100", answer.practitioner(t).Phone)
		assert.NotEqual(t, version, answer.Header.Get("ETag"))
	})

	t.Run("412 on a stale If-Match, carrying the current representation", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(practitionersURL(), `{"name":"Dr. Stale"}`).practitioner(t)

		answer := caller.patch(practitionerURL(created.ID), `{"phone":"+1 555 0101"}`, `"not-the-real-version"`)
		require.Equal(t, http.StatusPreconditionFailed, answer.Status, answer.Body)
	})

	t.Run("422 with If-Match missing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(practitionersURL(), `{"name":"Dr. No IfMatch"}`).practitioner(t)

		answer := caller.do(http.MethodPatch, practitionerURL(created.ID), `{"phone":"+1"}`, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("facility can be cleared with an explicit null", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(practitionersURL(),
			`{"name":"Dr. Clearable","facility":"`+testsupport.AccountAFacilityPracticeID+`"}`).practitioner(t)
		require.NotNil(t, created.Facility)

		version := caller.get(practitionerURL(created.ID)).etag(t)

		answer := caller.patch(practitionerURL(created.ID), `{"facility":null}`, version)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Nil(t, answer.practitioner(t).Facility)
	})

	t.Run("404 not owned or not existing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.patch(practitionerURL(testsupport.AccountBPractitionerID), `{"phone":"+1"}`, `"anything"`)
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})
}

func TestDeletePractitioner(t *testing.T) {
	t.Parallel()

	t.Run("204, and every referencing record survives with the reference cleared (US5-5)", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(practitionersURL(), `{"name":"Dr. Deletable"}`).practitioner(t)
		version := caller.get(practitionerURL(created.ID)).etag(t)

		answer := caller.delete(practitionerURL(created.ID), version)
		assert.Equal(t, http.StatusNoContent, answer.Status, answer.Body)

		missing := caller.get(practitionerURL(created.ID))
		assert.Equal(t, http.StatusNotFound, missing.Status)
	})

	t.Run("deleting one account's practitioner never touches another account's identically named one", func(t *testing.T) {
		t.Parallel()

		callerA := newCaller(t)
		callerB := callerA.as(testsupport.AccountBEmail)

		madeA := callerA.post(practitionersURL(), `{"name":"Dr. Same Name"}`).practitioner(t)
		madeB := callerB.post(practitionersURL(), `{"name":"Dr. Same Name"}`).practitioner(t)

		versionA := callerA.get(practitionerURL(madeA.ID)).etag(t)

		answer := callerA.delete(practitionerURL(madeA.ID), versionA)
		require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)

		stillThere := callerB.get(practitionerURL(madeB.ID))
		assert.Equal(t, http.StatusOK, stillThere.Status, stillThere.Body)
	})

	t.Run("412 on a stale If-Match", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(practitionersURL(), `{"name":"Dr. Stale Delete"}`).practitioner(t)

		answer := caller.delete(practitionerURL(created.ID), `"not-the-real-version"`)
		assert.Equal(t, http.StatusPreconditionFailed, answer.Status, answer.Body)
	})

	t.Run("404 not owned or not existing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.delete(practitionerURL(testsupport.AccountBPractitionerID), `"anything"`)
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})
}
