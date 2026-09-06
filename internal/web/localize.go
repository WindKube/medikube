package web

import (
	"context"
	"strings"
	"unicode"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
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

// KindLabel is a kind's own plural display name, title-cased for a heading
// or a tile (the patient chart's count tiles): i18n.N's "other" form of
// kind.<enum> (D-06), forced with a count of 2 and the leading "2 " it
// carries trimmed back off, since a tile label is never itself a count —
// mirrors internal/web/page.kindNoun, which does the same for the singular
// "one" form.
func KindLabel(ctx context.Context, k kind.Kind) string {
	noun := strings.TrimPrefix(i18n.N(ctx, "kind."+k.Enum(), 2), "2 ")
	if noun == "" {
		return noun
	}

	r := []rune(noun)
	r[0] = unicode.ToUpper(r[0])

	return string(r)
}
