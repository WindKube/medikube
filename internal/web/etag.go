package web

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
)

// The two headers optimistic concurrency is carried on.
const (
	ETagHeader    = "ETag"
	IfMatchHeader = "If-Match"
)

// ETag renders a version as an entity-tag.
//
// The version is store.Version(record) — a hash of `updated` and never a column
// of its own (research D-24). It is quoted because RFC 9110's etagc excludes
// the space PocketBase's own date layout carries, so the instant itself is not
// a legal entity-tag at all, and because a version that reads as a timestamp
// invites somebody to compare two of them for order.
//
// A record that has never been saved has no version, and therefore no
// entity-tag, rather than the one entity-tag every unsaved record would share.
func ETag(version string) string {
	if version == "" {
		return ""
	}

	return `"` + version + `"`
}

// SetETag puts the version on the response, and puts nothing there when the
// record has none.
func SetETag(e *core.RequestEvent, version string) {
	if tag := ETag(version); tag != "" {
		e.Response.Header().Set(ETagHeader, tag)
	}
}

// IfMatch returns the version the caller is replacing.
//
// The header is REQUIRED on PATCH and DELETE, not merely honoured: an optional
// precondition is a precondition nobody sends (FR-026, research D-24). Its
// absence is 422 validation_failed with field `If-Match` and code `required`,
// which is what contracts/README.md, contracts/records.md and research D-24 all
// say and what internal/records.Handler already answers for the same condition.
// tasks.md T115 says 428 instead; specs/002-patient-core/contracts/patients.md
// records the decision the other way round — "428 -> not used ... keeps one
// error taxonomy" — and 422 is what this phase ships.
//
// Only an entity-tag this server issued is accepted. A weak validator is
// refused because If-Match is a strong comparison, and `*` is refused because
// it is a precondition that always passes, which is exactly the overwrite
// FR-026 exists to prevent.
func IfMatch(e *core.RequestEvent) (string, error) {
	raw := strings.TrimSpace(e.Request.Header.Get(IfMatchHeader))

	if raw == "" {
		return "", ifMatchRefusal(domain.CodeRequired,
			"the version you are replacing is required, so a write cannot silently overwrite a change you have not seen")
	}

	version, ok := parseEntityTag(raw)
	if !ok {
		return "", ifMatchRefusal(domain.CodeInvalidValue,
			"send back the ETag this server gave you, quoted and on its own")
	}

	return version, nil
}

// parseEntityTag reads exactly one strong entity-tag. Everything else — a
// wildcard, a weak validator, a list, an unquoted value — is refused rather
// than interpreted.
func parseEntityTag(raw string) (string, bool) {
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}

	inner := raw[1 : len(raw)-1]
	if inner == "" || strings.ContainsAny(inner, `",`) {
		return "", false
	}

	return inner, true
}

func ifMatchRefusal(code, message string) error {
	var invalid domain.ValidationError
	invalid.Add(IfMatchHeader, code, message)

	return invalid.OrNil()
}

// VersionMismatch is the 412 body: the envelope, and the representation the
// server currently holds.
//
// FR-026 and US1-9 — "the current values are shown so they can decide what to
// do" — is a property of the response rather than a second request the page has
// to remember to make. contracts/records.md requires the current representation
// and does not fix its member name; `current` beside `error` is this phase's
// choice, and phases 002-005 apply the same mechanism to eight more resources.
type VersionMismatch struct {
	Error Failure `json:"error"`
	// Current is the kind's own detail DTO, whatever that is. It is `any`
	// because this file knows nothing about kinds and must not learn.
	Current any `json:"current"`
}

// NewVersionMismatch builds the 412 body.
func NewVersionMismatch(requestID string, current any) VersionMismatch {
	return VersionMismatch{
		Error:   NewFailure(domain.ErrVersionMismatch, requestID),
		Current: current,
	}
}

// WriteVersionMismatch answers a stale If-Match with the current
// representation and the version to retry with, so the caller has everything it
// needs to decide without asking again.
func WriteVersionMismatch(e *core.RequestEvent, requestID, version string, current any) error {
	SetETag(e, version)

	return WriteJSON(e, http.StatusPreconditionFailed, NewVersionMismatch(requestID, current))
}
