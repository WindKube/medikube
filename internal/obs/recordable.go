package obs

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
)

// Recordable is the error worth recording for a request answered with err.
// At or above 500 that is the cause. Below 500 the message is composed from
// what the request submitted (PocketBase's password-reset handler answers 204
// and returns an error quoting the address, defect D20), so only MediKube's
// own domain sentinels survive and everything else is withheld.
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

type withheldCause struct {
	status int
}

func (w withheldCause) Error() string {
	return fmt.Sprintf("answered %d; the cause is withheld below a server failure", w.status)
}
