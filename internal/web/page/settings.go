package page

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/i18n"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/settings"
)

// settings renders contracts/pages.md's P6: FR-011's five preferences, FR-009's
// password change, FR-075's confirmation state and FR-013's danger zone, all
// inside region[name="Settings"].
//
// It reads the account through the identity service rather than off the actor,
// so the page and GET /api/v1/me answer from one decision: an account an
// operator has taken out of service is refused here exactly as it is there,
// rather than being refused by the API and rendered by the page.
func (p *accountPages) settings(e *core.RequestEvent, actor access.Actor) error {
	if err := requireSession(actor); err != nil {
		return err
	}

	ctx := e.Request.Context()

	user, err := p.deps.Accounts.Me(ctx, actor)
	if err != nil {
		return err
	}

	// The same counter getMe answers with, so the number the confirmation
	// states and the number the API reports cannot differ. It is resolved
	// through the record family's authorization checkpoint with the actor as
	// its only input, so there is no other account it could be asked about.
	counts, err := p.deps.Counts(ctx, actor)
	if err != nil {
		return err
	}

	return p.render(e, settingsTitle, true, p.links.signedInNav(p.links.settingsPage), settings.Settings(settings.SettingsProps{
		SignOutOn: p.links.post(p.links.logout),
		Profile: settings.ProfileProps{
			FormID:         ids.ProfileForm,
			OnSubmit:       p.links.patch(p.links.me),
			Email:          user.Email,
			EmailConfirmed: user.EmailConfirmed,
			ResendOn:       p.links.post(p.links.verify),
			Name:           user.Name,
			UnitSystems:    unitSystemOptions(user.UnitSystem),
			Locale:         user.Locale,
			Locales:        localeOptions(user.Locale),
			DateFormats:    dateFormatOptions(user.DateFormat),
			Themes:         themeOptions(user.Theme),
		},
		Password: settings.PasswordProps{
			FormID:   ids.PasswordForm,
			OnSubmit: p.links.put(p.links.password),
			Rules:    domainidentity.PublishedPasswordRules(),
		},
		Danger: settings.DangerZoneProps{
			FormID:   ids.DeleteAccountForm,
			OnSubmit: p.links.remove(p.links.me),
			Phrase:   domainidentity.DeleteConfirmationPhrase,
			Holdings: holdings(counts),
		},
	}))
}

// The three preference vocabularies, each read from the domain's published
// order rather than listed here, so a value added to one of them appears in the
// control that offers it without anybody remembering to add it.
//
// The labels are this layer's: a domain vocabulary is a set of stored values and
// "Follow the device" is not one of them.
func unitSystemOptions(selected domainidentity.UnitSystem) []settings.Option {
	labels := map[domainidentity.UnitSystem]string{
		domainidentity.UnitSystemMetric:   "Metric",
		domainidentity.UnitSystemImperial: "Imperial",
	}

	return optionsOf(domainidentity.UnitSystems(), selected, labels)
}

func dateFormatOptions(selected domainidentity.DateFormat) []settings.Option {
	labels := map[domainidentity.DateFormat]string{
		domainidentity.DateFormatISO: "Year-month-day (2026-08-31)",
		domainidentity.DateFormatDMY: "Day/month/year (31/08/2026)",
		domainidentity.DateFormatMDY: "Month/day/year (08/31/2026)",
	}

	return optionsOf(domainidentity.DateFormats(), selected, labels)
}

// localeOptions is i18n.Supported() (D-07: derived from the embedded
// catalogues, never a Go slice this layer edits), labelled by each language's
// own name for itself.
//
// selected is matched by its base language, region stripped, the same
// comparison Resolve and IsSupported make: a stored "en-GB" selects the "en"
// option rather than matching nothing.
func localeOptions(selected string) []settings.Option {
	base, _, _ := strings.Cut(selected, "-")
	base = strings.ToLower(base)

	langs := i18n.Supported()
	rendered := make([]settings.Option, 0, len(langs))

	for _, lang := range langs {
		tag := lang.Tag.String()
		rendered = append(rendered, settings.Option{
			Value:    tag,
			Label:    lang.Name,
			Selected: tag == base,
		})
	}

	return rendered
}

func themeOptions(selected domainidentity.Theme) []settings.Option {
	labels := map[domainidentity.Theme]string{
		domainidentity.ThemeSystem: "Follow the device",
		domainidentity.ThemeLight:  "Light",
		domainidentity.ThemeDark:   "Dark",
	}

	return optionsOf(domainidentity.Themes(), selected, labels)
}

// optionsOf falls back to the stored value as its own label rather than
// dropping the option or rendering a blank one: a vocabulary that grew a member
// nobody labelled must still be offered, or the control would silently stop
// being able to express what the account already holds.
func optionsOf[T ~string](values []T, selected T, labels map[T]string) []settings.Option {
	rendered := make([]settings.Option, 0, len(values))

	for _, value := range values {
		label, named := labels[value]
		if !named {
			label = string(value)
		}

		rendered = append(rendered, settings.Option{
			Value:    string(value),
			Label:    label,
			Selected: value == selected,
		})
	}

	return rendered
}
