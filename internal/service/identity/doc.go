// Package identity decides registration, sign-in, the profile, the password,
// account deletion and password recovery.
//
// It declares the ports it needs — repository, authenticator, mailer, auditor,
// clock — and knows nothing about how any of them are implemented. It mints no
// token, hashes no password and renders no message: the session a caller ends
// up holding is minted at the edge through PocketBase's own auth response,
// which is what fires the hook that audits a sign-in on both MediKube's route
// and PocketBase's native one (research D-13, D-14).
//
// Every method that concerns an existing account reaches exactly one account —
// the caller's own. There is no id parameter in the package, so there is none
// to guess.
package identity
