// Package stream serves the Datastar SSE connections.
//
// Every stream is opened through the one helper declared here, because a stream
// that escapes it also escapes the per-subscriber re-authorisation and the
// heartbeat a long-lived connection needs to survive an intermediary.
//
// It sits on the PocketBase side of the import boundary.
package stream
