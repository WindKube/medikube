package web

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/obs"
	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// The taxonomy of contracts/README.md, transcribed once. Every other assertion
// in this file reads from here rather than repeating a number, so a row changed
// in one place changes everywhere it is asserted.
type taxonomyRow struct {
	name   string
	err    error
	status int
	code   string
}

func contractTaxonomy() []taxonomyRow {
	return []taxonomyRow{
		{"no valid session", domain.ErrUnauthenticated, http.StatusUnauthorized, CodeUnauthenticated},
		{"a resource whose existence the caller already knows", domain.ErrForbidden, http.StatusForbidden, CodeForbidden},
		{"not there, and every authorization failure on owner-scoped data", domain.ErrNotFound, http.StatusNotFound, CodeNotFound},
		{"If-Match did not match", domain.ErrVersionMismatch, http.StatusPreconditionFailed, CodeVersionMismatch},
		{"uniqueness or invariant", domain.ErrConflict, http.StatusConflict, CodeConflict},
		{"rate limited", domain.ErrRateLimited, http.StatusTooManyRequests, CodeRateLimited},
		{"a rejected field", new(domain.ValidationError), http.StatusUnprocessableEntity, domain.CodeValidationFailed},
		{"registration while closed", ErrRegistrationClosed, http.StatusForbidden, CodeRegistrationClosed},
		{"an expired, used or tampered token", ErrInvalidToken, http.StatusBadRequest, CodeInvalidToken},
		{"no outgoing mail configured", ErrMailUnconfigured, http.StatusServiceUnavailable, CodeMailUnconfigured},
		{"a forged, tampered or unparseable cursor", store.ErrInvalidCursor, http.StatusBadRequest, CodeInvalidCursor},
		{"the client went away", context.Canceled, StatusClientClosed, CodeClientClosed},
		{"the deadline passed", context.DeadlineExceeded, http.StatusGatewayTimeout, CodeTimeout},
		{"anything else", errors.New("a driver error naming a query"), http.StatusInternalServerError, CodeInternal},
	}
}

func TestEveryRowOfTheContractTaxonomyMapsToItsStatusAndItsCode(t *testing.T) {
	t.Parallel()

	for _, row := range contractTaxonomy() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			status, code := Classify(row.err)
			assert.Equal(t, row.status, status)
			assert.Equal(t, row.code, code)

			// Wrapped, which is how a service actually returns one. A mapper
			// that compared with == rather than errors.Is passes the line above
			// and fails every real call site.
			wrapped := fmt.Errorf("the medication service: %w", row.err)
			wrappedStatus, wrappedCode := Classify(wrapped)
			assert.Equal(t, row.status, wrappedStatus, "wrapping changed the status")
			assert.Equal(t, row.code, wrappedCode, "wrapping changed the code")
		})
	}
}

// The guard that bites when a seventh sentinel is added to internal/domain and
// nowhere else: an unmapped sentinel falls through to 500 internal_error, which
// is a stack trace in production and a green suite here.
func TestEveryDomainSentinelIsMappedAndNoTwoShareACode(t *testing.T) {
	t.Parallel()

	sentinels := domain.Sentinels()
	require.NotEmpty(t, sentinels)

	byCode := make(map[string]error, len(sentinels))
	byStatus := make(map[int]error, len(sentinels))

	for _, sentinel := range sentinels {
		status, code := Classify(sentinel)

		assert.NotEqualf(t, CodeInternal, code,
			"%v has no row in the mapper, so it reaches a client as an internal error", sentinel)
		assert.NotEqualf(t, http.StatusInternalServerError, status,
			"%v has no row in the mapper, so it reaches a client as a 500", sentinel)

		if previous, taken := byCode[code]; taken {
			assert.Failf(t, "two sentinels share one machine code",
				"%v and %v both map to %q, so a client cannot tell them apart", previous, sentinel, code)
		}
		byCode[code] = sentinel

		if previous, taken := byStatus[status]; taken {
			assert.Failf(t, "two sentinels share one status",
				"%v and %v both map to %d", previous, sentinel, status)
		}
		byStatus[status] = sentinel
	}
}

// FR-033. The response for another person's record and the response for a
// record that never existed are the same bytes, because the code, the message
// and the absence of everything else are the same. request_id is the only
// member that differs, and the matrix in internal/testsupport asserts that over
// real responses; here it is asserted over the envelope that produces them.
func TestAStrangersRefusalIsByteIdenticalToAGenuineMiss(t *testing.T) {
	t.Parallel()

	const requestID = "0000000000000000"

	miss, err := json.Marshal(NewEnvelope(domain.ErrNotFound, requestID))
	require.NoError(t, err)

	refusal, err := json.Marshal(NewEnvelope(OwnerScoped(domain.ErrForbidden), requestID))
	require.NoError(t, err)

	assert.Equal(t, string(miss), string(refusal),
		"the refusal is distinguishable from a miss, so the identifier is confirmed by the body")

	assert.NotContains(t, string(miss), "forbidden")
}

// The rule the brief states and contracts/README.md states from the other side:
// a 403 is never returned for owner-scoped data. The service is supposed to
// return ErrNotFound; this is the edge refusing to disclose existence even when
// it does not.
func TestOwnerScopedTurnsEveryRefusalIntoAMiss(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{"a forbidden", domain.ErrForbidden},
		{"a wrapped forbidden", fmt.Errorf("the authorizer: %w", domain.ErrForbidden)},
		{"a miss stays a miss", domain.ErrNotFound},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			status, code := Classify(OwnerScoped(one.err))

			assert.Equal(t, http.StatusNotFound, status)
			assert.Equal(t, CodeNotFound, code)
			assert.False(t, errors.Is(OwnerScoped(one.err), domain.ErrForbidden),
				"the forbidden survives, so a later errors.Is would answer 403 after all")
		})
	}

	t.Run("everything else is untouched", func(t *testing.T) {
		t.Parallel()

		status, code := Classify(OwnerScoped(domain.ErrVersionMismatch))
		assert.Equal(t, http.StatusPreconditionFailed, status)
		assert.Equal(t, CodeVersionMismatch, code)

		assert.Nil(t, OwnerScoped(nil), "a success became a refusal")
	})
}

// The 500 message is a constant. A PocketBase validation message can embed a
// filename and a driver error can embed a query; both are disclosures here.
func TestAnInternalFailureDisclosesNothingItWasGiven(t *testing.T) {
	t.Parallel()

	// Built from the kind table rather than typed: the collection is spelled in
	// exactly one file in this repository and an AST guard enforces it.
	secret := "SELECT name FROM " + kind.Medication.Collection() + " WHERE owner = '" + testsupport.AccountAID + "'"

	failure := NewFailure(fmt.Errorf("the store: %s", secret), "req")

	assert.Equal(t, CodeInternal, failure.Code)
	assert.Equal(t, InternalMessage, failure.Message)
	assert.NotContains(t, failure.Message, "SELECT")
	assert.Empty(t, failure.Fields)

	body, err := json.Marshal(Envelope{Error: failure})
	require.NoError(t, err)
	assert.NotContains(t, string(body), secret)
}

// A *ValidationError is the only code that carries fields, and it carries all
// of them: FR-027 requires a form to show every problem at once.
func TestValidationCarriesEveryFieldAndNothingElseCarriesAny(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add("name", domain.CodeRequired, "a name is required")
	invalid.Add("ended_on", "end_before_start", "the end is before the start")

	failure := NewFailure(fmt.Errorf("validating: %w", invalid.OrNil()), "req")

	assert.Equal(t, domain.CodeValidationFailed, failure.Code)
	assert.Equal(t, domain.ValidationMessage, failure.Message)
	require.Len(t, failure.Fields, 2, "FR-027 wants every problem in one response")
	assert.Equal(t, "name", failure.Fields[0].Field)
	assert.Equal(t, "ended_on", failure.Fields[1].Field)

	for _, row := range contractTaxonomy() {
		if row.code == domain.CodeValidationFailed {
			continue
		}

		assert.Emptyf(t, NewFailure(row.err, "req").Fields,
			"%s carries fields[], which contracts/README.md gives to validation_failed alone", row.name)
	}
}

// research D-28 and the flaky-gate ban of Constitution VIII. encoding/json/v2
// does not sort map keys, so an envelope built as a map[string]any marshals in
// a different member order on roughly one run in five — and the ownership
// matrix compares whole bodies.
func TestTheEnvelopeIsByteStableAcrossMarshals(t *testing.T) {
	t.Parallel()

	envelope := NewEnvelope(domain.ErrNotFound, "01JQ8Z")

	first, err := json.Marshal(envelope)
	require.NoError(t, err)

	for range 200 {
		again, err := json.Marshal(envelope)
		require.NoError(t, err)
		require.Equal(t, string(first), string(again),
			"the envelope's member order is not stable, so every whole-body assertion in the suite is flaky")
	}

	assert.JSONEq(t, `{"error":{"code":"not_found","message":"`+Message(CodeNotFound)+`","request_id":"01JQ8Z"}}`, string(first))
	assert.NotContains(t, string(first), `"fields"`, "fields is present outside validation_failed")
}

func TestEveryCodeHasAMessageAndNoMessageNamesAResource(t *testing.T) {
	t.Parallel()

	for _, row := range contractTaxonomy() {
		message := Message(row.code)

		assert.NotEmptyf(t, message, "%s has no message", row.code)
		assert.NotContainsf(t, message, "medication", "%s names the resource", row.code)
		assert.NotContainsf(t, message, "%", "%s reads as an unfilled format string", row.code)
	}

	assert.Equal(t, InternalMessage, Message(CodeInternal), "the 500 message is a constant")
	assert.Equal(t, InternalMessage, Message("a code nobody declared"))
}

// The middleware owns EVERY error the chain produces, not only the ones
// MediKube's handlers return. PocketBase writes its own {"data","message",
// "status"} envelope from the mux, outside every handler, so anything not
// intercepted here reaches a client in the wrong shape.
func TestTheMiddlewareOwnsEveryErrorTheChainProduces(t *testing.T) {
	t.Parallel()

	handlers := binder(func(se *core.ServeEvent) {
		se.Router.Route(http.MethodGet, "/x/domain", func(e *core.RequestEvent) error {
			return fmt.Errorf("the service: %w", domain.ErrNotFound)
		})
		se.Router.Route(http.MethodGet, "/x/panic", func(e *core.RequestEvent) error {
			panic("a nil map write deep in a store")
		})
		se.Router.Route(http.MethodGet, "/x/guarded", func(e *core.RequestEvent) error {
			return e.NoContent(http.StatusNoContent)
		}).Bind(apis.RequireAuth())
	})

	factory := testsupport.NewAppFactory(
		middleware(obs.RequestLogger(discardLogger()), Errors(nil)),
		handlers,
	)

	scenarios := []tests.ApiScenario{
		{
			Name:            "a domain sentinel",
			Method:          http.MethodGet,
			URL:             "/x/domain",
			ExpectedStatus:  http.StatusNotFound,
			ExpectedContent: []string{`"error":{`, `"code":"not_found"`, `"request_id":"`},
			// PocketBase's own envelope, which must not survive.
			NotExpectedContent: []string{`"data":{}`, `"status":404`},
			TestAppFactory:     factory,
		},
		{
			Name:               "a panic",
			Method:             http.MethodGet,
			URL:                "/x/panic",
			ExpectedStatus:     http.StatusInternalServerError,
			ExpectedContent:    []string{`"code":"internal_error"`, `"message":"` + InternalMessage + `"`},
			NotExpectedContent: []string{"PANIC RECOVER", "nil map write", "goroutine"},
			TestAppFactory:     factory,
		},
		{
			Name:               "PocketBase's own RequireAuth",
			Method:             http.MethodGet,
			URL:                "/x/guarded",
			ExpectedStatus:     http.StatusUnauthorized,
			ExpectedContent:    []string{`"code":"unauthenticated"`},
			NotExpectedContent: []string{`"status":401`},
			TestAppFactory:     factory,
		},
		{
			Name:               "a path nobody registered",
			Method:             http.MethodGet,
			URL:                "/x/nothing-here",
			ExpectedStatus:     http.StatusNotFound,
			ExpectedContent:    []string{`"code":"not_found"`},
			NotExpectedContent: []string{`"status":404`},
			TestAppFactory:     factory,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// The request id on the wire and the request id in the envelope are the same
// value, because that is the whole of FR-054: a person quotes one reference and
// an operator finds the line.
func TestTheEnvelopeCarriesTheCorrelationIdTheResponseHeaderCarries(t *testing.T) {
	t.Parallel()

	var header string

	scenario := tests.ApiScenario{
		Name:           "the id is the same on both",
		Method:         http.MethodGet,
		URL:            "/x/boom",
		Headers:        map[string]string{obs.CorrelationHeader: "an-inbound-id"},
		ExpectedStatus: http.StatusNotFound,
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Errors(nil)),
			route(http.MethodGet, "/x/boom", func(e *core.RequestEvent) error { return domain.ErrNotFound }),
		),
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			header = res.Header.Get(obs.CorrelationHeader)
		},
		ExpectedContent: []string{`"request_id":"an-inbound-id"`},
	}
	scenario.Test(t)

	assert.Equal(t, "an-inbound-id", header)
}

// A view is offered the failure first, so a page route answers with a rendered
// MediKube page and an API route answers with the envelope. A view that
// declines, or that fails, still gets the envelope — an error page that cannot
// render must not turn a 404 into a hang.
func TestAViewMayAnswerAndDecliningFallsBackToTheEnvelope(t *testing.T) {
	t.Parallel()

	rendered := func(e *core.RequestEvent, status int, failure Failure) (bool, error) {
		if e.Request.URL.Path != "/x/page" {
			return false, nil
		}

		return true, e.HTML(status, "<section aria-label=\"Not found\">"+failure.RequestID+"</section>")
	}

	broken := func(e *core.RequestEvent, _ int, _ Failure) (bool, error) {
		return false, errors.New("the error view itself is broken")
	}

	for name, view := range map[string]ErrorView{"a working view": rendered, "a broken view": broken} {
		expected := []string{`"code":"not_found"`}
		if name == "a working view" {
			expected = []string{`aria-label="Not found"`}
		}

		scenario := tests.ApiScenario{
			Name:            name,
			Method:          http.MethodGet,
			URL:             "/x/page",
			ExpectedStatus:  http.StatusNotFound,
			ExpectedContent: expected,
			TestAppFactory: testsupport.NewAppFactory(
				middleware(obs.RequestLogger(discardLogger()), Errors(view)),
				route(http.MethodGet, "/x/page", func(e *core.RequestEvent) error { return domain.ErrNotFound }),
			),
		}
		scenario.Test(t)
	}
}

// A handler that has already written cannot have its response rewritten, and
// the middleware must not try: a second WriteHeader is a corrupted response and
// a line on stderr outside the one log stream.
func TestAnErrorAfterTheResponseWasWrittenIsRecordedAndNotRewritten(t *testing.T) {
	t.Parallel()

	scenario := tests.ApiScenario{
		Name:           "a handler that wrote and then failed",
		Method:         http.MethodGet,
		URL:            "/x/late",
		ExpectedStatus: http.StatusOK,
		// The body the handler wrote, not an envelope over the top of it.
		ExpectedContent:    []string{"already sent"},
		NotExpectedContent: []string{`"code":`},
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Errors(nil)),
			route(http.MethodGet, "/x/late", func(e *core.RequestEvent) error {
				if err := e.String(http.StatusOK, "already sent"); err != nil {
					return err
				}

				return domain.ErrConflict
			}),
		),
	}
	scenario.Test(t)
}

func TestAnApiErrorFromPocketBaseKeepsItsStatusAndGetsAMediKubeCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unauthorized", router.NewUnauthorizedError("", nil), http.StatusUnauthorized, CodeUnauthenticated},
		{"forbidden", router.NewForbiddenError("", nil), http.StatusForbidden, CodeForbidden},
		{"not found", router.NewNotFoundError("", nil), http.StatusNotFound, CodeNotFound},
		{"too many requests", router.NewTooManyRequestsError("", nil), http.StatusTooManyRequests, CodeRateLimited},
		{"bad request", router.NewBadRequestError("", nil), http.StatusBadRequest, CodeBadRequest},
		{"internal", router.NewInternalServerError("", nil), http.StatusInternalServerError, CodeInternal},
		{
			"one wrapping a sentinel keeps the sentinel's code",
			router.NewBadRequestError("", fmt.Errorf("%w", domain.ErrVersionMismatch)),
			http.StatusPreconditionFailed,
			CodeVersionMismatch,
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			status, code := Classify(one.err)
			assert.Equal(t, one.status, status)
			assert.Equal(t, one.code, code)

			assert.NotContains(t, NewFailure(one.err, "req").Message, "went wrong while processing",
				"PocketBase's own prose reached the client")
		})
	}
}
