package page

import (
	"context"

	"medikube/internal/domain/access"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/i18n"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/settings"
)

// settingsForms implements api.SettingsForms, mirroring tagForms' own
// reasoning: the API renders a submit through the same component the
// /settings page itself builds.
type settingsForms struct {
	links accountLinks
}

// NewSettingsForms builds the adapter api.SettingsForms renders a Datastar
// form submit through.
func NewSettingsForms() (api.SettingsForms, error) {
	links, err := newAccountLinks()
	if err != nil {
		return nil, err
	}

	return settingsForms{links: links}, nil
}

// Updated re-renders the profile form alone, patched into place by its own
// id (ids.ProfileForm): the password and deletion forms are unaffected by a
// preference change, so only the one form the submit came from moves.
func (f settingsForms) Updated(ctx context.Context, _ access.Actor, user domainidentity.User) (web.Component, error) {
	return settings.Profile(settings.ProfileProps{
		FormID:         ids.ProfileForm,
		OnSubmit:       f.links.patch(f.links.me, settings.FieldName, settings.FieldUnitSystem, settings.FieldLocale, settings.FieldDateFormat, settings.FieldTheme),
		Email:          user.Email,
		EmailConfirmed: user.EmailConfirmed,
		ResendOn:       f.links.post(f.links.verify),
		Name:           user.Name,
		UnitSystems:    unitSystemOptions(ctx, user.UnitSystem),
		Locale:         user.Locale,
		Locales:        localeOptions(i18n.Supported(), user.Locale),
		DateFormats:    dateFormatOptions(ctx, user.DateFormat),
		Themes:         themeOptions(ctx, user.Theme),
	}), nil
}
