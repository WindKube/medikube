// Package identitytest provides the fakes and the shared contract suite for the
// ports package identity declares, so every implementation of a port — the
// fakes included — is held to one contract.
//
// The fake authenticator carries the counting seam research D-17 asks for: a
// refusal for an address with no account costs the same comparison a wrong
// password does, and a test says so with a counter rather than a clock.
package identitytest
