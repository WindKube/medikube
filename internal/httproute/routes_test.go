package httproute_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/testsupport/seed"
)

// T097. The table is the only inventory that exists, so it is pinned here
// against contracts/README.md's operation list and contracts/pages.md's page
// table by hand. A generated expectation would agree with the table by
// construction and prove nothing; these literals are the contract, transcribed.

func inventoryByOpID(t *testing.T) map[string]httproute.Route {
	t.Helper()

	byOpID := make(map[string]httproute.Route)
	for _, route := range httproute.Inventory().Routes() {
		_, duplicate := byOpID[route.OpID]
		require.Falsef(t, duplicate, "%s appears twice", route.OpID)
		byOpID[route.OpID] = route
	}

	return byOpID
}

// contracts/README.md, "Operation inventory — 22". The stream is one of the
// twenty-two, which is also what plan.md's Project Structure says.
func TestTheTableCarriesTheTwentyNineDocumentedOperations(t *testing.T) {
	t.Parallel()

	want := []struct {
		opID   string
		method string
		path   string
		kind   httproute.RouteKind
		auth   httproute.Auth
	}{
		{"getAuthConfig", http.MethodGet, "/api/v1/auth/config", httproute.KindAPI, httproute.AuthPublic},
		{"register", http.MethodPost, "/api/v1/auth/register", httproute.KindAPI, httproute.AuthPublic},
		{"login", http.MethodPost, "/api/v1/auth/login", httproute.KindAPI, httproute.AuthPublic},
		{"refreshSession", http.MethodPost, "/api/v1/auth/refresh", httproute.KindAPI, httproute.AuthUser},
		{"logout", http.MethodPost, "/api/v1/auth/logout", httproute.KindAPI, httproute.AuthUser},
		{"getMe", http.MethodGet, "/api/v1/me", httproute.KindAPI, httproute.AuthUser},
		{"updateMe", http.MethodPatch, "/api/v1/me", httproute.KindAPI, httproute.AuthUser},
		{"deleteMe", http.MethodDelete, "/api/v1/me", httproute.KindAPI, httproute.AuthUser},
		{"changePassword", http.MethodPut, "/api/v1/me/password", httproute.KindAPI, httproute.AuthUser},
		{"listRecords", http.MethodGet, "/api/v1/records", httproute.KindAPI, httproute.AuthUser},
		{"listRecordsOfKind", http.MethodGet, "/api/v1/records/{kind}", httproute.KindAPI, httproute.AuthUser},
		{"createRecord", http.MethodPost, "/api/v1/records/{kind}", httproute.KindAPI, httproute.AuthUser},
		{"getRecord", http.MethodGet, "/api/v1/records/{kind}/{id}", httproute.KindAPI, httproute.AuthUser},
		{"updateRecord", http.MethodPatch, "/api/v1/records/{kind}/{id}", httproute.KindAPI, httproute.AuthUser},
		{"deleteRecord", http.MethodDelete, "/api/v1/records/{kind}/{id}", httproute.KindAPI, httproute.AuthUser},
		{"streamRecords", http.MethodGet, "/api/v1/streams/records", httproute.KindStream, httproute.AuthUser},
		{"healthz", http.MethodGet, "/api/v1/healthz", httproute.KindAPI, httproute.AuthPublic},
		{"readyz", http.MethodGet, "/api/v1/readyz", httproute.KindAPI, httproute.AuthPublic},
		{"requestPasswordReset", http.MethodPost, "/api/v1/auth/password-reset", httproute.KindAPI, httproute.AuthPublic},
		{"confirmPasswordReset", http.MethodPost, "/api/v1/auth/password-reset/confirm", httproute.KindAPI, httproute.AuthPublic},
		{"requestEmailVerification", http.MethodPost, "/api/v1/auth/verify-email", httproute.KindAPI, httproute.AuthUser},
		{"confirmEmailVerification", http.MethodPost, "/api/v1/auth/verify-email/confirm", httproute.KindAPI, httproute.AuthPublic},
		{"listPatients", http.MethodGet, "/api/v1/patients", httproute.KindAPI, httproute.AuthUser},
		{"createPatient", http.MethodPost, "/api/v1/patients", httproute.KindAPI, httproute.AuthUser},
		{"getPatient", http.MethodGet, "/api/v1/patients/{patientId}", httproute.KindAPI, httproute.AuthUser},
		{"updatePatient", http.MethodPatch, "/api/v1/patients/{patientId}", httproute.KindAPI, httproute.AuthUser},
		{"putPatientPhoto", http.MethodPut, "/api/v1/patients/{patientId}/photo", httproute.KindAPI, httproute.AuthUser},
		{"getPatientPhoto", http.MethodGet, "/api/v1/patients/{patientId}/photo", httproute.KindAPI, httproute.AuthUser},
		{"deletePatientPhoto", http.MethodDelete, "/api/v1/patients/{patientId}/photo", httproute.KindAPI, httproute.AuthUser},
	}
	require.Len(t, want, 29)

	byOpID := inventoryByOpID(t)

	for _, wanted := range want {
		t.Run(wanted.opID, func(t *testing.T) {
			t.Parallel()

			route, registered := byOpID[wanted.opID]
			require.True(t, registered, "documented but not registered")

			assert.Equal(t, wanted.method, route.Method)
			assert.Equal(t, wanted.path, route.Path)
			assert.Equal(t, wanted.kind, route.Kind)
			assert.Equal(t, wanted.auth, route.Auth)
			assert.NotEmpty(t, route.Summary, "`medikube routes` prints a summary column")
		})
	}

	operations := 0
	for _, route := range byOpID {
		if route.Kind == httproute.KindAPI || route.Kind == httproute.KindStream {
			operations++
		}
	}
	assert.Equal(t, len(want), operations, "registered but not documented")
}

// contracts/README.md, "The public eight, and nothing else".
func TestExactlyEightOperationsArePublic(t *testing.T) {
	t.Parallel()

	want := []string{
		"getAuthConfig", "register", "login",
		"requestPasswordReset", "confirmPasswordReset", "confirmEmailVerification",
		"healthz", "readyz",
	}

	var got []string
	for _, route := range httproute.Inventory().Routes() {
		if (route.Kind == httproute.KindAPI || route.Kind == httproute.KindStream) && route.Auth == httproute.AuthPublic {
			got = append(got, route.OpID)
		}
	}

	assert.ElementsMatch(t, want, got)
}

// contracts/pages.md, "The pages". The landmark strings are what a Playwright
// getByRole selector contains, so changing one is a breaking change to the gate
// and has to break this test first.
func TestTheTableCarriesTheElevenPages(t *testing.T) {
	t.Parallel()

	segment := kind.Medication.Segment()

	want := []struct {
		opID     string
		path     string
		auth     httproute.Auth
		landmark string
		smokeURL string
	}{
		{"loginPage", "/login", httproute.AuthPublic, `form[name="Sign in"]`, "/login"},
		{"registerPage", "/register", httproute.AuthPublic, `form[name="Create account"]`, "/register"},
		{"overviewPage", "/", httproute.AuthUser, `region[name="Overview"]`, "/"},
		{"medicationListPage", "/" + segment, httproute.AuthUser, `region[name="Medications"]`, "/" + segment},
		{
			"medicationDetailPage", "/" + segment + "/{id}", httproute.AuthUser,
			`article[name="Medication"]`, "/" + segment + "/" + seed.NameOnlyID,
		},
		{"settingsPage", "/settings", httproute.AuthUser, `region[name="Settings"]`, "/settings"},
		{"forgotPasswordPage", "/forgot-password", httproute.AuthPublic, `form[name="Reset password"]`, "/forgot-password"},
		{
			"resetPasswordPage", "/reset-password/{token}", httproute.AuthPublic,
			`form[name="Choose a new password"]`, "/reset-password/expired-token-for-smoke",
		},
		{
			"verifyEmailPage", "/verify-email/{token}", httproute.AuthPublic,
			`region[name="Email confirmation"]`, "/verify-email/expired-token-for-smoke",
		},
		{"patientListPage", "/patients", httproute.AuthUser, `region[name="Patients"]`, "/patients"},
		{
			"patientDetailPage", "/patients/{patientId}", httproute.AuthUser,
			`region[name="Patient chart"]`, "/patients/" + seed.AccountAPatientSelfID,
		},
	}
	require.Len(t, want, 11)

	byOpID := inventoryByOpID(t)

	for _, wanted := range want {
		t.Run(wanted.opID, func(t *testing.T) {
			t.Parallel()

			route, registered := byOpID[wanted.opID]
			require.True(t, registered, "declared in contracts/pages.md but not registered")

			assert.Equal(t, http.MethodGet, route.Method)
			assert.Equal(t, wanted.path, route.Path)
			assert.Equal(t, httproute.KindPage, route.Kind)
			assert.Equal(t, wanted.auth, route.Auth)
			assert.Equal(t, wanted.landmark, route.Landmark)
			assert.Equal(t, wanted.smokeURL, route.SmokeURL)
		})
	}

	pages := 0
	for _, route := range byOpID {
		if route.Kind == httproute.KindPage {
			pages++
		}
	}
	assert.Equal(t, len(want), pages)
}

// contracts/pages.md, "The three error views".
func TestTheTableCarriesTheThreeErrorViews(t *testing.T) {
	t.Parallel()

	want := map[httproute.ErrorViewName]struct {
		status   int
		landmark string
	}{
		"notFound":       {http.StatusNotFound, `region[name="Not found"]`},
		"signInRequired": {http.StatusForbidden, `region[name="Sign in required"]`},
		"serverError":    {http.StatusInternalServerError, `region[name="Something went wrong"]`},
	}

	views := httproute.Inventory().ErrorViews()
	require.Len(t, views, len(want))

	for _, view := range views {
		wanted, declared := want[view.Name]
		require.Truef(t, declared, "%s is not one of contracts/pages.md's three", view.Name)

		assert.Equal(t, wanted.status, view.Status)
		assert.Equal(t, wanted.landmark, view.Landmark)
	}
}

// contracts/README.md, "Documented PocketBase-native paths that stay public".
// They are recorded so the Principle IX gate does not flag them and so nobody
// believes they were closed.
func TestTheTableDocumentsThePocketBaseNativePathsThatStayReachable(t *testing.T) {
	t.Parallel()

	var externals []string
	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			externals = append(externals, route.Pattern())
		}
	}

	assert.ElementsMatch(t, []string{
		"GET /_/{path...}",
		"POST /api/collections/_superusers/auth-with-password",
		"POST /api/collections/_superusers/auth-refresh",
		"GET /api/collections/_superusers/auth-methods",
		"POST /api/collections/users/auth-with-password",
		"POST /api/collections/users/auth-refresh",
		"GET /api/collections/users/auth-methods",
		"POST /api/collections/users/request-password-reset",
		"POST /api/collections/users/confirm-password-reset",
		"POST /api/collections/users/request-verification",
		"POST /api/collections/users/confirm-verification",
	}, externals)
}

// The medication pages spell a kind's plural exactly once, in kind.go, and read
// it back through Segment(). internal/architecture/kind_literals_test.go is the
// mechanical half of the same rule; this is the behavioural half.
func TestTheMedicationPagesAreDerivedFromTheKindTable(t *testing.T) {
	t.Parallel()

	byOpID := inventoryByOpID(t)

	assert.Equal(t, "/"+kind.Medication.Segment(), byOpID["medicationListPage"].Path)
	assert.Equal(t, "/"+kind.Medication.Segment()+"/{id}", byOpID["medicationDetailPage"].Path)
}

func TestNewPairsTheTableWithItsHandlers(t *testing.T) {
	t.Parallel()

	handlers := make(httproute.Handlers)
	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			continue
		}
		handlers[route.OpID] = func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) }
	}

	registry, err := httproute.New(handlers)
	require.NoError(t, err)
	assert.Equal(t, httproute.Inventory().Routes(), registry.Routes())
	assert.Equal(t, httproute.Inventory().ErrorViews(), registry.ErrorViews())
	require.NoError(t, registry.Bind(serveEvent(t)))
}

func TestNewReportsWiringThatDoesNotMatchTheTable(t *testing.T) {
	t.Parallel()

	full := make(httproute.Handlers)
	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			continue
		}
		full[route.OpID] = func(e *core.RequestEvent) error { return nil }
	}

	t.Run("a table row with no handler", func(t *testing.T) {
		t.Parallel()

		partial := make(httproute.Handlers, len(full))
		for opID, handler := range full {
			partial[opID] = handler
		}
		delete(partial, "getMe")

		_, err := httproute.New(partial)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getMe")
	})

	t.Run("a handler naming no table row", func(t *testing.T) {
		t.Parallel()

		stray := make(httproute.Handlers, len(full)+1)
		for opID, handler := range full {
			stray[opID] = handler
		}
		stray["notARegisteredOperation"] = func(e *core.RequestEvent) error { return nil }

		_, err := httproute.New(stray)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notARegisteredOperation")
	})

	t.Run("a handler for a documented external", func(t *testing.T) {
		t.Parallel()

		external := make(httproute.Handlers, len(full)+1)
		for opID, handler := range full {
			external[opID] = handler
		}
		external["nativeAdminUI"] = func(e *core.RequestEvent) error { return nil }

		_, err := httproute.New(external)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nativeAdminUI")
	})
}
