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

	"medikube/internal/domain/kind"
	auditservice "medikube/internal/service/audit"
	auditstore "medikube/internal/store/audit"
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
	driveClinicalKinds(t, c)
	driveTagsAndSearch(t, c)
	driveAuditRetentionPurge(t, instance)

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

// clinicalKindFixtures is every registered kind's minimal create body, the
// same shape internal/store/patient_cascade_test.go's cascadeFixtures builds
// from — gathered here too because this phase's whole endpoint exercise (not
// only phase 001/002's patient and directory surface) has to run under the
// trap (T210a, FR-088).
func clinicalKindFixtures(patientID string) map[string]any {
	occurredOn := "2026-01-10"
	weightKg := 70.0

	return map[string]any{
		kind.Medication.Segment():   api.MedicationCreate{Patient: patientID, Name: "Netgate Medication"},
		kind.Allergy.Segment():      api.AllergyCreate{Patient: patientID, Allergen: "Netgate Allergen", Severity: "mild"},
		kind.Condition.Segment():    api.ConditionCreate{Patient: patientID, Diagnosis: "Netgate Condition", Status: "active"},
		kind.Encounter.Segment():    api.EncounterCreate{Patient: patientID, Reason: "Netgate Encounter", OccurredOn: &occurredOn},
		kind.Procedure.Segment():    api.ProcedureCreate{Patient: patientID, Name: "Netgate Procedure", OccurredOn: &occurredOn, Status: "completed"},
		kind.Treatment.Segment():    api.TreatmentCreate{Patient: patientID, Name: "Netgate Treatment", StartedOn: &occurredOn},
		kind.Symptom.Segment():      api.SymptomCreate{Patient: patientID, Name: "Netgate Symptom", Severity: "moderate", OccurredAt: "2026-01-01T09:00:00Z"},
		kind.Vitals.Segment():       api.VitalsCreate{Patient: patientID, RecordedAt: "2026-01-01T09:00:00Z", WeightKg: &weightKg},
		kind.Immunization.Segment(): api.ImmunizationCreate{Patient: patientID, VaccineName: "Netgate Vaccine", AdministeredOn: &occurredOn},
		kind.Injury.Segment():       api.InjuryCreate{Patient: patientID, Name: "Netgate Injury", BodyPart: "ankle"},
		kind.Insurance.Segment(): api.InsuranceCreate{
			Patient: patientID, Type: "medical", Company: "Netgate Health",
			MemberName: "Netgate Member", MemberID: "MEM-001", EffectiveOn: "2024-01-01",
		},
		kind.Equipment.Segment(): api.EquipmentCreate{Patient: patientID, Name: "Netgate Equipment", Type: "cpap"},
		kind.EmergencyContact.Segment(): api.EmergencyContactCreate{
			Patient: patientID, Name: "Netgate Contact", Relationship: "spouse", Phone: "+1-555-0100",
		},
		kind.FamilyMember.Segment(): api.FamilyMemberCreate{Patient: patientID, Name: "Netgate Relative", Relationship: "aunt"},
	}
}

// driveClinicalKinds walks the six generic operations for every one of the
// fourteen registered kinds, plus US6's own course-medication join, all
// against one freshly created patient — the endpoint surface phase 003 adds
// on top of phase 001/002's patients and directory (T210a, FR-088).
func driveClinicalKinds(t *testing.T, c *caller) {
	t.Helper()

	patient := c.do(http.MethodPost, "/api/v1/patients", jsonBody(t, api.PatientCreate{
		FirstName: "Netgate", LastName: "Clinical", BirthDate: "2000-01-01",
	}), "application/json", nil)
	require.Equal(t, http.StatusCreated, patient.Code, patient.Body.String())
	patientID := idOf(t, patient.Body.Bytes())

	fixtures := clinicalKindFixtures(patientID)
	require.Len(t, fixtures, len(kind.Kinds()), "every registered kind must have a fixture here")

	ids := make(map[string]string, len(fixtures))

	for _, k := range kind.Kinds() {
		segment := k.Segment()
		url := "/api/v1/records/" + segment

		created := c.do(http.MethodPost, url, jsonBody(t, fixtures[segment]), "application/json", nil)
		require.Equalf(t, http.StatusCreated, created.Code, "%s: %s", segment, created.Body.String())

		ids[segment] = idOf(t, created.Body.Bytes())
		address := url + "/" + ids[segment]

		got := c.do(http.MethodGet, address, nil, "", nil)
		require.Equalf(t, http.StatusOK, got.Code, "%s: %s", segment, got.Body.String())

		assert.Equalf(t, http.StatusOK, c.do(http.MethodGet, url+"?patient="+patientID, nil, "", nil).Code, segment)
	}

	driveCourseMedication(t, c, ids[kind.Treatment.Segment()], ids[kind.Medication.Segment()])

	for segment, id := range ids {
		address := "/api/v1/records/" + segment + "/" + id

		current := c.do(http.MethodGet, address, nil, "", nil)
		require.Equalf(t, http.StatusOK, current.Code, "%s: %s", segment, current.Body.String())

		deleted := c.do(http.MethodDelete, address, nil, "", map[string]string{"If-Match": current.Header().Get("ETag")})
		assert.Equalf(t, http.StatusNoContent, deleted.Code, "%s: %s", segment, deleted.Body.String())
	}
}

// driveCourseMedication walks contracts/treatment-medications.md's own three
// routes: attach, read from both ends, detach.
func driveCourseMedication(t *testing.T, c *caller, treatmentID, medicationID string) {
	t.Helper()

	treatmentURL := "/api/v1/records/" + kind.Treatment.Segment() + "/" + treatmentID
	current := c.do(http.MethodGet, treatmentURL, nil, "", nil)
	require.Equal(t, http.StatusOK, current.Code, current.Body.String())

	joinURL := treatmentURL + "/" + kind.Medication.Segment() + "/" + medicationID

	attached := c.do(http.MethodPut, joinURL, jsonBody(t, api.CourseMedicationPut{Dosage: ptr("1 tablet")}),
		"application/json", map[string]string{"If-Match": current.Header().Get("ETag")})
	require.Equal(t, http.StatusCreated, attached.Code, attached.Body.String())

	assert.Equal(t, http.StatusOK,
		c.do(http.MethodGet, treatmentURL+"/"+kind.Medication.Segment(), nil, "", nil).Code)

	detached := c.do(http.MethodDelete, joinURL, nil, "", map[string]string{"If-Match": current.Header().Get("ETag")})
	assert.Equal(t, http.StatusNoContent, detached.Code, detached.Body.String())
}

// driveTagsAndSearch walks contracts/tags.md and contracts/search.md's own
// operations: this phase's other two additions to the endpoint surface,
// neither of which is one of the fourteen kinds.
func driveTagsAndSearch(t *testing.T, c *caller) {
	t.Helper()

	created := c.do(http.MethodPost, "/api/v1/tags", jsonBody(t, map[string]string{"name": "netgate-tag"}),
		"application/json", nil)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())

	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/tags", nil, "", nil).Code)

	patient := c.do(http.MethodPost, "/api/v1/patients", jsonBody(t, api.PatientCreate{
		FirstName: "Netgate", LastName: "Search", BirthDate: "2000-01-01",
	}), "application/json", nil)
	require.Equal(t, http.StatusCreated, patient.Code, patient.Body.String())
	patientID := idOf(t, patient.Body.Bytes())

	assert.Equal(t, http.StatusOK,
		c.do(http.MethodGet, "/api/v1/search?patient="+patientID+"&q=netgate", nil, "", nil).Code)
}

// driveAuditRetentionPurge runs FR-037's nightly purge directly, under the
// same trap, rather than waiting for the scheduler: the purge is this
// phase's one background job with no HTTP request behind it, and it must
// dial out no more than the request-serving path does (T210a).
//
// The scale suite (internal/store/{search,tag,timeline}'s own
// *_scale_test.go) is not run here: each drives a repository directly
// against a tests.NewTestApp with no HTTP client, no Sentry transport and no
// OTel exporter wired, so there is nothing in it capable of a network dial
// in the first place — wrapping it would prove nothing this test does not
// already prove more directly.
func driveAuditRetentionPurge(t *testing.T, instance *apitest.Instance) {
	t.Helper()

	trail, err := auditstore.New(instance.App)
	require.NoError(t, err)

	retention, err := auditservice.NewRetention(trail, apitest.AuditRetentionDays, auditservice.SystemClock{})
	require.NoError(t, err)

	_, err = retention.Purge(t.Context())
	require.NoError(t, err)
}
