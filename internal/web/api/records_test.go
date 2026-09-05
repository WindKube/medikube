package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/apitest"
)

// T141, FR-025. The six operations of contracts/records.md driven through
// tests.ApiScenario: the statuses, the response shapes, the error envelope,
// unknown-member rejection and the published query vocabulary.
//
// Every case builds its OWN tests.TestApp through apitest.Factory. A shared one
// grows an OnServe handler per scenario — apis.NewRouter binds one with no Id
// and hook.Bind appends rather than replaces without one — and the chain runs
// as nested e.Next() calls, so the stack deepens per case until the goroutine
// limit ends the process (reconciliation C14).

// signedIn returns a header map the scenario sends and the before-func that
// fills it in.
//
// The token has to be minted from the app the factory built for THIS case: it
// is signed with that clone's own collection secret, and one minted against
// another clone is refused in a way that reads exactly like an authorization
// defect. The map is shared by reference, and ApiScenario reads it after
// BeforeTestFunc has run.
func signedIn(email string) (map[string]string, func(testing.TB, *tests.TestApp, *core.ServeEvent)) {
	headers := map[string]string{}

	return headers, func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
		t.Helper()

		headers["Authorization"] = testsupport.UserToken(t, app, email)
	}
}

func run(t *testing.T, s tests.ApiScenario) {
	t.Helper()

	s.TestAppFactory = apitest.Factory()
	s.Test(t)
}

// unregisteredSegment is a path segment no kind declares. It is deliberately
// plausible: an implausible one would pass a check that only rejects nonsense.
const unregisteredSegment = "prescriptions"

func TestTheRecordFamilyAnswersEveryDocumentedShape(t *testing.T) {
	t.Parallel()

	one := recordURL(testsupport.NameOnlyMedicationID)

	cases := []struct {
		name     string
		build    func(headers map[string]string) tests.ApiScenario
		email    string
		withAuth bool
	}{
		{
			name:     "the cross-kind list answers the account's records",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet,
					URL:    crossKindURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=2", Headers: headers,
					ExpectedStatus: http.StatusOK,
					ExpectedContent: []string{
						`"items":[`,
						`"kind":"` + kind.Medication.Enum() + `"`,
						`"next_cursor":"`,
					},
				}
			},
		},
		{
			name:     "one kind's list answers the same envelope",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet,
					URL: collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID +
						"&limit=2&count=true",
					Headers:        headers,
					ExpectedStatus: http.StatusOK,
					ExpectedContent: []string{
						`"items":[`,
						`"total":` + itoa(testsupport.AccountAMedicationCount),
					},
					NotExpectedContent: []string{`"owner"`},
				}
			},
		},
		{
			name:     "an unregistered kind is a miss and not a refusal",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet, URL: "/api/v1/records/" + unregisteredSegment, Headers: headers,
					ExpectedStatus:  http.StatusNotFound,
					ExpectedContent: []string{`"code":"` + web.CodeNotFound + `"`},
					// The refusal names no kind: a 400 saying "that is not a
					// kind" tells an anonymous prober which ones exist.
					NotExpectedContent: []string{unregisteredSegment},
				}
			},
		},
		{
			name:     "an ordering outside the allowlist is refused rather than ignored",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet,
					URL:    collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&sort=owner", Headers: headers,
					ExpectedStatus: http.StatusUnprocessableEntity,
					ExpectedContent: []string{
						`"code":"` + domain.CodeValidationFailed + `"`,
						`"field":"` + web.ParamSort + `"`,
						`"code":"` + domain.CodeInvalidValue + `"`,
					},
				}
			},
		},
		{
			name:     "a page size outside the bounds is refused",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet,
					URL:    collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=1000", Headers: headers,
					ExpectedStatus:  http.StatusUnprocessableEntity,
					ExpectedContent: []string{`"field":"` + web.ParamLimit + `"`},
				}
			},
		},
		{
			name:     "a forged cursor is refused as a cursor and not as a miss",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet,
					URL: collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID +
						"&cursor=not-a-cursor",
					Headers:         headers,
					ExpectedStatus:  http.StatusBadRequest,
					ExpectedContent: []string{`"code":"` + web.CodeInvalidCursor + `"`},
				}
			},
		},
		{
			name:     "a create answers the created representation",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPost, URL: collectionURL(), Headers: headers,
					Body: strings.NewReader(`{"patient":"` + testsupport.AccountAPatientSelfID +
						`","name":"Amoxicillin","dosage":"500 mg"}`),
					ExpectedStatus: http.StatusCreated,
					ExpectedContent: []string{
						`"kind":"` + kind.Medication.Enum() + `"`,
						`"name":"Amoxicillin"`,
						`"status":"` + string(activeStatus) + `"`,
					},
				}
			},
		},
		{
			name:     "a create carrying an owner is refused as an unknown member",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPost, URL: collectionURL(), Headers: headers,
					Body: strings.NewReader(`{"patient":"` + testsupport.AccountAPatientSelfID +
						`","name":"Amoxicillin","owner":"` + testsupport.AccountBID + `"}`),
					ExpectedStatus: http.StatusUnprocessableEntity,
					ExpectedContent: []string{
						`"field":"owner"`,
						`"code":"` + domain.CodeUnknownField + `"`,
					},
				}
			},
		},
		{
			name:     "a create of an unregistered kind is a miss",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPost, URL: "/api/v1/records/" + unregisteredSegment, Headers: headers,
					Body:            strings.NewReader(`{"name":"Amoxicillin"}`),
					ExpectedStatus:  http.StatusNotFound,
					ExpectedContent: []string{`"code":"` + web.CodeNotFound + `"`},
				}
			},
		},
		{
			name:     "a read answers the full representation and omits what was never recorded",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet, URL: one, Headers: headers,
					ExpectedStatus: http.StatusOK,
					ExpectedContent: []string{
						`"id":"` + testsupport.NameOnlyMedicationID + `"`,
						`"started_on":null`,
						`"created_at":"`,
					},
					// FR-024: a member never filled in is absent, not present
					// and empty.
					NotExpectedContent: []string{`"dosage"`, `"notes"`, `"type"`},
				}
			},
		},
		{
			name:     "an identifier that never existed is a miss",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodGet, URL: recordURL(missingID), Headers: headers,
					ExpectedStatus:  http.StatusNotFound,
					ExpectedContent: []string{`"code":"` + web.CodeNotFound + `"`},
				}
			},
		},
		{
			name:     "a change with no precondition is refused on the header",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodPatch, URL: one, Headers: headers,
					Body:           strings.NewReader(`{"dosage":"1 g"}`),
					ExpectedStatus: http.StatusUnprocessableEntity,
					ExpectedContent: []string{
						`"field":"` + web.IfMatchHeader + `"`,
						`"code":"` + domain.CodeRequired + `"`,
					},
				}
			},
		},
		{
			name:     "a deletion with no precondition is refused on the header",
			withAuth: true,
			build: func(headers map[string]string) tests.ApiScenario {
				return tests.ApiScenario{
					Method: http.MethodDelete, URL: one, Headers: headers,
					ExpectedStatus:  http.StatusUnprocessableEntity,
					ExpectedContent: []string{`"field":"` + web.IfMatchHeader + `"`},
				}
			},
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			headers, before := signedIn(testsupport.AccountAEmail)

			scenario := one.build(headers)
			scenario.Name = one.name
			scenario.BeforeTestFunc = before

			run(t, scenario)
		})
	}
}

// TestEveryRecordOperationRefusesAnAnonymousCaller is the 401 half, and it is
// separate because the assertion is that the refusal happens BEFORE any
// handler: apis.RequireAuth is bound by the route table on every non-page
// route, so the body names no resource because no handler ran.
func TestEveryRecordOperationRefusesAnAnonymousCaller(t *testing.T) {
	t.Parallel()

	one := recordURL(testsupport.NameOnlyMedicationID)

	cases := []struct {
		name   string
		method string
		url    string
		body   string
	}{
		{"the cross-kind list", http.MethodGet, crossKindURL(), ""},
		{"one kind's list", http.MethodGet, collectionURL(), ""},
		{"a create", http.MethodPost, collectionURL(), `{"name":"Amoxicillin"}`},
		{"a read", http.MethodGet, one, ""},
		{"a change", http.MethodPatch, one, `{"dosage":"1 g"}`},
		{"a deletion", http.MethodDelete, one, ""},
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
					testsupport.NameOnlyMedicationID,
					kind.Medication.Segment(),
				},
			}

			if body != nil {
				scenario.Body = body
			}

			run(t, scenario)
		})
	}
}
