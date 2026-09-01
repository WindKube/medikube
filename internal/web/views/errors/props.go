package errors

// The three landmarks contracts/pages.md fixes for the error views. They are
// the accessible names a Playwright getByRole resolves, so they are constants
// and a change to any of them is a change to the browser gate.
//
// internal/httproute's ErrorViews table spells the same three as selectors;
// internal/web/page/errors_test.go is the mechanical tie, so a rename in either
// place is a failing test rather than a landmark the gate opens the page to
// find and does not.
const (
	NotFoundLandmark       = "Not found"
	SignInRequiredLandmark = "Sign in required"
	ServerErrorLandmark    = "Something went wrong"
)

// ReferenceID is the element the request id is rendered into. Exactly one error
// view renders per response, so one id serves all three.
const ReferenceID = "error-reference"

// NotFoundProps is contracts/pages.md's E1, the privacy view.
//
// It has one member, and that is the point: this view is what a request for
// somebody else's record produces AND what a request for a record that never
// existed produces, and FR-033 requires the two to be identical to the byte. A
// member carrying anything about what was asked for would be the difference
// between them.
type NotFoundProps struct {
	// RequestID is the only correlation handle any error view carries. It says
	// nothing about the person or the record and matches the request_id on the
	// zerolog line (FR-054), which is what makes it quotable to an operator.
	RequestID string
}

// SignInRequiredProps is contracts/pages.md's E2.
type SignInRequiredProps struct {
	RequestID string

	// SignInHref is the page the prompt offers. It is a bare address with no
	// return-to parameter: FR-046 forbids an error view from echoing the
	// address that was refused, and a ?next= is exactly that address written
	// into a link.
	SignInHref string
}

// ServerErrorProps is contracts/pages.md's E3.
//
// The absence of a message member is the whole of FR-046 here. A view that
// cannot be handed the error text cannot render a driver's message, a query or
// a stack trace, whatever a later handler decides to put in the envelope —
// which is a stronger guarantee than a rule about what to pass.
type ServerErrorProps struct {
	RequestID string
}
