package components

import "strconv"

// EmptyStateProps is what a list shows in place of its rows, and never in place
// of its landmark (FR-029, contracts/pages.md). The caller supplies the words
// because the component is shared: a kind's own vocabulary belongs to the kind.
type EmptyStateProps struct {
	// ID is the element the first record's arrival patches away.
	ID    string
	Title string
	Body  string

	// A caller with no action to offer leaves both empty and gets the
	// explanation alone rather than an empty link.
	ActionHref  string
	ActionLabel string
}

// ConfirmProps is a destructive action's confirmation: a rendered element with
// its own landmark, never a window.confirm, which the render gate cannot see
// and the smoke run cannot dismiss (FR-028).
type ConfirmProps struct {
	ID string
	// ReturnTo is the element that opened the confirmation; cancelling
	// hands focus back to it instead of dropping it on <body>.
	ReturnTo string

	// Signal is the Datastar signal that reveals it. The element is rendered
	// with the page and hidden, rather than created on demand, so that it
	// exists in the DOM for the gate whether or not it is on screen.
	Signal string

	Title string
	// Subject is what is about to be destroyed, named. FR-028 requires the
	// confirmation to name the medication rather than ask about "this item".
	Subject     string
	Consequence string

	ConfirmLabel string
	// ConfirmOn is the Datastar expression the confirming button runs. The
	// caller builds it, because the URL is the caller's and no view may spell a
	// kind's path segment (research D-05).
	ConfirmOn string

	CancelLabel string
}

// PaginationProps is one list's paging controls. Both hrefs may be empty and
// the element is rendered anyway: Datastar patches by id and an element that
// does not exist cannot be patched.
type PaginationProps struct {
	ID string

	PreviousHref string
	NextHref     string
}

func (p ConfirmProps) cancelExpression() string {
	expression := "$" + p.Signal + " = false"
	if p.ReturnTo != "" {
		expression += "; document.getElementById(" + strconv.Quote(p.ReturnTo) + ")?.focus()"
	}

	return expression
}
