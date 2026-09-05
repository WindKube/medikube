package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T052, FR-042, FR-043, FR-044, FR-045, US1-8, SC-005, SC-006. Every one of
// contracts/patients.md's four CRUD operations and contracts/patient-photo.md's
// three, run as the owner, as a signed-in stranger and as nobody —
// testsupport.RunOwnershipMatrix, the same harness records_authz_test.go
// drives its own six through.
//
// Only seven operations exist in this build: deletePatient is US6 (tasks.md),
// not yet implemented, so it is not one of the cases below.
func TestEveryPatientAndPhotoOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	// The child record: not the self-record, so its version does not move
	// under everything else this test does, and it starts with no
	// photograph, so putPatientPhoto's owner leg is a real store rather than
	// a no-op replace.
	subject := testsupport.AccountAPatientChildID

	owner := &caller{t: t, app: instance.App, handler: handler,
		token: testsupport.UserToken(t, instance.App, testsupport.AccountAEmail)}

	// Seeded so getPatientPhoto and deletePatientPhoto have a real
	// photograph to be refused reaching, rather than the 404 a missing
	// photograph would answer for every caller regardless of ownership.
	body, contentType := multipartPhoto(t, "seed.png", onePixelPNG(t))
	seeded := owner.do(http.MethodPut, patientPhotoURL(subject), body.String(), map[string]string{"Content-Type": contentType})
	require.Equal(t, http.StatusOK, seeded.Status, seeded.Body)

	// The version fetched AFTER seeding the photo: SetPhoto is itself a
	// write, so a precondition read before it would be stale by the time the
	// change leg below sends it.
	current := owner.get(patientURL(subject))
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	precondition := map[string]string{"If-Match": current.etag(t)}

	secrets := []string{subject, "Chiamaka"}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:         handler,
		Owner:           bearer(t, instance.App, testsupport.AccountAEmail),
		Stranger:        bearer(t, instance.App, testsupport.AccountBEmail),
		Normalise:       withoutCorrelationID,
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:             "list every patient",
				Method:           http.MethodGet,
				Path:             patientsURL() + "?limit=100",
				StrangerIsolated: true,
				Secrets:          secrets,
			},
			{
				Name:             "record a patient",
				Method:           http.MethodPost,
				Path:             patientsURL(),
				Body:             `{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01"}`,
				OwnerStatus:      http.StatusCreated,
				StrangerStatus:   http.StatusCreated,
				StrangerIsolated: true,
				Secrets:          secrets,
			},
			{
				Name:        "read one",
				Method:      http.MethodGet,
				Path:        patientURL(subject),
				MissingPath: patientURL(missingPatientID),
				Secrets:     secrets,
			},
			{
				Name:        "change one",
				Method:      http.MethodPatch,
				Path:        patientURL(subject),
				Body:        `{"address":"12 Marina Road"}`,
				ContentType: "application/json",
				Headers:     precondition,
				MissingPath: patientURL(missingPatientID),
				Secrets:     secrets,
			},
			{
				Name:        "upload a photograph",
				Method:      http.MethodPut,
				Path:        patientPhotoURL(subject),
				Body:        body.String(),
				ContentType: contentType,
				MissingPath: patientPhotoURL(missingPatientID),
				Secrets:     secrets,
			},
			{
				Name:        "download the photograph",
				Method:      http.MethodGet,
				Path:        patientPhotoURL(subject),
				MissingPath: patientPhotoURL(missingPatientID),
				Secrets:     secrets,
			},
			{
				Name:        "remove the photograph",
				Method:      http.MethodDelete,
				Path:        patientPhotoURL(subject),
				OwnerStatus: http.StatusNoContent,
				MissingPath: patientPhotoURL(missingPatientID),
				Secrets:     secrets,
			},
		},
	})
}

// TestARefusedPatientOrPhotoRequestWritesOneAuditRow is FR-045's other half:
// the row exists whether or not the caller's own request completes
// (internal/service/access.Authorizer.Patient records it before answering).
// list and create address no existing record, so a stranger is answered
// rather than refused, and neither writes one of these rows.
func TestARefusedPatientOrPhotoRequestWritesOneAuditRow(t *testing.T) {
	t.Parallel()

	// Any value satisfies web.IfMatch's "a precondition is present" check; the
	// authorization refusal happens before the version is ever compared
	// (patient.Service.Update calls authorizer.Patient first), so what is
	// asserted here is not affected by whether this is the record's real one.
	anyPrecondition := map[string]string{"If-Match": `"anything"`}

	for _, tc := range []struct {
		name    string
		method  string
		url     func(subject string) string
		body    string
		headers map[string]string
	}{
		{"a read", http.MethodGet, patientURL, "", nil},
		{"a change", http.MethodPatch, patientURL, `{"address":"12 Marina Road"}`, anyPrecondition},
		{"a photo download", http.MethodGet, patientPhotoURL, "", nil},
		{"a photo removal", http.MethodDelete, patientPhotoURL, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			instance := apitest.New(t)
			handler := testsupport.NewEdgeHandler(t, instance.App)

			subject := testsupport.AccountAPatientChildID

			stranger := &caller{t: t, app: instance.App, handler: handler,
				token: testsupport.UserToken(t, instance.App, testsupport.AccountBEmail)}

			require.Empty(t, apitest.Events(t, stranger.app),
				"the fixture already holds audit rows, so nothing below is attributable to this request")

			answer := stranger.do(tc.method, tc.url(subject), tc.body, tc.headers)

			require.Equal(t, http.StatusNotFound, answer.Status, answer.Body)

			events := apitest.Events(t, stranger.app)
			require.Len(t, events, 1, "the refusal wrote no row, or more than one")

			event := events[0]
			assert.Equal(t, audit.ActionAccessDenied, event.Action)
			assert.Equal(t, audit.TargetKindPatient, event.TargetKind)
			assert.Equal(t, subject, event.TargetID)
			assert.Equal(t, testsupport.AccountBID, event.ActorID)
		})
	}
}

// TestARefusedPhotoUploadWritesOneAuditRow is the same guarantee as
// TestARefusedPatientOrPhotoRequestWritesOneAuditRow's own cases, split out
// because a PUT needs a real multipart body to reach the authorization check
// at all — an empty one is refused on the missing `photo` part first, before
// SetPhoto's own authorizer.Patient call ever runs.
func TestARefusedPhotoUploadWritesOneAuditRow(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	subject := testsupport.AccountAPatientChildID

	stranger := &caller{t: t, app: instance.App, handler: handler,
		token: testsupport.UserToken(t, instance.App, testsupport.AccountBEmail)}

	require.Empty(t, apitest.Events(t, stranger.app))

	body, contentType := multipartPhoto(t, "whatever.png", onePixelPNG(t))
	answer := stranger.do(http.MethodPut, patientPhotoURL(subject), body.String(), map[string]string{"Content-Type": contentType})

	require.Equal(t, http.StatusNotFound, answer.Status, answer.Body)

	events := apitest.Events(t, stranger.app)
	require.Len(t, events, 1)

	assert.Equal(t, audit.ActionAccessDenied, events[0].Action)
	assert.Equal(t, audit.TargetKindPatient, events[0].TargetKind)
	assert.Equal(t, subject, events[0].TargetID)
}

// TestAnAnonymousRequestForAPatientOrPhotoDisclosesNothing is FR-043: a
// caller with no session is refused before any handler runs, and the refusal
// names no resource.
func TestAnAnonymousRequestForAPatientOrPhotoDisclosesNothing(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	subject := testsupport.AccountAPatientChildID

	anonymous := &caller{t: t, app: instance.App, handler: handler}

	for _, tc := range []struct {
		name   string
		method string
		url    string
	}{
		{"the list", http.MethodGet, patientsURL()},
		{"a read", http.MethodGet, patientURL(subject)},
		{"a photo download", http.MethodGet, patientPhotoURL(subject)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answer := anonymous.do(tc.method, tc.url, "", nil)

			assert.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
			assert.NotContains(t, answer.Body, subject)
			assert.NotContains(t, answer.Body, "Chiamaka")
		})
	}
}
