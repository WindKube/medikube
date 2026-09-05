package domain

import "errors"

// The sentinels every layer above the domain compares against. They are values,
// wrapped with %w and inspected with errors.Is — which is why samber/mo is
// forbidden: mo.Result severs the chain that the error mapper, the zerolog
// Err() hook and the Sentry integration all read.
//
// Each message is PHI-free and stays PHI-free: a wrapped sentinel's text is
// logged, and in a medical-records application an error string is a disclosure
// surface. None of these is what the client is shown — internal/web maps a
// sentinel to a status and a machine code, and writes its own message.
var (
	// ErrNotFound is also every authorization failure on owner-scoped data:
	// another person's record is answered exactly as a non-existent one, so the
	// service returns this and not ErrForbidden (FR-033).
	ErrNotFound = errors.New("not found")

	// ErrForbidden is reserved for resources whose existence the caller already
	// knows about — refusing it discloses nothing it did not already have.
	ErrForbidden = errors.New("forbidden")

	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrVersionMismatch is the If-Match failure. The edge answers 412 with the
	// current representation so the person can see what changed (FR-026).
	ErrVersionMismatch = errors.New("version mismatch")

	ErrConflict = errors.New("conflict")

	ErrRateLimited = errors.New("rate limited")

	// ErrUnsupportedMedia is an upload whose sniffed content type PocketBase's
	// own MimeTypes validator refused (research D-17). It replaces, and never
	// wraps, PocketBase's own message: that message embeds the uploaded
	// filename, which constitution VII names as PHI.
	ErrUnsupportedMedia = errors.New("unsupported media type")
)

// Sentinels returns every sentinel the domain defines, so the error mapper's
// test can assert it covers all of them rather than the ones its author
// remembered. A new sentinel with no status and no machine code is a 500 with
// a stack trace attached to it; this is what turns that into a red test.
//
// The order is the declaration order above and is not otherwise meaningful.
func Sentinels() []error {
	return []error{
		ErrNotFound,
		ErrForbidden,
		ErrUnauthenticated,
		ErrVersionMismatch,
		ErrConflict,
		ErrRateLimited,
		ErrUnsupportedMedia,
	}
}
