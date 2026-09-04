package obs

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
)

// Recordable returns the error worth recording for a request answered with
// err, which is not always the error itself.
//
// A server failure — a 500, a panic, a driver refusal — is recorded as its
// Cause: that message is what makes a log line or a Sentry issue actionable,
// and the code composing it is MediKube's own or the driver's. Everything
// answered below 500 is a refusal, or a success PocketBase chose to pair with
// an error, and a refusal is completely described by its status and its
// contract code. Its message survives only when it is MediKube's own — a
// chain ending in one of internal/domain's sentinels, whose messages are
// PHI-free by contract — and is withheld otherwise, because a message composed
// anywhere else is composed from what the request submitted: PocketBase's
// password-reset handler answers 204 and returns "failed to fetch users record
// with email <address>" so that its own activity logger can record the address
// (defect D20). Recording that puts an account-enumeration oracle into the
// operational record — the line exists when the address has no account and is
// absent when it has one — which is exactly the difference FR-073 requires to
// be unobservable.
func Recordable(e *core.RequestEvent, err error) error {
	if err == nil {
		return nil
	}

	cause := Cause(err)

	code := status(e, err)
	if code >= http.StatusInternalServerError {
		return cause
	}

	for _, sentinel := range domain.Sentinels() {
		if errors.Is(cause, sentinel) {
			return cause
		}
	}

	return withheldCause{status: code}
}

// withheldCause stands in for a message that must not be recorded. It has no
// Unwrap on purpose: a reporter that walks the chain finds nothing behind it.
type withheldCause struct {
	status int
}

func (w withheldCause) Error() string {
	return fmt.Sprintf("answered %d; the cause is withheld below a server failure", w.status)
}
