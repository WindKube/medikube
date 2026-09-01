// Package openapi builds the OpenAPI document from the route registry rather
// than reflecting it out of the router, which PocketBase's route table does not
// allow.
//
// The result is committed and diffed, so an unintended API change arrives as a
// reviewable diff instead of as a client's bug report.
package openapi
