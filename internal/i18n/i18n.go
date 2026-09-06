// Package i18n is the only importer of go-i18n and golang.org/x/text/language
// in MediKube: one message bundle, one per-request Localizer, one context key.
//
// internal/domain never imports this package. A shipped language is whatever
// active.<lang>.toml files are embedded (Supported); nothing here is a Go
// slice a new language would have to edit.
package i18n

import (
	"context"
	"embed"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"medikube/internal/domain/kind"
)

//go:embed locales/*.toml
var localeFS embed.FS

const localesDir = "locales"

const languageNameID = "language.name"

var (
	bundleOnce sync.Once
	bundle     *goi18n.Bundle
	supported  []Language
	english    *Localizer
)

// Language is one shipped catalogue: its BCP 47 tag and its own name for
// itself, read from that file's language.name message.
type Language struct {
	Tag  language.Tag
	Name string
}

// loadBundle reads every active.*.toml under dir in fsys into a fresh
// Bundle and derives Supported()'s list from the filenames it found. It is
// the one seam an fs.FS-backed fixture drives: Supported() calls it against
// the embedded locales/, and a test can call it against a fixture with one
// extra file to prove a shipped language is derived rather than declared
// (FR-010).
func loadBundle(fsys fs.FS, dir string) (*goi18n.Bundle, []Language, error) {
	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, nil, err
	}

	langs := make([]Language, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		file, err := b.LoadMessageFileFS(fsys, dir+"/"+entry.Name())
		if err != nil {
			return nil, nil, err
		}

		name := file.Tag.String()
		localizer := goi18n.NewLocalizer(b, file.Tag.String())
		if translated, tErr := localizer.Localize(&goi18n.LocalizeConfig{MessageID: languageNameID}); tErr == nil {
			name = translated
		}

		langs = append(langs, Language{Tag: file.Tag, Name: name})
	}

	sort.Slice(langs, func(i, j int) bool { return langs[i].Tag.String() < langs[j].Tag.String() })

	return b, langs, nil
}

func ensureBundle() {
	bundleOnce.Do(func() {
		b, langs, err := loadBundle(localeFS, localesDir)
		if err != nil {
			panic("i18n: loading locales: " + err.Error())
		}

		bundle = b
		supported = langs
		english = &Localizer{inner: goi18n.NewLocalizer(bundle, language.English.String()), Tag: language.English}
	})
}

// Supported lists every shipped language, sorted by tag.
func Supported() []Language {
	ensureBundle()

	return append([]Language(nil), supported...)
}

// IsSupported reports whether code's base language matches a shipped
// language, region stripped (pl-PL -> pl).
func IsSupported(code string) bool {
	tag, err := language.Parse(code)
	if err != nil {
		return false
	}

	base, _ := tag.Base()

	for _, l := range Supported() {
		lBase, _ := l.Tag.Base()
		if lBase == base {
			return true
		}
	}

	return false
}

// Localizer wraps go-i18n's own Localizer with the resolved tag, so a
// consumer can read Tag (for <html lang>) without asking go-i18n for it back.
type Localizer struct {
	inner *goi18n.Localizer
	Tag   language.Tag
}

// Resolve picks the Localizer for a request: the account's stored locale if
// it names a shipped language, else the best Accept-Language match, else
// English.
func Resolve(accountLocale, acceptLanguage string) *Localizer {
	ensureBundle()

	if accountLocale != "" && IsSupported(accountLocale) {
		return newLocalizer(accountLocale)
	}

	// English first, so it is the matcher's default when nothing else fits.
	tags := make([]language.Tag, 0, len(supported)+1)
	tags = append(tags, language.English)
	for _, l := range supported {
		if l.Tag != language.English {
			tags = append(tags, l.Tag)
		}
	}

	if acceptLanguage != "" {
		if parsed, _, err := language.ParseAcceptLanguage(acceptLanguage); err == nil && len(parsed) > 0 {
			matcher := language.NewMatcher(tags)
			_, index, _ := matcher.Match(parsed...)

			return newLocalizer(tags[index].String())
		}
	}

	return english
}

func newLocalizer(tag string) *Localizer {
	parsed, err := language.Parse(tag)
	if err != nil {
		parsed = language.English
	}

	base, _ := parsed.Base()

	return &Localizer{inner: goi18n.NewLocalizer(bundle, base.String(), language.English.String()), Tag: parsed}
}

type contextKey struct{}

// With places a Localizer on ctx.
func With(ctx context.Context, l *Localizer) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// Present reports whether ctx already carries a Localizer placed by With,
// so a second layer of a response (internal/web's Render/Patch, underneath a
// page that already resolved one) does not re-resolve and risk disagreeing.
func Present(ctx context.Context) bool {
	_, ok := ctx.Value(contextKey{}).(*Localizer)

	return ok
}

// From reads the Localizer off ctx, defaulting to English when there is
// none.
func From(ctx context.Context) *Localizer {
	ensureBundle()

	if l, ok := ctx.Value(contextKey{}).(*Localizer); ok && l != nil {
		return l
	}

	return english
}

func mergeData(count *int, data []map[string]any) map[string]any {
	merged := map[string]any{}
	for _, one := range data {
		for k, v := range one {
			merged[k] = v
		}
	}

	if count != nil {
		merged["Count"] = *count
	}

	return merged
}

// T resolves a fixed-form message. A missing id in the localizer's own
// language falls back to English (go-i18n's own behaviour, since the
// Localizer is built with the language and English as fallbacks); a missing
// id in English is unreachable at runtime because reference_test.go fails the
// build first, so this never panics and returns the bare id in that case.
func T(ctx context.Context, id string, data ...map[string]any) string {
	l := From(ctx)

	config := &goi18n.LocalizeConfig{MessageID: id}
	if merged := mergeData(nil, data); len(merged) > 0 {
		config.TemplateData = merged
	}

	// Localize returns a best-effort message alongside a non-nil error
	// whenever the resolved language lacks the id and it fell back to
	// English (i18n/localizer.go) — that error is the fallback working as
	// designed, not a failure, so only an empty result (the id is undefined
	// even in English, which reference_test.go makes unreachable at runtime)
	// falls back to the bare id here.
	text, err := l.inner.Localize(config)
	if err != nil && text == "" {
		return id
	}

	return text
}

// N resolves a plural message for count, setting PluralCount and passing it
// through to the template data as {{.PluralCount}} — go-i18n only injects
// PluralCount into the template automatically when TemplateData is left nil
// (i18n/localizer.go), and this always supplies TemplateData so {{.Count}}
// is also available — so it is set explicitly here instead.
func N(ctx context.Context, id string, count int, data ...map[string]any) string {
	l := From(ctx)

	merged := mergeData(&count, data)
	merged["PluralCount"] = count

	config := &goi18n.LocalizeConfig{MessageID: id, PluralCount: count, TemplateData: merged}

	// Localize returns a best-effort message alongside a non-nil error
	// whenever the resolved language lacks the id and it fell back to
	// English (i18n/localizer.go) — that error is the fallback working as
	// designed, not a failure, so only an empty result (the id is undefined
	// even in English, which reference_test.go makes unreachable at runtime)
	// falls back to the bare id here.
	text, err := l.inner.Localize(config)
	if err != nil && text == "" {
		return id
	}

	return text
}

// KnownDynamicIDs is every message id produced by code rather than written as
// a literal i18n.T/N call: today, a kind's own plural display-name id
// (i18n.N(ctx, "kind."+k.Enum(), count)), one per kind.Kinds() value.
func KnownDynamicIDs() []string {
	kinds := kind.Kinds()
	ids := make([]string, 0, len(kinds))

	for _, k := range kinds {
		ids = append(ids, "kind."+k.Enum())
	}

	return ids
}
