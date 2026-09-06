// Package shell holds the document every page renders inside: the four
// landmarks contracts/pages.md requires on every page, signed in or out, and
// the two empty containers Datastar patches by id.
//
// It sits on the PocketBase side of the import boundary.
package shell

import (
	"html"
	"strconv"
	"time"

	"github.com/a-h/templ"

	"medikube/internal/web/views/ids"
)

// SuffixSeparator and ProductName compose contracts/pages.md's title column:
// every page's title is its own name, then this separator, then the product.
// They are constants because a Playwright assertion matches the whole string.
const (
	ProductName     = "MediKube"
	SuffixSeparator = " — "
)

// Title renders one page's <title>.
func Title(page string) string {
	if page == "" {
		return ProductName
	}

	return page + SuffixSeparator + ProductName
}

// TitleElement is the whole <title> as the layout renders it and as a detail
// patch re-renders it: the element carries ids.PageTitle so a Datastar patch
// can find it. It is one function so the two never drift.
func TitleElement(page string) string {
	return `<title id="` + ids.PageTitle + `">` + html.EscapeString(Title(page)) + `</title>`
}

// NavLink is one entry of the primary navigation.
type NavLink struct {
	Label string
	Href  string
	// Current marks the entry the page being rendered belongs to, which is
	// what aria-current announces.
	Current bool
}

// DocumentProps is one whole page.
type DocumentProps struct {
	// Title is the page's own name, without the product suffix: Document adds
	// it, so the nine pages cannot spell the suffix nine ways.
	Title string

	// SignedIn decides the contents of the navigation landmark and never its
	// existence. contracts/pages.md is explicit: navigation[name="Primary"] is
	// on every page in the application, because phase 005's public invitation
	// page is opened by somebody with no session who needs exactly the two
	// links it holds.
	SignedIn bool

	// StreamHref opens the record stream for the patient in view; empty when
	// there is no patient to follow.
	StreamHref string

	Nav []NavLink

	// Switcher is FR-014's shell control, nil on the signed-out surface. It
	// is a rendered component rather than PatientSwitcherProps directly, so a
	// page that has none to offer can leave it nil rather than build one with
	// no options.
	Switcher templ.Component

	// ThemeClass is D-36's class on <html>: "dark", "light" or "" — resolved
	// server-side, before the first byte, from the account's stored
	// preference (shell.ThemeClass). Empty is a legitimate value and not a
	// missing one: it is what "system" renders, and what leaves Tailwind's
	// dark variant following prefers-color-scheme instead.
	ThemeClass string

	// Version is the footer's build stamp — cmd/medikube's own version
	// string, threaded through rather than read from a package this side of
	// the [PB] boundary would have no way to reach.
	Version string

	// Main is the page's own landmark and everything inside it.
	Main templ.Component

	// RenderedAt seeds the staleness detector's clock. Zero means now, so a
	// page that does not care does not have to say so, and a test that does
	// can pin it.
	RenderedAt time.Time
}

// streamSeed is RenderedAt or now.
func (p DocumentProps) streamSeed() time.Time {
	if p.RenderedAt.IsZero() {
		return time.Now()
	}

	return p.RenderedAt
}

// The staleness detector of FR-031, contracts/streams.md's client half.
//
// The server cannot tell a person their live view has died — if the stream is
// dead there is no channel to say so on — so the detection is the page's, and
// it uses only FREE Datastar attributes. data-persist, data-match-media and
// data-on-raf are Pro and are not in the vendored v1.0.2 bundle at all: an
// attribute the runtime does not register is silently inert, so a detector
// built on one would be a banner that never appears and a test that never
// notices.
const (
	// SignalStreamBeat is patched by the server every HeartbeatInterval and
	// SignalStreamStale is what the page derives from it. internal/web/stream
	// owns both spellings; heartbeat_test.go is the mechanical tie, because a
	// signal named one thing on the wire and another in the attribute is a
	// comparison against a value nothing ever sets.
	SignalStreamBeat  = "_stream_beat"
	SignalStreamStale = "_stream_stale"

	// StreamStaleAfter is the gap without a heartbeat that means the live view
	// has stopped. It is two missed beats plus slack against a 25-second
	// interval: one dropped frame or one garbage-collection pause must not
	// tell somebody their data has stopped arriving.
	StreamStaleAfter = 60 * time.Second

	// StreamPollInterval is how often the page re-checks. It has to divide the
	// staleness threshold comfortably: the banner appears at the first poll
	// after the gap opens, so a poll interval near the threshold would leave a
	// dead view unremarked for nearly twice as long as advertised.
	StreamPollInterval = 10 * time.Second
)

// StreamPollAttribute is the attribute name the interval is declared under.
//
// It is composed from StreamPollInterval rather than spelled, and asserted
// against the rendered document, because Datastar's modifier syntax lives in
// the attribute NAME and a template cannot compose one. The delimiter before a
// plugin's KEY is a colon — data-on:click — and the delimiter before a MODIFIER
// is a double underscore; data-on-interval takes no key at all (the plugin
// declares requirement key "denied"), so data-on-interval:something throws and
// data-on-click, with a hyphen, parses as a plugin nothing registered and does
// precisely nothing.
func StreamPollAttribute() string {
	return "data-on-interval__duration." + strconv.Itoa(int(StreamPollInterval.Seconds())) + "s"
}

// StreamSignals is the initial signal state, written on <body>.
//
// _stream_beat is seeded with the moment the page was rendered rather than left
// empty, and that is the load-bearing half: a page whose stream never connects
// at all has no beat to compare against, so an empty seed would leave
// Date.parse returning NaN, every comparison false, and the one failure mode
// FR-031 is about — a view that never updates — the one the detector cannot
// see.
func StreamSignals(renderedAt time.Time) string {
	return "{" + SignalStreamBeat + ": '" + renderedAt.UTC().Format(time.RFC3339) + "', " +
		SignalStreamStale + ": false}"
}

// StaleExpression is what the interval evaluates.
//
// Date.parse of an RFC3339 UTC string is milliseconds since the epoch, which is
// what Date.now() answers in, so the subtraction is in the same unit on both
// sides. A local-offset timestamp would make the comparison depend on the
// server's timezone.
func StaleExpression() string {
	return "$" + SignalStreamStale + " = (Date.now() - Date.parse($" + SignalStreamBeat + ")) > " +
		strconv.FormatInt(StreamStaleAfter.Milliseconds(), 10)
}
