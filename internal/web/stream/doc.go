// Package stream serves MediKube's one Server-Sent Events endpoint: the
// Datastar element stream behind the live record list.
//
// Every stream is opened through the one helper in stream.go, and a lint rule
// says so, because the two things that helper does are both invisible when they
// are missing. It clears the per-request write deadline PocketBase hardcodes at
// five minutes, without which every stream dies at exactly 5:00 and every test
// shorter than that passes; and it commits the header block before the SDK
// overwrites Cache-Control and drops X-Accel-Buffering (research D-34,
// contracts/streams.md).
//
// A stream that escaped the helper would also escape the per-subscriber
// re-authorisation and the heartbeat a long-lived connection needs to survive
// an intermediary.
//
// Two things are re-checked on every beat and every event, and they are
// different questions. The record checkpoint answers "may this actor see this
// record"; session.go answers "is this actor still signed in". Only the first
// was ever asked, and an actor is built once at subscribe — so a revoked
// session went on receiving rendered rows on the connection it already had,
// which is the "usable again from anywhere it was still open" FR-007 forbids.
//
// It sits on the PocketBase side of the import boundary.
package stream
