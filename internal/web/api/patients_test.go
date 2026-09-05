package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/apitest"
	"medikube/internal/web/views/ids"
)

// T050, T054. contracts/patients.md's four CRUD operations driven through
// tests.ApiScenario, mirroring records_test.go's own shape for the sibling
// kind: every case builds its own tests.TestApp through apitest.Factory,
// because a shared one grows an OnServe handler per case and the chain
// recurses until the goroutine stack ends the process (reconciliation C14).

func patientsURL() string { return "/api/v1/patients" }

func patientURL(id string) string { return patientsURL() + "/" + id }

func runPatients(t *testing.T, s tests.ApiScenario) {
	t.Helper()

	s.TestAppFactory = apitest.Factory()
	s.Test(t)
}

func patientSignedIn(email string) (map[string]string, func(testing.TB, *tests.TestApp, *core.ServeEvent)) {
	headers := map[string]string{}

	return headers, func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		t.Helper()

		headers["Authorization"] = testsupport.UserToken(t, app, email)
	}
}

func TestThePatientFamilyAnswersEveryDocumentedShape(t *testing.T) {
	t.Parallel()

	self := patientURL(testsupport.AccountAPatientSelfID)

	cases := []struct {
		name  string
		build func(headers map[string]string) tests.ApiScenario
	}{
		{
			name: "the list answers every owned patient with an unconditional total and owned_count",
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet, URL: patientsURL() + "?limit=10", Headers: headers,
					ExpectedStatus: http.StatusOK,
					ExpectedContent: []string{
						`"items":[`,
						`"total":3`,
						`"owned_count":3`,
					},
				}
			},
		},
		{
			name: "a create answers the created representation",
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPost, URL: patientsURL(), Headers: headers,
					Body:           strings.NewReader(`{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01"}`),
					ExpectedStatus: http.StatusCreated,
					ExpectedContent: []string{
						`"first_name":"Ngozi"`,
						`"is_self_record":false`,
					},
				}
			},
		},
		{
			name: "a create carrying owner is refused as an unknown member",
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPost, URL: patientsURL(), Headers: headers,
					Body: strings.NewReader(
						`{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01","owner":"` + testsupport.AccountBID + `"}`),
					ExpectedStatus: http.StatusUnprocessableEntity,
					ExpectedContent: []string{
						`"field":"owner"`,
						`"code":"` + domain.CodeUnknownField + `"`,
					},
				}
			},
		},
		{
			name: "a create carrying is_self_record is refused as an unknown member and the existing self-record is unchanged",
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPost, URL: patientsURL(), Headers: headers,
					Body: strings.NewReader(
						`{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01","is_self_record":true}`),
					ExpectedStatus: http.StatusUnprocessableEntity,
					ExpectedContent: []string{
						`"field":"is_self_record"`,
						`"code":"` + domain.CodeUnknownField + `"`,
					},
				}
			},
		},
		{
			name: "a read answers the full representation and omits what was never recorded",
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet, URL: self, Headers: headers,
					ExpectedStatus: http.StatusOK,
					ExpectedContent: []string{
						`"id":"` + testsupport.AccountAPatientSelfID + `"`,
						`"is_self_record":true`,
					},
				}
			},
		},
		{
			name: "an identifier that never existed is a miss",
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet, URL: patientURL(missingPatientID), Headers: headers,
					ExpectedStatus:  http.StatusNotFound,
					ExpectedContent: []string{`"code":"` + web.CodeNotFound + `"`},
				}
			},
		},
		{
			name: "a change with no precondition is refused on the header",
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPatch, URL: self, Headers: headers,
					Body:           strings.NewReader(`{"address":"12 Marina Road"}`),
					ExpectedStatus: http.StatusUnprocessableEntity,
					ExpectedContent: []string{
						`"field":"` + web.IfMatchHeader + `"`,
						`"code":"` + domain.CodeRequired + `"`,
					},
				}
			},
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			headers, before := patientSignedIn(testsupport.AccountAEmail)

			scenario := one.build(headers)
			scenario.Name = one.name
			scenario.BeforeTestFunc = before

			runPatients(t, scenario)
		})
	}
}

// TestAPatientChangeAnsweringAStaleVersionCarriesTheCurrentRepresentation is
// FR-011/US1-7: a 412 is not a bare refusal, it is the current row, so the UI
// can show what actually happened without a second round trip.
func TestAPatientChangeAnsweringAStaleVersionCarriesTheCurrentRepresentation(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	self := patientURL(testsupport.AccountAPatientSelfID)

	scenario := tests.ApiScenario{
		Name:   "a stale If-Match is refused with the current representation",
		Method: http.MethodPatch, URL: self, Headers: headers,
		Body:           strings.NewReader(`{"address":"12 Marina Road"}`),
		ExpectedStatus: http.StatusPreconditionFailed,
		ExpectedContent: []string{
			`"code":"` + web.CodeVersionMismatch + `"`,
			`"current":{`,
			`"id":"` + testsupport.AccountAPatientSelfID + `"`,
		},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
			t.Helper()

			before(t, app, se)
			headers["If-Match"] = `"not-the-real-version"`
		},
	}

	runPatients(t, scenario)
}

// TestAPatientChangeCarryingOwnerIsRefusedAsAnUnknownMember is FR-002: the
// owning account is fixed at creation, and no PATCH can move it — asserted
// with a valid If-Match so the refusal is unambiguously about the unknown
// member and not the precondition.
func TestAPatientChangeCarryingOwnerIsRefusedAsAnUnknownMember(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	self := patientURL(testsupport.AccountAPatientSelfID)

	scenario := tests.ApiScenario{
		Name:   "a change carrying owner is refused as an unknown member",
		Method: http.MethodPatch, URL: self, Headers: headers,
		Body:           strings.NewReader(`{"owner":"` + testsupport.AccountBID + `"}`),
		ExpectedStatus: http.StatusUnprocessableEntity,
		ExpectedContent: []string{
			`"field":"owner"`,
			`"code":"` + domain.CodeUnknownField + `"`,
		},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
			t.Helper()

			before(t, app, se)

			record, err := app.FindRecordById(store.PatientCollection, testsupport.AccountAPatientSelfID)
			require.NoError(t, err)
			headers["If-Match"] = `"` + store.Version(record) + `"`
		},
	}

	runPatients(t, scenario)
}

// TestEveryPatientOperationRefusesAnAnonymousCaller is contracts/patients.md's
// 401 row: nothing about the resource is disclosed to a caller who never
// authenticated.
func TestEveryPatientOperationRefusesAnAnonymousCaller(t *testing.T) {
	t.Parallel()

	self := patientURL(testsupport.AccountAPatientSelfID)

	cases := []struct {
		name   string
		method string
		url    string
		body   string
	}{
		{"the list", http.MethodGet, patientsURL(), ""},
		{"a create", http.MethodPost, patientsURL(), `{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01"}`},
		{"a read", http.MethodGet, self, ""},
		{"a change", http.MethodPatch, self, `{"address":"12 Marina Road"}`},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			var body *strings.Reader
			if one.body != "" {
				body = strings.NewReader(one.body)
			}

			scenario := tests.ApiScenario{
				Name:            one.name,
				Method:          one.method,
				URL:             one.url,
				ExpectedStatus:  http.StatusUnauthorized,
				ExpectedContent: []string{`"code":"` + web.CodeUnauthenticated + `"`},
				NotExpectedContent: []string{
					testsupport.AccountAPatientSelfID,
				},
			}

			if body != nil {
				scenario.Body = body
			}

			runPatients(t, scenario)
		})
	}
}

// T054, FR-036/FR-038. A patient write fires MediKube's own audit hooks (never
// PocketBase's native record-CRUD request events — this application's routes
// bypass the auto-API entirely, apis.NewRouter never mounts it for this
// collection) and answers a status that proves the request actually landed.
func TestAPatientWriteFiresNoRecordCRUDRequestEvents(t *testing.T) {
	t.Parallel()

	headers, before := patientSignedIn(testsupport.AccountAEmail)

	scenario := tests.ApiScenario{
		Name:   "creating a patient fires zero PocketBase record-CRUD request events",
		Method: http.MethodPost, URL: patientsURL(), Headers: headers,
		Body:            strings.NewReader(`{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01"}`),
		ExpectedStatus:  http.StatusCreated,
		ExpectedContent: []string{`"first_name":"Ngozi"`},
		ExpectedEvents: map[string]int{
			"OnRecordCreateRequest": 0,
			"OnRecordUpdateRequest": 0,
			"OnRecordDeleteRequest": 0,
			"OnRecordsListRequest":  0,
			"OnRecordViewRequest":   0,
		},
		BeforeTestFunc: before,
	}

	runPatients(t, scenario)
}

// missingPatientID is a well-formed id no fixture uses.
const missingPatientID = "mkpatnobody0001"

// TestAPatientCreateOverDatastarAnswersHTML is 002's own form-patch
// behaviour: a Datastar submit gets the form or the list back as text/html
// on 200, and every other caller keeps today's JSON exactly (422/201).
func TestAPatientCreateOverDatastarAnswersHTML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    string
		headers map[string]string
		status  int
		content []string
	}{
		{
			name:    "an invalid create over Datastar answers 200 text/html with the form and the field error",
			body:    `{}`,
			headers: map[string]string{"Datastar-Request": "true"},
			status:  http.StatusOK,
			content: []string{
				ids.PatientForm(""),
				"a first name is required",
			},
		},
		{
			name:    "the same invalid create with no Datastar-Request header still answers 422 JSON",
			body:    `{}`,
			headers: nil,
			status:  http.StatusUnprocessableEntity,
			content: []string{`"code":"` + domain.CodeValidationFailed + `"`},
		},
		{
			name:    "a valid create over Datastar answers 200 text/html with the list landmark",
			body:    `{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01"}`,
			headers: map[string]string{"Datastar-Request": "true"},
			status:  http.StatusOK,
			content: []string{ids.PatientList()},
		},
		{
			name:    "the same valid create with no Datastar-Request header still answers 201 JSON",
			body:    `{"first_name":"Ngozi","last_name":"Adeyemi","birth_date":"1990-06-01"}`,
			headers: nil,
			status:  http.StatusCreated,
			content: []string{`"first_name":"Ngozi"`},
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			headers, before := patientSignedIn(testsupport.AccountAEmail)
			for k, v := range one.headers {
				headers[k] = v
			}

			scenario := tests.ApiScenario{
				Name:            one.name,
				Method:          http.MethodPost,
				URL:             patientsURL(),
				Headers:         headers,
				Body:            strings.NewReader(one.body),
				ExpectedStatus:  one.status,
				ExpectedContent: one.content,
				BeforeTestFunc:  before,
			}

			runPatients(t, scenario)
		})
	}
}
