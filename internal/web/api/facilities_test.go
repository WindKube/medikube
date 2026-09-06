package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/testsupport"
	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
)

// contracts/facilities.md's five operations: T129.

func facilitiesURL() string { return "/api/v1/facilities" }

func facilityURL(id string) string { return facilitiesURL() + "/" + id }

type facilityUsageDTO struct {
	Practitioners int `json:"practitioners"`
	Records       int `json:"records"`
}

type facilityDTO struct {
	ID           string           `json:"id"`
	Kind         string           `json:"kind"`
	Name         string           `json:"name"`
	Brand        string           `json:"brand"`
	City         string           `json:"city"`
	Phone        string           `json:"phone"`
	UpdatedAt    string           `json:"updated_at"`
	Street       string           `json:"street"`
	Region       string           `json:"region"`
	PostalCode   string           `json:"postal_code"`
	Country      string           `json:"country"`
	Fax          string           `json:"fax"`
	Email        string           `json:"email"`
	Website      string           `json:"website"`
	PortalURL    string           `json:"portal_url"`
	Hours        string           `json:"hours"`
	Open24h      bool             `json:"open_24h"`
	DriveThrough bool             `json:"drive_through"`
	Services     string           `json:"services"`
	Notes        string           `json:"notes"`
	Usage        facilityUsageDTO `json:"usage"`
}

type facilityListDTO struct {
	Items      []facilityDTO `json:"items"`
	NextCursor *string       `json:"next_cursor"`
	Total      *int          `json:"total"`
}

func (r response) facility(t *testing.T) facilityDTO {
	t.Helper()

	var dto facilityDTO
	r.decode(t, &dto)

	return dto
}

func (r response) facilityList(t *testing.T) facilityListDTO {
	t.Helper()

	var dto facilityListDTO
	r.decode(t, &dto)

	return dto
}

func TestListFacilities(t *testing.T) {
	t.Parallel()

	t.Run("200 always, an empty directory answers items: []", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).as(testsupport.AccountBEmail)

		answer := caller.get(facilitiesURL())
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, `"items":[]`)
	})

	t.Run("200 lists the account's own facilities", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(facilitiesURL())
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		items := answer.facilityList(t).Items
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}

		assert.Contains(t, ids, testsupport.AccountAFacilityPracticeID)
		assert.Contains(t, ids, testsupport.AccountAFacilityPharmacyID)
	})

	t.Run("401 with no session", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).anonymous()

		answer := caller.get(facilitiesURL())
		assert.Equal(t, http.StatusUnauthorized, answer.Status)
	})

	t.Run("?q= matches name and brand, case-insensitively", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		byName := caller.get(facilitiesURL() + "?q=riverside")
		require.Equal(t, http.StatusOK, byName.Status, byName.Body)
		assert.Len(t, byName.facilityList(t).Items, 2)

		byBrand := caller.get(facilitiesURL() + "?q=boots")
		require.Equal(t, http.StatusOK, byBrand.Status, byBrand.Body)

		items := byBrand.facilityList(t).Items
		require.Len(t, items, 1)
		assert.Equal(t, testsupport.AccountAFacilityPharmacyID, items[0].ID)
	})

	t.Run("?q= never leaks another account's facility (FR-037, US5-6, SC-014)", func(t *testing.T) {
		t.Parallel()

		stranger := newCaller(t).as(testsupport.AccountBEmail)

		answer := stranger.get(facilitiesURL() + "?q=riverside")
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, `"items":[]`)
	})

	t.Run("?kind= narrows the list", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(facilitiesURL() + "?kind=pharmacy")
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		items := answer.facilityList(t).Items
		require.Len(t, items, 1)
		assert.Equal(t, testsupport.AccountAFacilityPharmacyID, items[0].ID)
	})

	t.Run("?kind= outside the vocabulary is 422 invalid_value", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(facilitiesURL() + "?kind=spaceship")
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})
}

func TestCreateFacility(t *testing.T) {
	t.Parallel()

	t.Run("201 with Location and ETag", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(facilitiesURL(), `{"kind":"hospital","name":"General Hospital"}`)
		require.Equal(t, http.StatusCreated, answer.Status, answer.Body)

		created := answer.facility(t)
		require.NotEmpty(t, created.ID)
		assert.Equal(t, facilityURL(created.ID), answer.Header.Get("Location"))
		assert.NotEmpty(t, answer.Header.Get("ETag"))
	})

	t.Run("there is no uniqueness constraint on name (FR-035, US5-3)", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		first := caller.post(facilitiesURL(), `{"kind":"pharmacy","name":"Chain Pharmacy","city":"Lagos"}`)
		require.Equal(t, http.StatusCreated, first.Status, first.Body)

		second := caller.post(facilitiesURL(), `{"kind":"pharmacy","name":"Chain Pharmacy","city":"Abuja"}`)
		require.Equal(t, http.StatusCreated, second.Status, second.Body)

		assert.NotEqual(t, first.facility(t).ID, second.facility(t).ID)

		answer := caller.get(facilitiesURL() + "?q=" + urlValue("Chain Pharmacy"))
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Len(t, answer.facilityList(t).Items, 2)
	})

	t.Run("422 when kind is missing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(facilitiesURL(), `{"name":"No Kind"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 when kind is outside the vocabulary", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(facilitiesURL(), `{"kind":"spaceship","name":"Nope"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 when name is missing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(facilitiesURL(), `{"kind":"lab"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("422 on malformed website", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(facilitiesURL(), `{"kind":"lab","name":"Bad Website","website":"not-a-url"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("401 with no session", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).anonymous()

		answer := caller.post(facilitiesURL(), `{"kind":"lab","name":"Nobody"}`)
		assert.Equal(t, http.StatusUnauthorized, answer.Status)
	})
}

func TestGetFacility(t *testing.T) {
	t.Parallel()

	t.Run("200 with usage", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.get(facilityURL(testsupport.AccountAFacilityPracticeID))
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		found := answer.facility(t)
		assert.Equal(t, testsupport.AccountAFacilityPracticeID, found.ID)
		assert.GreaterOrEqual(t, found.Usage.Practitioners, 1)
		assert.NotEmpty(t, answer.Header.Get("ETag"))
	})

	t.Run("404 not owned or not existing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).as(testsupport.AccountBEmail)

		answer := caller.get(facilityURL(testsupport.AccountAFacilityPracticeID))
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)

		answer = caller.get(facilityURL("mkfacnobody0001"))
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})

	t.Run("401 with no session", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).anonymous()

		answer := caller.get(facilityURL(testsupport.AccountAFacilityPracticeID))
		assert.Equal(t, http.StatusUnauthorized, answer.Status)
	})
}

func TestUpdateFacility(t *testing.T) {
	t.Parallel()

	t.Run("200 with a new ETag; changing kind is permitted", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(facilitiesURL(), `{"kind":"practice","name":"Reclassify Me"}`).facility(t)
		version := caller.get(facilityURL(created.ID)).etag(t)

		answer := caller.patch(facilityURL(created.ID), `{"kind":"hospital"}`, version)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Equal(t, "hospital", answer.facility(t).Kind)
		assert.NotEqual(t, version, answer.Header.Get("ETag"))
	})

	t.Run("412 on a stale If-Match, carrying the current representation", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(facilitiesURL(), `{"kind":"lab","name":"Stale Lab"}`).facility(t)

		answer := caller.patch(facilityURL(created.ID), `{"name":"Changed"}`, `"not-the-real-version"`)
		assert.Equal(t, http.StatusPreconditionFailed, answer.Status, answer.Body)
	})

	t.Run("422 with If-Match missing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(facilitiesURL(), `{"kind":"lab","name":"No IfMatch"}`).facility(t)

		answer := caller.do(http.MethodPatch, facilityURL(created.ID), `{"name":"Changed"}`, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	t.Run("404 not owned or not existing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).as(testsupport.AccountBEmail)

		answer := caller.patch(facilityURL(testsupport.AccountAFacilityPracticeID), `{"name":"Changed"}`, `"anything"`)
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})
}

func TestDeleteFacility(t *testing.T) {
	t.Parallel()

	t.Run("204, and referencing practitioners survive with the reference cleared", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		facility := caller.post(facilitiesURL(), `{"kind":"practice","name":"Deletable Facility"}`).facility(t)
		practitioner := caller.post(practitionersURL(),
			`{"name":"Dr. Points At Facility","facility":"`+facility.ID+`"}`).practitioner(t)

		version := caller.get(facilityURL(facility.ID)).etag(t)

		answer := caller.delete(facilityURL(facility.ID), version)
		require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)

		missing := caller.get(facilityURL(facility.ID))
		assert.Equal(t, http.StatusNotFound, missing.Status)

		stillThere := caller.get(practitionerURL(practitioner.ID))
		require.Equal(t, http.StatusOK, stillThere.Status, stillThere.Body)
		assert.Nil(t, stillThere.practitioner(t).Facility)
	})

	t.Run("412 on a stale If-Match", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		created := caller.post(facilitiesURL(), `{"kind":"lab","name":"Stale Delete"}`).facility(t)

		answer := caller.delete(facilityURL(created.ID), `"not-the-real-version"`)
		assert.Equal(t, http.StatusPreconditionFailed, answer.Status, answer.Body)
	})

	t.Run("404 not owned or not existing", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t).as(testsupport.AccountBEmail)

		answer := caller.delete(facilityURL(testsupport.AccountAFacilityPracticeID), `"anything"`)
		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	})
}

// TestCreateFacilityOverDatastarAnswersHTML mirrors patients_test.go's own: a
// Datastar submit gets the form or the list back as text/html on 200, and
// every other caller keeps today's JSON exactly (422/201).
func TestCreateFacilityOverDatastarAnswersHTML(t *testing.T) {
	t.Parallel()

	datastar := map[string]string{"Datastar-Request": "true"}

	t.Run("an invalid create over Datastar answers 200 text/html with the form and the field error", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.do(http.MethodPost, facilitiesURL(), `{}`, datastar)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, ids.DirectoryForm(directory.FacilitySegment, ""))
		assert.Contains(t, answer.Body, "This is required.")
	})

	t.Run("the same invalid create with no Datastar-Request header still answers 422 JSON", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(facilitiesURL(), `{}`)
		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, `"code":"`+domain.CodeValidationFailed+`"`)
	})

	t.Run("a valid create over Datastar answers 200 text/html with the list landmark", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.do(http.MethodPost, facilitiesURL(), `{"kind":"hospital","name":"Datastar Hospital"}`, datastar)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Contains(t, answer.Body, ids.DirectoryList(directory.FacilitySegment))
	})

	t.Run("the same valid create with no Datastar-Request header still answers 201 JSON", func(t *testing.T) {
		t.Parallel()

		caller := newCaller(t)

		answer := caller.post(facilitiesURL(), `{"kind":"hospital","name":"Plain Hospital"}`)
		assert.Equal(t, http.StatusCreated, answer.Status, answer.Body)
	})
}
