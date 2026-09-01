// Package logging builds MediKube's zerolog stream and redirects PocketBase's
// own framework logs into it, so that diagnosing the process means reading one
// stream instead of correlating two.
//
// It sits on the PocketBase side of the import boundary.
package logging
