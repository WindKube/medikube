// Package records is the kind-agnostic registry each clinical record kind
// registers itself with, and the dispatch table the one generic record handler
// works from.
//
// A new kind is added by satisfying the interfaces declared here, never by
// extending a switch statement somewhere else.
package records
