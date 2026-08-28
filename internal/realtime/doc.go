// Package realtime is the in-process hub carrying record identifiers from a
// post-commit hook to the subscribers watching for them.
//
// It publishes identifiers and never record bodies, which is what lets each
// subscriber's stream re-fetch and re-authorise a change for its own viewer
// before rendering it. MediKube is single-instance by construction, so the hub
// is a channel and a map with no broker interface in front of it.
package realtime
