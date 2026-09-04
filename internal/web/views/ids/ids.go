package ids

import (
	"strings"

	"medikube/internal/domain/kind"
)

// The shell's fixed identifiers (contracts/pages.md). #error-banner and #toast
// are empty containers rendered on every page because Datastar patches by id
// and an element that does not exist cannot be patched — a patch aimed at a
// missing target logs PatchElementsNoTargetsFound, which is a console warning
// and therefore a failed smoke run rather than a failed request.
const (
	Main        = "main"
	ErrorBanner = "error-banner"
	Toast       = "toast"

	// StreamStale is FR-031's banner: the element revealed when the live view
	// has stopped being kept current. It is an id rather than a class because
	// a test asserts on the element and because a later phase will patch it.
	StreamStale = "stream-stale"

	// Overview is contracts/pages.md P3's landmark, the application root.
	Overview = "overview"
)

// The roles an element plays, spelled once. They are what separates
// #medication-row-abc from #medication-detail-abc, so a rename here moves the
// element and its selector together.
const (
	roleList    = "list"
	roleRows    = "rows"
	roleRow     = "row"
	roleEmpty   = "empty"
	rolePager   = "pager"
	roleDetail  = "detail"
	roleForm    = "form"
	roleConfirm = "confirm"
	roleField   = "field"
	roleError   = "error"
	roleHeading = "heading"
)

// The forms that belong to no kind: the five on the signed-out surface and the
// three on the settings page. They are constants rather than composed because
// there is exactly one of each in the application, and because ids.Field and
// ids.FieldError take a form id — so a form whose id was spelled at the call
// site would name its controls one thing and its refusals another.
//
// ConfirmAddressForm names no <form>: contracts/pages.md's P9 is a region with
// one control in it. It is here anyway because that control still needs an id
// that cannot collide, and Field is the one thing that mints those.
const (
	SignInForm         = "sign-in"
	CreateAccountForm  = "create-account"
	ForgotPasswordForm = "forgot-password"
	NewPasswordForm    = "new-password"
	ConfirmAddressForm = "confirm-address"
	ProfileForm        = "profile"
	PasswordForm       = "password-change"
	DeleteAccountForm  = "delete-account"
)

// The prefix for a kind the table does not declare. An id has to begin with a
// letter to be a legal CSS id selector, and "-row-abc" is not one: the element
// would render, the patch would match nothing, and the only symptom would be a
// live view that quietly stopped updating.
const undeclaredKind = "record"

// RecordList is the region a kind's list renders into, and the target of a
// full-region patch.
func RecordList(k kind.Kind) string { return join(prefix(k), roleList) }

// RecordRows is the container the rows sit in — what a newly created record is
// prepended to, so that a create patches one element rather than the region.
func RecordRows(k kind.Kind) string { return join(prefix(k), roleRows) }

// RecordRow is the id contracts/streams.md patches by: the stream renders the
// row component and targets datastar.WithSelectorID(RecordRow(kind, id)).
func RecordRow(k kind.Kind, recordID string) string { return join(prefix(k), roleRow, recordID) }

// RecordEmpty is the empty state, which lives inside the region rather than
// instead of it (FR-029), and is therefore an element of its own to patch away
// when the first record arrives.
func RecordEmpty(k kind.Kind) string { return join(prefix(k), roleEmpty) }

func RecordPager(k kind.Kind) string { return join(prefix(k), rolePager) }

func RecordDetail(k kind.Kind, recordID string) string {
	return join(prefix(k), roleDetail, recordID)
}

// RecordListHeading and RecordDetailHeading are where focus moves after a
// full-region patch (FR-048): tabindex="-1" plus autofocus, no script needed —
// the CSP bans inline scripts and data-persist/data-on-signal-patch are Pro or
// answer a different question. autofocus fires on any DOM connection, parsed
// or patched, so the same static attribute covers both.
func RecordListHeading(k kind.Kind) string { return join(prefix(k), roleList, roleHeading) }

func RecordDetailHeading(k kind.Kind, recordID string) string {
	return join(prefix(k), roleDetail, recordID, roleHeading)
}

// RecordForm takes an empty recordID for the create form, which has no record
// to name yet.
func RecordForm(k kind.Kind, recordID string) string { return join(prefix(k), roleForm, recordID) }

// RecordConfirm is the delete confirmation, a rendered element and never a
// window.confirm (FR-028, contracts/pages.md).
func RecordConfirm(k kind.Kind, recordID string) string {
	return join(prefix(k), roleConfirm, recordID)
}

// Field and FieldError are the two halves of FR-048's aria-describedby link.
// They take the form's own id rather than a kind, because two forms for one
// kind on one page would otherwise give their controls the same id and the
// label would point at whichever the browser found first.
func Field(formID, field string) string { return join(formID, roleField, field) }

func FieldError(formID, field string) string { return join(formID, roleError, field) }

func prefix(k kind.Kind) string {
	sanitised := safe(k.Enum())
	if sanitised == "" || !isLetter(sanitised[0]) {
		return undeclaredKind
	}
	return sanitised
}

// join drops empty parts, so RecordForm with no record is "medication-form"
// rather than "medication-form-" — and cannot collide with a record whose id
// sanitised to nothing.
func join(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if sanitised := safe(part); sanitised != "" {
			kept = append(kept, sanitised)
		}
	}
	return strings.Join(kept, "-")
}

// safe is the reason this is a package and not a fmt.Sprintf at each call site.
//
// The id is written into an attribute and read back as "#"+id by
// web.WithSelectorID, which takes a bare id and so has nowhere to escape it. A
// record id carrying a quote would close the attribute; one carrying a dot or a
// hash would silently address a different element. Both are unreachable if the
// only ids that exist are the ones this produced.
func safe(value string) string {
	if value == "" {
		return ""
	}

	out := []byte(value)
	for i, b := range out {
		if !isIDByte(b) {
			out[i] = '_'
		}
	}
	return string(out)
}

func isIDByte(b byte) bool {
	return isLetter(b) || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
