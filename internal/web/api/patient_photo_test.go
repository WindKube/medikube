package api_test

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T051. contracts/patient-photo.md's three operations, driven through
// tests.ApiScenario exactly as patients_test.go drives the CRUD family:
// detection is content-based (FR-008), a refusal never carries the uploaded
// filename anywhere a caller or a log line could read it (FR-046), and a
// download is never cacheable and never named after what was uploaded.

func patientPhotoURL(id string) string { return patientURL(id) + "/photo" }

// onePixelPNG is the same one-pixel fixture
// internal/service/patient/patienttest's contract accepts, decoded locally so
// this file needs no import of a test-only package from another module.
const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// notAnImage is the PDF-shaped fixture: enough bytes that content sniffing
// has something to look at, none of them a real image signature.
var notAnImage = []byte("%PDF-1.4 this is not a photograph, whatever its name claims")

func onePixelPNG(t *testing.T) []byte {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	require.NoError(t, err)

	return raw
}

// multipartPhoto builds a one-part multipart/form-data body named "photo",
// answering the body and the Content-Type header value carrying its boundary.
func multipartPhoto(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("photo", filename)
	require.NoError(t, err)

	_, err = part.Write(content)
	require.NoError(t, err)

	require.NoError(t, writer.Close())

	return &buf, writer.FormDataContentType()
}

// TestUploadingAPhotoAcceptsWhatItSniffsAndRejectsWhatItDoesNot is the content
// vs. filename split FR-008 turns on: a PNG named .jpg is accepted, a PDF
// named .jpg is refused, and the refusal is a 415 that never lands on disk.
func TestUploadingAPhotoAcceptsWhatItSniffsAndRejectsWhatItDoesNot(t *testing.T) {
	t.Parallel()

	t.Run("a png renamed to jpg is accepted", func(t *testing.T) {
		t.Parallel()

		headers, before := patientSignedIn(testsupport.AccountAEmail)

		body, contentType := multipartPhoto(t, "totally-a.jpg", onePixelPNG(t))
		headers["Content-Type"] = contentType

		self := patientURL(testsupport.AccountAPatientChildID)

		scenario := tests.ApiScenario{
			Name: "a png renamed to jpg is accepted", Method: http.MethodPut, URL: patientPhotoURL(testsupport.AccountAPatientChildID),
			Headers: headers, Body: body,
			ExpectedStatus: http.StatusOK,
			ExpectedContent: []string{
				`"photo_url":"` + self + `/photo?size=100x100t"`,
				`"sizes":[`,
			},
			BeforeTestFunc: before,
		}

		runPatients(t, scenario)
	})

	t.Run("a pdf renamed to jpg is refused and nothing is stored", func(t *testing.T) {
		t.Parallel()

		headers, before := patientSignedIn(testsupport.AccountAEmail)

		body, contentType := multipartPhoto(t, "secret-diagnosis.jpg", notAnImage)
		headers["Content-Type"] = contentType

		scenario := tests.ApiScenario{
			Name: "a pdf renamed to jpg is refused", Method: http.MethodPut, URL: patientPhotoURL(testsupport.AccountAPatientChildID),
			Headers: headers, Body: body,
			ExpectedStatus: http.StatusUnsupportedMediaType,
			NotExpectedContent: []string{
				"secret-diagnosis",
			},
			BeforeTestFunc: before,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				t.Helper()

				record, err := app.FindRecordById(store.PatientCollection, testsupport.AccountAPatientChildID)
				require.NoError(t, err)
				assert.Empty(t, record.GetString(store.PatientPhoto), "the refused upload was stored anyway")
			},
		}

		runPatients(t, scenario)
	})
}

// TestARefusedUploadsFilenameReachesNeitherTheBodyNorTheLogStream is FR-046 /
// SC-008: a filename is PHI, and PocketBase's own validation message embeds it
// — this asserts the mapping this package does strips it, in both places it
// could otherwise leak.
func TestARefusedUploadsFilenameReachesNeitherTheBodyNorTheLogStream(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer

	instance := apitest.New(t, apitest.WithLogWriter(&log))
	handler := testsupport.NewEdgeHandler(t, instance.App)

	caller := &caller{t: t, app: instance.App, handler: handler,
		token: testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)}

	body, contentType := multipartPhoto(t, "the-patients-secret-name.jpg", notAnImage)

	request := caller.do(http.MethodPut, patientPhotoURL(testsupport.AccountAPatientChildID), body.String(),
		map[string]string{"Content-Type": contentType})

	assert.Equal(t, http.StatusUnsupportedMediaType, request.Status, request.Body)
	assert.NotContains(t, request.Body, "the-patients-secret-name")
	assert.NotContains(t, log.String(), "the-patients-secret-name")
}

// TestGettingAPhotoServesTheGenericHeadersAndNeverTheUploadedName is
// getPatientPhoto's own contract: never cacheable, and the Content-Disposition
// filename is the fixed generic one, never what was uploaded.
func TestGettingAPhotoServesTheGenericHeadersAndNeverTheUploadedName(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	owner := &caller{t: t, app: instance.App, handler: handler,
		token: testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)}

	body, contentType := multipartPhoto(t, "a-name-nobody-should-see.png", onePixelPNG(t))

	put := owner.do(http.MethodPut, patientPhotoURL(testsupport.AccountAPatientChildID), body.String(),
		map[string]string{"Content-Type": contentType})
	require.Equal(t, http.StatusOK, put.Status, put.Body)

	get := owner.get(patientPhotoURL(testsupport.AccountAPatientChildID))

	require.Equal(t, http.StatusOK, get.Status, get.Body)
	assert.NotContains(t, get.Body, "a-name-nobody-should-see")
	assert.Equal(t, "private, no-store", get.Header.Get("Cache-Control"))
	disposition := get.Header.Get("Content-Disposition")
	assert.Contains(t, disposition, "filename=")
	assert.Contains(t, disposition, "photo.jpg")
	assert.NotContains(t, disposition, "a-name-nobody-should-see")
}

// TestDeletingAPhotoIsIdempotent covers deletePatientPhoto's own 204, twice
// over: once for a real removal, once for a patient with no photo at all.
func TestDeletingAPhotoIsIdempotent(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	scenario := tests.ApiScenario{
		Name: "deleting a photo that never existed is still 204", Method: http.MethodDelete,
		URL: patientPhotoURL(testsupport.AccountAPatientChildID), Headers: headers,
		ExpectedStatus: http.StatusNoContent,
		BeforeTestFunc: before,
	}

	runPatients(t, scenario)
}

// TestAnUnrecognisedSizeIsRefused is getPatientPhoto's ?size= validation.
func TestAnUnrecognisedSizeIsRefused(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	scenario := tests.ApiScenario{
		Name: "an unrecognised size is refused", Method: http.MethodGet,
		URL: patientPhotoURL(testsupport.AccountAPatientSelfID) + "?size=huge", Headers: headers,
		ExpectedStatus:  http.StatusUnprocessableEntity,
		ExpectedContent: []string{`"code":"` + domain.CodeInvalidValue + `"`},
		BeforeTestFunc:  before,
	}

	runPatients(t, scenario)
}
