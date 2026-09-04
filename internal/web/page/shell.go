package page

import (
	"sync/atomic"

	"github.com/pocketbase/pocketbase/core"

	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/web"
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

// NavState is the primary navigation's contents plus whether this page
// renders the signed-in shell.
type NavState struct {
	SignedIn bool
	Nav      []shell.NavLink
}

// RenderPage is the one place a page becomes a response, replacing three
// call sites that built shell.DocumentProps by hand.
func RenderPage(e *core.RequestEvent, status int, title string, nav NavState, main web.Component) error {
	e.Response.Header().Set("Cache-Control", pageCacheControl)

	return web.Render(e, status, shell.Document(shell.DocumentProps{
		Title:      title,
		SignedIn:   nav.SignedIn,
		Nav:        nav.Nav,
		ThemeClass: shell.ThemeClass(resolveTheme(e)),
		Version:    getBuildVersion(),
		Main:       main,
	}))
}
