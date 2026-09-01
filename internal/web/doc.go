// Package web is the HTTP edge: DTO decoding and encoding, the one
// error-to-status mapper and its envelope, ETag concurrency control, the
// session cookie, the security headers, and rendering templ components into a
// response.
//
// The edge has two layers, and the outer one is not optional. [SecurityHeaders]
// is a PocketBase router middleware and therefore only sees requests the router
// routed; [Outermost] wraps the built handler from outside the router
// altogether, which is the only place a CORS preflight and net/http's own
// path-normalising redirects are visible at all.
//
// It sits on the PocketBase side of the import boundary.
package web
