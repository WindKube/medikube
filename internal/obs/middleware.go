package obs

import (
	"errors"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// FaultKey is the request-scoped key the one occurrence is recorded under. It
// is exported so that an operator reading a handler's e.GetAll() dump knows
// what they are looking at, not so that anything writes it directly.
const FaultKey = "medikubeFault"

// PanicMarker is what PocketBase's panic-recovery middleware prefixes the
// recovered value with (apis/middlewares.go:274). It is a string rather than a
// type, so recognising a recovered panic means recognising this.
const PanicMarker = "[PANIC RECOVER]"

// Report records err as THE occurrence for this request and reports whether
// this call is the one that recorded it.
//
// FR-057: a single occurrence must not be reported more than once across the
// log stream, the error-reporting destination and the measurements. The
// mechanism is deliberately a ledger on the request rather than a convention,
// because a convention is what produces the same failure in the log twice, in
// Sentry twice and in a span twice — each written by a different author who
// believed theirs was the only one.
//
// The first report wins. A later one is refused rather than layered on top:
// the first is the cause and the second is what the first turned into, and a
// ledger that kept the last would report the symptom.
func Report(e *core.RequestEvent, err error) bool {
	if err == nil || e == nil {
		return false
	}

	if Reported(e) {
		return false
	}

	e.Set(FaultKey, err)

	return true
}

// Fault returns the occurrence recorded for the request, or nil.
//
// The request logger reads it to write the single line: the error middleware
// answers the client and returns nil, so without this the one line for a failed
// request would say nothing about why it failed.
func Fault(e *core.RequestEvent) error {
	if e == nil {
		return nil
	}

	recorded, ok := e.Get(FaultKey).(error)
	if !ok {
		return nil
	}

	return recorded
}

// Reported reports whether an occurrence has already been recorded for the
// request. A second reporter asks this before doing its own work, so that the
// work itself — assembling a Sentry event, ending a span with a status — is not
// done twice either.
func Reported(e *core.RequestEvent) bool { return Fault(e) != nil }

// Cause returns the error worth recording, which is not always the error that
// was returned.
//
// PocketBase wraps everything it raises in a *router.ApiError whose Error() is
// the public, deliberately vague message a client is shown — "Something went
// wrong while processing your request." A log line carrying that says nothing
// at all: the fact is in RawData, which is where the panic and its stack live
// (apis/middlewares.go:274) and where a driver error ends up.
//
// contracts/README.md is explicit that the real error goes to the log while the
// client is shown a constant, so this is the unwrapping that makes a 500
// actionable. It is one level deep on purpose — RawData is the cause, and a
// recursive walk would be walking the cause's own wrapping, which %w already
// renders.
func Cause(err error) error {
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		return err
	}

	if raw, ok := apiErr.RawData().(error); ok {
		return raw
	}

	return err
}

// Panicked reports whether err is a panic PocketBase's recovery converted into
// an error.
//
// It matters because the two are answered identically to a client and must be
// distinguishable to an operator: an ordinary 500 is a failure the code
// anticipated, and a panic is one it did not. MediKube adds no second recover —
// PocketBase's runs at -1030 and a second one would report the same panic
// twice, which is exactly what FR-057 forbids.
func Panicked(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(Cause(err).Error(), PanicMarker)
}
