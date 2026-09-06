package web

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/i18n"
)

// localeField mirrors internal/store's unexported userFieldLocale.
const localeField = "locale"

// Localize is the one seam every response resolves a Localizer through: a
// whole page (internal/web/page.RenderPage) and a Datastar patch or JSON
// error (Render, Patch) alike carry the same D-04 order — the account's
// stored locale, else Accept-Language, else English.
//
// It is idempotent: a request whose context already carries a Localizer
// (RenderPage resolves one before building the components Render then
// writes) is left alone, so the render call underneath a page never
// second-guesses the page's own resolution.
func Localize(e *core.RequestEvent) *i18n.Localizer {
	ctx := e.Request.Context()
	if i18n.Present(ctx) {
		return i18n.From(ctx)
	}

	var accountLocale string
	if e.Auth != nil {
		accountLocale = e.Auth.GetString(localeField)
	}

	l := i18n.Resolve(accountLocale, e.Request.Header.Get("Accept-Language"))
	e.Request = e.Request.WithContext(i18n.With(ctx, l))

	return l
}
