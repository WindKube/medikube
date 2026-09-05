package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"

	"medikube/internal/domain"
	"medikube/internal/obs"
	"medikube/internal/store"
)

// The machine codes of contracts/README.md's table. A code is what a client
// switches on and what a translation is keyed by, so it is a constant here
// rather than a literal at each call site. domain.CodeValidationFailed is the
// fourteenth and lives with the type that raises it.
const (
	CodeUnauthenticated      = "unauthenticated"
	CodeForbidden            = "forbidden"
	CodeNotFound             = "not_found"
	CodeVersionMismatch      = "version_mismatch"
	CodeConflict             = "conflict"
	CodeRegistrationClosed   = "registration_closed"
	CodeInvalidToken         = "invalid_token"
	CodeMailUnconfigured     = "mail_unconfigured"
	CodeInvalidCursor        = "invalid_cursor"
	CodeRateLimited          = "rate_limited"
	CodeClientClosed         = "client_closed"
	CodeTimeout              = "timeout"
	CodeInternal             = "internal_error"
	CodeUnsupportedMediaType = "unsupported_media_type"

	// CodeBadRequest is the one code contracts/README.md's table does not name.
	// It is what a 4xx raised inside PocketBase and not by MediKube resolves to
	// — the body-limit middleware, a method mismatch — so that such a response
	// still carries a code a client can switch on instead of an empty string.
	CodeBadRequest = "bad_request"
)

// StatusClientClosed is nginx's 499, which contracts/README.md's table names
// for a cancelled request. net/http declares no constant for it and
// http.StatusText returns the empty string, so it is declared here.
const StatusClientClosed = 499

// InternalMessage is the literal contracts/README.md requires on every 500.
//
// No handler ever echoes an internal error string to a client: a PocketBase
// validation message can embed a filename, a driver error can embed a query,
// and both are disclosures in a medical-records application. The real error
// goes to the log, once.
const InternalMessage = "internal error"

// ErrorsMiddlewareID names the handler so it can be reordered or replaced by id.
const ErrorsMiddlewareID = "medikubeErrors"

// ErrorsMiddlewarePriority puts the envelope OUTSIDE PocketBase's panic
// recovery, which is what makes a recovered panic answer in MediKube's shape
// rather than PocketBase's. Everything the chain produces passes through here:
// apis.RequireAuth's 401, the rate limiter's 429, the lockdown's 404 and the
// mux's own miss, none of which any handler sees.
const ErrorsMiddlewarePriority = apis.DefaultPanicRecoverMiddlewarePriority - 1

// The rows of contracts/README.md's table that are conditions rather than
// domain sentinels. They are values so that a handler writes
// `fmt.Errorf("...: %w", web.ErrInvalidToken)` and the mapper reads the status
// and the code off the error rather than off the handler's opinion.
var (
	// ErrRegistrationClosed is the one 403 in the application. Nothing about
	// any person is revealed by it, which is why it may say so.
	ErrRegistrationClosed = &Coded{Status: http.StatusForbidden, Code: CodeRegistrationClosed}

	// ErrInvalidToken is one code for expired, already used and tampered with,
	// so that no token's former existence is disclosed by the difference.
	ErrInvalidToken = &Coded{Status: http.StatusBadRequest, Code: CodeInvalidToken}

	// ErrMailUnconfigured is a recovery message that cannot be sent because the
	// instance has no outgoing mail (FR-076).
	ErrMailUnconfigured = &Coded{Status: http.StatusServiceUnavailable, Code: CodeMailUnconfigured}
)

// Coded is an error that names its own status and machine code, for the
// conditions internal/domain has no sentinel for. It carries no message: the
// message a client is shown comes from Message(code) and is the same for every
// occurrence of a code, so that no occurrence can be told from another.
type Coded struct {
	Status int
	Code   string
}

func (c *Coded) Error() string { return c.Code }

// Failure is the inside of the error envelope on every non-2xx.
//
// It is a struct and not a map[string]any, and that is load-bearing:
// encoding/json/v2 does not sort map keys, so a three-member map marshals in
// three different member orders across runs. FR-033's byte-identical refusal
// and the whole-body comparison in testsupport.RunOwnershipMatrix would then be
// flaky, which Constitution VIII forbids outright.
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// RequestID is on every error including 500, and is the same value every
	// log line for the request carries (FR-054). It is what lets a person quote
	// a reference to an operator without disclosing anything about themselves.
	RequestID string `json:"request_id"`
	// Fields is present only for validation_failed.
	Fields []domain.FieldError `json:"fields,omitempty"`
}

// Envelope is the whole body of a non-2xx response.
type Envelope struct {
	Error Failure `json:"error"`
}

// ErrorView renders a failure as a MediKube page, for the routes that serve
// people rather than programs. It reports whether it rendered: a route that is
// not a page returns false and the envelope is written as JSON instead.
//
// It is a seam rather than a dependency because internal/web cannot import the
// views without importing every page they need, and because a 404 whose error
// view is broken must still be a 404 rather than a hang.
type ErrorView func(e *core.RequestEvent, status int, failure Failure) (bool, error)

// Classify is the ONE error→status mapper. Every non-2xx in the application
// gets its status and its machine code from here.
//
// The order of the checks is contract, not convenience. A *ValidationError is
// matched before anything else because it is a type rather than a sentinel; the
// domain sentinels are matched before *router.ApiError so that a sentinel
// PocketBase wrapped keeps its own answer (ApiError.Is unwraps its raw data);
// and the ApiError fallback is last because its statuses are the ones MediKube
// did not choose.
func Classify(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}

	var invalid *domain.ValidationError
	if errors.As(err, &invalid) {
		return http.StatusUnprocessableEntity, domain.CodeValidationFailed
	}

	var coded *Coded
	if errors.As(err, &coded) {
		return coded.Status, coded.Code
	}

	switch {
	case errors.Is(err, context.Canceled):
		return StatusClientClosed, CodeClientClosed
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, CodeTimeout
	case errors.Is(err, store.ErrInvalidCursor):
		return http.StatusBadRequest, CodeInvalidCursor
	case errors.Is(err, domain.ErrUnauthenticated):
		return http.StatusUnauthorized, CodeUnauthenticated
	// Before ErrForbidden, and the order is the whole of FR-033: a refusal on
	// owner-scoped data is wrapped into ErrNotFound by OwnerScoped, and an
	// answer that reached ErrForbidden first would hand back the 403 that
	// confirms the record exists.
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, CodeNotFound
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, CodeForbidden
	case errors.Is(err, domain.ErrVersionMismatch):
		return http.StatusPreconditionFailed, CodeVersionMismatch
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, CodeConflict
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests, CodeRateLimited
	case errors.Is(err, domain.ErrUnsupportedMedia):
		return http.StatusUnsupportedMediaType, CodeUnsupportedMediaType
	}

	var apiErr *router.ApiError
	if errors.As(err, &apiErr) {
		return apiErr.Status, codeForStatus(apiErr.Status)
	}

	return http.StatusInternalServerError, CodeInternal
}

// codeForStatus answers for a status MediKube did not choose. Only PocketBase
// raises these: a 4xx keeps its status and gets a code a client can switch on;
// a 5xx is an internal error whatever it says it is.
func codeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return CodeUnauthenticated
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusPreconditionFailed:
		return CodeVersionMismatch
	case http.StatusConflict:
		return CodeConflict
	case http.StatusTooManyRequests:
		return CodeRateLimited
	}

	if status >= http.StatusInternalServerError || status < http.StatusBadRequest {
		return CodeInternal
	}

	return CodeBadRequest
}

// Message is what a client is shown for a code. It is a constant per code and
// never assembled from the request, because a message built from a submission
// is a disclosure waiting for a log line — and because two occurrences of one
// code must be indistinguishable (FR-033).
func Message(code string) string {
	switch code {
	case CodeUnauthenticated:
		return "sign in to continue"
	case CodeForbidden:
		return "that is not something this account may do"
	case CodeNotFound:
		return "not found"
	case CodeVersionMismatch:
		return "this changed since you last read it"
	case domain.CodeValidationFailed:
		return domain.ValidationMessage
	case CodeConflict:
		return "that conflicts with something already recorded"
	case CodeRegistrationClosed:
		return "this instance is not open for registration"
	case CodeInvalidToken:
		return "that link is no longer usable; request another"
	case CodeMailUnconfigured:
		return "this instance cannot send mail; ask an operator"
	case CodeInvalidCursor:
		return "that page reference is not one this instance issued"
	case CodeRateLimited:
		return "too many requests; try again shortly"
	case CodeClientClosed:
		return "the request was cancelled"
	case CodeTimeout:
		return "the request took too long"
	case CodeUnsupportedMediaType:
		return "that file type is not accepted"
	case CodeBadRequest:
		return "the request could not be processed"
	}

	return InternalMessage
}

// NewFailure builds the inside of the envelope for err.
func NewFailure(err error, requestID string) Failure {
	_, code := Classify(err)

	failure := Failure{Code: code, Message: Message(code), RequestID: requestID}

	var invalid *domain.ValidationError
	if errors.As(err, &invalid) {
		failure.Fields = invalid.Fields
	}

	return failure
}

// unsupportedFileTypeSubstring is the sentence core/validators/file.go:63
// builds around the uploaded filename: `Failed to upload %q due to unsupported
// file type.`. This is the one recognisable, filename-free part of it.
const unsupportedFileTypeSubstring = "unsupported file type"

// MapFileValidationError replaces a PocketBase file-field validation failure
// with domain.ErrUnsupportedMedia (research D-17), and returns every other
// error unchanged.
//
// PocketBase's own message embeds the uploaded filename
// (`core/validators/file.go:63`), which constitution VII names as PHI: it must
// never reach the response, the log stream or Sentry. The match is on a
// substring PocketBase's own message always contains and the filename never
// does, so this never needs to parse or echo any part of what it replaces.
func MapFileValidationError(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), unsupportedFileTypeSubstring) {
		return domain.ErrUnsupportedMedia
	}

	return err
}

// NewEnvelope builds the whole body.
func NewEnvelope(err error, requestID string) Envelope {
	return Envelope{Error: NewFailure(err, requestID)}
}

// OwnerScoped answers a refusal on owner-scoped data the way FR-033 requires:
// exactly as a request for a record that has never existed.
//
// The services are supposed to return ErrNotFound for this already — that is
// what internal/domain's own comment says — and this is the edge refusing to
// disclose existence on the day one of them does not. It is the same rule the
// lockdown enforces at the PocketBase layer, enforced again at MediKube's own.
//
// The original is folded into the message rather than wrapped, deliberately: a
// %w would leave errors.Is(result, domain.ErrForbidden) true, and the next
// author to add a check in that order would hand back the 403 again. The domain
// sentinel messages are PHI-free by contract, so carrying the text costs
// nothing.
func OwnerScoped(err error) error {
	if err == nil || !errors.Is(err, domain.ErrForbidden) {
		return err
	}

	// %s and not %w, and errorlint is silenced rather than obeyed for exactly
	// the reason above: wrapping here would restore the 403 this function
	// exists to remove.
	return fmt.Errorf("owner-scoped refusal answered as a miss (%s): %w", err.Error(), domain.ErrNotFound)
}

// Errors is the middleware that owns every error the chain produces.
//
// It has to be a middleware. The contract's envelope is
// {"error":{code,message,request_id,fields}} and PocketBase's is
// {"data","message","status"}, written by its ErrorHandler at the mux level
// (tools/router/router.go:144) outside every handler — and the errors that
// matter most originate outside MediKube's handlers anyway: RequireAuth's 401,
// the rate limiter's 429, the lockdown's 404, the panic recovery's 500 and the
// mux's own miss.
//
// view may be nil, which is what an API-only build passes.
func Errors(view ErrorView) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       ErrorsMiddlewareID,
		Priority: ErrorsMiddlewarePriority,
		Func: func(e *core.RequestEvent) error {
			err := e.Next()
			if err == nil {
				return nil
			}

			// One occurrence, one report (FR-057). The request logger reads it
			// back and writes the single line; nothing here logs.
			obs.Report(e, err)

			if e.Written() {
				// The response has already gone and there is nothing left to
				// rewrite. Returning the error would have PocketBase's
				// ErrorHandler write a second header, which corrupts the
				// response and writes to stderr outside the one log stream.
				return nil
			}

			status, _ := Classify(err)
			failure := NewFailure(err, obs.CorrelationID(e.Request.Context()))

			if view != nil {
				rendered, viewErr := view(e, status, failure)
				if rendered && viewErr == nil {
					return nil
				}
				// A broken error view must not turn a 404 into a hang, so the
				// envelope is written anyway. viewErr is dropped rather than
				// reported: the occurrence is already recorded above, and
				// FR-057 gives one occurrence one report.
				if e.Written() {
					return nil
				}
			}

			return WriteError(e, status, failure)
		},
	}
}

// WriteError writes the envelope. It is exported because a handler that must
// add something to a refusal — the 412 carrying the current representation —
// writes its own body and still wants this one inside it.
func WriteError(e *core.RequestEvent, status int, failure Failure) error {
	return WriteJSON(e, status, Envelope{Error: failure})
}
