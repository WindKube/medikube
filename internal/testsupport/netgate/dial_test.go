//go:build netgate

package netgate

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// onePixelPNG is the smallest valid PNG, reused from
// internal/testsupport/phileak's own fixture: this exercise needs a real
// image the upload pipeline will accept, not a sentinel.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// TestArmActuallyRefusesADial is the trap's own self-check: an empty pass
// below would be indistinguishable from a Control hook nothing ever calls,
// which is exactly the false negative this whole gate exists to not be.
func TestArmActuallyRefusesADial(t *testing.T) {
	trap, restore := Arm()
	t.Cleanup(restore)

	_, err := http.DefaultClient.Get("http://127.0.0.1:1/netgate-self-check")
	require.Error(t, err, "the dial should have been refused by the trap")

	dials := trap.Dials()
	require.Len(t, dials, 1)
	assert.Contains(t, dials[0], "127.0.0.1:1")
}

// TestNoOutboundConnectionEscapesWithNoDestinationConfigured is T159a
// (FR-047). apitest.New wires an instance with neither a Sentry DSN nor an
// OTel endpoint — production's own default when an operator configures
// neither — so this is that instance. The trap is armed before a single
// request is made and every request below is served in-process
// (httptest.NewRecorder over the handler directly, never a real listener),
// so the only way it could fire is the application itself trying to reach
// the network. Anything the trap caught is exactly the leak FR-047 forbids.
func TestNoOutboundConnectionEscapesWithNoDestinationConfigured(t *testing.T) {
	trap, restore := Arm()
	t.Cleanup(restore)

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)
	token := testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)

	c := &caller{t: t, handler: handler, token: token}

	drivePatients(t, c)
	driveDirectory(t, c)

	assert.Empty(t, trap.Dials(), "the exercise dialed out with no destination configured: %v", trap.Dials())
}

// caller drives one request at a time through the handler directly, with no
// socket in the path: the whole point of this exercise is that DRIVING it
// cannot be mistaken for the leak it is trying to catch.
type caller struct {
	t       *testing.T
	handler http.Handler
	token   string
}

func (c *caller) do(method, url string, body []byte, contentType string, extra map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequestWithContext(c.t.Context(), method, url, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}

	for name, value := range extra {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	return rec
}

func jsonBody(t *testing.T, v any) []byte {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	return raw
}

func idOf(t *testing.T, body []byte) string {
	t.Helper()

	var out struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &out), string(body))
	require.NotEmpty(t, out.ID)

	return out.ID
}

func photoUpload(t *testing.T) ([]byte, string) {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(onePixelPNG)
	require.NoError(t, err)

	var buffer bytes.Buffer
	form := multipart.NewWriter(&buffer)
	part, err := form.CreateFormFile("photo", "proband.png")
	require.NoError(t, err)
	_, err = part.Write(raw)
	require.NoError(t, err)
	require.NoError(t, form.Close())

	return buffer.Bytes(), form.FormDataContentType()
}

// drivePatients exercises contracts/patients.md's whole family: create,
// list, get, patch, the photo trio, the chart summary, the active-patient
// switch and delete.
func drivePatients(t *testing.T, c *caller) {
	t.Helper()

	created := c.do(http.MethodPost, "/api/v1/patients", jsonBody(t, api.PatientCreate{
		FirstName: "Netgate", LastName: "Proband", BirthDate: "2000-01-01",
	}), "application/json", nil)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())

	patientID := idOf(t, created.Body.Bytes())
	address := "/api/v1/patients/" + patientID

	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/patients", nil, "", nil).Code)
	got := c.do(http.MethodGet, address, nil, "", nil)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())

	patched := c.do(http.MethodPatch, address, jsonBody(t, api.PatientPatch{Address: ptr("221B Baker Street")}),
		"application/json", map[string]string{"If-Match": got.Header().Get("ETag")})
	require.Equal(t, http.StatusOK, patched.Code, patched.Body.String())

	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, address+"/summary", nil, "", nil).Code)

	photo, contentType := photoUpload(t)
	assert.Equal(t, http.StatusOK, c.do(http.MethodPut, address+"/photo", photo, contentType, nil).Code)
	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, address+"/photo", nil, "", nil).Code)
	assert.Equal(t, http.StatusNoContent, c.do(http.MethodDelete, address+"/photo", nil, "", nil).Code)

	assert.Equal(t, http.StatusOK,
		c.do(http.MethodPut, "/api/v1/me/active-patient", jsonBody(t, api.ActivePatientBody{Patient: &patientID}), "application/json", nil).Code)

	current := c.do(http.MethodGet, address, nil, "", nil)
	deleted := c.do(http.MethodDelete, address, nil, "", map[string]string{"If-Match": current.Header().Get("ETag")})
	assert.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
}

// driveDirectory exercises contracts/practitioners.md and
// contracts/facilities.md: create, list, search, get, patch and delete for
// both directories.
func driveDirectory(t *testing.T, c *caller) {
	t.Helper()

	facility := c.do(http.MethodPost, "/api/v1/facilities",
		jsonBody(t, api.FacilityCreate{Kind: "hospital", Name: "Netgate General"}), "application/json", nil)
	require.Equal(t, http.StatusCreated, facility.Code, facility.Body.String())
	facilityID := idOf(t, facility.Body.Bytes())

	practitioner := c.do(http.MethodPost, "/api/v1/practitioners",
		jsonBody(t, api.PractitionerCreate{Name: "Dr. Netgate"}), "application/json", nil)
	require.Equal(t, http.StatusCreated, practitioner.Code, practitioner.Body.String())
	practitionerID := idOf(t, practitioner.Body.Bytes())

	for _, url := range []string{
		"/api/v1/facilities", "/api/v1/facilities?q=Netgate", "/api/v1/facilities/" + facilityID,
		"/api/v1/practitioners", "/api/v1/practitioners?q=Netgate", "/api/v1/practitioners/" + practitionerID,
	} {
		assert.Equal(t, http.StatusOK, c.do(http.MethodGet, url, nil, "", nil).Code, url)
	}

	facilityNow := c.do(http.MethodGet, "/api/v1/facilities/"+facilityID, nil, "", nil)
	require.Equal(t, http.StatusOK, facilityNow.Code, facilityNow.Body.String())
	facilityPatched := c.do(http.MethodPatch, "/api/v1/facilities/"+facilityID,
		jsonBody(t, api.FacilityPatch{Name: ptr("Netgate General Renamed")}), "application/json",
		map[string]string{"If-Match": facilityNow.Header().Get("ETag")})
	require.Equal(t, http.StatusOK, facilityPatched.Code, facilityPatched.Body.String())

	practitionerNow := c.do(http.MethodGet, "/api/v1/practitioners/"+practitionerID, nil, "", nil)
	require.Equal(t, http.StatusOK, practitionerNow.Code, practitionerNow.Body.String())
	practitionerPatched := c.do(http.MethodPatch, "/api/v1/practitioners/"+practitionerID,
		jsonBody(t, api.PractitionerPatch{Name: ptr("Dr. Netgate II")}), "application/json",
		map[string]string{"If-Match": practitionerNow.Header().Get("ETag")})
	require.Equal(t, http.StatusOK, practitionerPatched.Code, practitionerPatched.Body.String())

	deletePractitioner := c.do(http.MethodDelete, "/api/v1/practitioners/"+practitionerID, nil, "",
		map[string]string{"If-Match": practitionerPatched.Header().Get("ETag")})
	assert.Equal(t, http.StatusNoContent, deletePractitioner.Code, deletePractitioner.Body.String())

	deleteFacility := c.do(http.MethodDelete, "/api/v1/facilities/"+facilityID, nil, "",
		map[string]string{"If-Match": facilityPatched.Header().Get("ETag")})
	assert.Equal(t, http.StatusNoContent, deleteFacility.Code, deleteFacility.Body.String())
}

func ptr[T any](v T) *T { return &v }
