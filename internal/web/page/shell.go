package page

import (
	"net/url"
	"sync/atomic"

	"github.com/a-h/templ"
	"github.com/pocketbase/pocketbase/core"

	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/i18n"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/shell"
)

// buildVersion is the footer's build stamp, set once from cmd/medikube's own
// `version`. It's an atomic.Value rather than a plain package var because
// cmd/medikube's own test suite builds several instances in parallel
// subtests, each calling SetBuildVersion.
var buildVersion atomic.Value

func init() { buildVersion.Store("dev") }

// SetBuildVersion sets the footer's build stamp.
func SetBuildVersion(v string) { buildVersion.Store(v) }

func getBuildVersion() string {
	v, _ := buildVersion.Load().(string)
	return v
}

// themeField mirrors internal/store's unexported userFieldTheme: e.Auth is
// the record the router already resolved, so reading it back here avoids a
// second identity-service call per page render.
const themeField = "theme"

func resolveTheme(e *core.RequestEvent) domainidentity.Theme {
	if e == nil || e.Auth == nil {
		return domainidentity.ThemeSystem
	}

	theme := domainidentity.Theme(e.Auth.GetString(themeField))
	if !theme.Valid() {
		return domainidentity.ThemeSystem
	}

	return theme
}

// resolveLocale is D-04's resolution order: the account's stored locale,
// else the browser's Accept-Language, else English. web.Localize is the one
// place that actually resolves it (T013), because internal/web's own
// Render/Patch need the same rule for the responses that are not a whole
// page; this fixes it on the request's context as a side effect.
func resolveLocale(e *core.RequestEvent) *i18n.Localizer {
	return web.Localize(e)
}

// NavState is the primary navigation's contents plus whether this page
// renders the signed-in shell.
type NavState struct {
	SignedIn bool
	Nav      []shell.NavLink

	// Switcher is FR-014's shell control content. Zero value renders present
	// but with no options — every signed-in page carries the element, and a
	// page that has not yet been taught to resolve the patient list renders
	// it empty rather than omitting it.
	Switcher shell.PatientSwitcherProps
}

// RenderPage is the one place a page becomes a response, replacing three
// call sites that built shell.DocumentProps by hand.
func RenderPage(e *core.RequestEvent, status int, title string, nav NavState, main web.Component) error {
	e.Response.Header().Set("Cache-Control", pageCacheControl)

	locale := resolveLocale(e)

	var switcher templ.Component
	if nav.SignedIn {
		switcher = shell.PatientSwitcher(nav.Switcher)
	}

	return web.Render(e, status, shell.Document(shell.DocumentProps{
		Title:      title,
		Lang:       locale.Tag.String(),
		SignedIn:   nav.SignedIn,
		StreamHref: streamHref(e, nav.SignedIn),
		Nav:        nav.Nav,
		Switcher:   switcher,
		ThemeClass: shell.ThemeClass(resolveTheme(e)),
		Version:    getBuildVersion(),
		Main:       main,
	}))
}

const opStreamRecords = "streamRecords"

// streamHref is the record stream for the patient in view (contracts/streams.md):
// the page opens it with data-init so created rows and heartbeats arrive live.
func streamHref(e *core.RequestEvent, signedIn bool) string {
	patientID := e.Request.URL.Query().Get(api.ParamPatient)
	if !signedIn || patientID == "" {
		return ""
	}

	paths, err := routePaths(map[string]string{opStreamRecords: ""})
	if err != nil {
		return ""
	}

	return paths[opStreamRecords] + "?" + api.ParamPatient + "=" + url.QueryEscape(patientID)
}
