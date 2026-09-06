package settings_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/web/views/components"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/settings"
	"medikube/internal/web/views/viewstest"
)

// T204, contracts/pages.md P6. The three parts of the account surface each
// render INSIDE region[name="Settings"] — the landmark the browser gate
// resolves — rather than beside it, because a landmark the page's own content
// sits outside of is a landmark that passes a presence check while the page is
// broken.

// holdings is what the danger zone states will be destroyed. Text arrives
// already resolved (D-06): the page, not the template, is what knows both
// the kind and the count.
const holdingText = "12 widgets"

func props(confirmed bool, errs *domain.ValidationError) settings.SettingsProps {
	refusals := components.NewFieldErrors(errs)

	return settings.SettingsProps{
		SignOutOn: "@post('/api/v1/auth/logout')",
		Profile: settings.ProfileProps{
			FormID:         ids.ProfileForm,
			OnSubmit:       "@patch('/api/v1/me')",
			Email:          "amara@example.test",
			EmailConfirmed: confirmed,
			ResendOn:       "@post('/api/v1/auth/verify-email')",
			Name:           "Amara Okonkwo",
			UnitSystems:    options(domainidentity.UnitSystems(), domainidentity.DefaultUnitSystem),
			Locale:         domainidentity.DefaultLocale,
			Locales:        localeOptionsFixture(domainidentity.DefaultLocale),
			DateFormats:    options(domainidentity.DateFormats(), domainidentity.DefaultDateFormat),
			Themes:         options(domainidentity.Themes(), domainidentity.DefaultTheme),
			Errors:         refusals,
		},
		Password: settings.PasswordProps{
			FormID:   ids.PasswordForm,
			OnSubmit: "@put('/api/v1/me/password')",
			Rules:    domainidentity.PublishedPasswordRules(),
			Errors:   refusals,
		},
		Danger: settings.DangerZoneProps{
			FormID:   ids.DeleteAccountForm,
			OnSubmit: "@delete('/api/v1/me')",
			Phrase:   domainidentity.DeleteConfirmationPhrase,
			Holdings: []settings.Holding{{Text: holdingText}},
			Errors:   refusals,
		},
	}
}

// localeOptionsFixture stands in for page.localeOptions, which this package
// cannot import (views does not import page). Two shipped languages, English
// and Polish, mirrors internal/i18n/locales' own set.
func localeOptionsFixture(selected string) []settings.Option {
	return []settings.Option{
		{Value: "en", Label: "English", Selected: selected == "en"},
		{Value: "pl", Label: "Polski", Selected: selected == "pl"},
	}
}

func options[T ~string](values []T, selected T) []settings.Option {
	rendered := make([]settings.Option, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, settings.Option{
			Value:    string(value),
			Label:    string(value),
			Selected: value == selected,
		})
	}

	return rendered
}

func render(t *testing.T, p settings.SettingsProps) viewstest.Tree {
	t.Helper()

	return viewstest.Render(t, settings.Settings(p), "div")
}

func TestEveryPartOfTheAccountSurfaceRendersInsideTheSettingsLandmark(t *testing.T) {
	t.Parallel()

	tree := render(t, props(true, nil))
	region := tree.One(t, viewstest.Region(settings.SettingsLandmark))

	for name, match := range map[string]viewstest.Matcher{
		settings.ProfileLandmark:       viewstest.Form(settings.ProfileLandmark),
		settings.PasswordLandmark:      viewstest.Form(settings.PasswordLandmark),
		settings.DangerZoneLandmark:    viewstest.Region(settings.DangerZoneLandmark),
		settings.DeleteAccountLandmark: viewstest.Form(settings.DeleteAccountLandmark),
	} {
		t.Run(name, func(t *testing.T) {
			part := tree.One(t, match)

			assert.True(t, viewstest.Descends(region, part),
				"%s renders outside region[name=%q]", name, settings.SettingsLandmark)
			assert.NotEmpty(t, viewstest.Text(part), "%s rendered empty", name)
		})
	}
}

// FR-011 names five things and the shape of MePatch accepts exactly those five.
// A control for `role`, `email` or `verified` would be a control that collects
// a value the API answers 422 to.
func TestTheProfileFormOffersTheFiveThingsFRelevenNamesAndNothingElse(t *testing.T) {
	t.Parallel()

	tree := render(t, props(true, nil))
	form := tree.One(t, viewstest.Form(settings.ProfileLandmark))

	named := make([]string, 0, 5)
	for _, control := range viewstest.Find(form, viewstest.HasAttr("name")) {
		named = append(named, viewstest.Attr(control, "name"))
	}

	assert.ElementsMatch(t, []string{
		settings.FieldName, settings.FieldUnitSystem, settings.FieldLocale,
		settings.FieldDateFormat, settings.FieldTheme,
	}, named)
}

// T018: the language control is a <select> over the shipped languages, each
// labelled by its own name, and it carries the catalogue's aria-label rather
// than relying on the visible <label> alone.
func TestTheLanguageControlIsASelectOverTheShippedLanguages(t *testing.T) {
	t.Parallel()

	p := props(true, nil)
	tree := render(t, p)

	control := tree.One(t, viewstest.WithID(ids.Field(p.Profile.FormID, settings.FieldLocale)))
	assert.Equal(t, "select", control.Data)
	assert.Equal(t, "Language", viewstest.Attr(control, "aria-label"))

	labels := make([]string, 0, 2)
	for _, option := range viewstest.Find(control, viewstest.Tag("option")) {
		labels = append(labels, viewstest.Text(option))
	}

	assert.ElementsMatch(t, []string{"English", "Polski"}, labels)
}

// The address is shown and is not a control (contracts/account.md): FR-011 does
// not list it, MePatch has no member for it, and a form that offered it would
// be collecting a value the API refuses.
func TestTheAddressIsShownAndIsNotSomethingThisPageCanChange(t *testing.T) {
	t.Parallel()

	p := props(true, nil)
	tree := render(t, p)
	form := tree.One(t, viewstest.Form(settings.ProfileLandmark))

	assert.Contains(t, viewstest.Text(form), p.Profile.Email)

	for _, control := range viewstest.Find(form, viewstest.HasAttr("name")) {
		assert.NotEqual(t, "email", viewstest.Attr(control, "name"),
			"the page offers a control for an address this version cannot change")
	}
}

// FR-075's third clause, both ways round. Exactly one of the two states is
// rendered, and the offer to send the message again belongs to the unconfirmed
// one — an account whose address is confirmed has nothing to resend.
func TestAnUnconfirmedAddressSaysSoAndOffersToSendItAgain(t *testing.T) {
	t.Parallel()

	unconfirmed := render(t, props(false, nil))

	state := unconfirmed.One(t, viewstest.WithID(settings.EmailUnconfirmedID))
	require.NotEmpty(t, viewstest.Text(state))
	assert.Empty(t, unconfirmed.All(viewstest.WithID(settings.EmailConfirmedID)))

	resend := viewstest.Find(state, viewstest.Tag("button"))
	require.Len(t, resend, 1, "the unconfirmed state offers no way to send the message again")
	assert.Equal(t, props(false, nil).Profile.ResendOn, viewstest.Attr(resend[0], "data-on:click"))

	confirmed := render(t, props(true, nil))
	assert.NotEmpty(t, confirmed.All(viewstest.WithID(settings.EmailConfirmedID)))
	assert.Empty(t, confirmed.All(viewstest.WithID(settings.EmailUnconfirmedID)),
		"a confirmed address is asked to confirm itself")
}

// FR-013's first clause: the consequence is stated PLAINLY BEFOREHAND, and it
// names what will be destroyed rather than asking the person to take it on
// trust. The count and the label are data; a view that knew the kind's plural
// would be spelling it a fourth time.
func TestTheDangerZoneStatesWhatWillBeDestroyedBeforeAskingForAnything(t *testing.T) {
	t.Parallel()

	p := props(true, nil)
	tree := render(t, p)

	zone := tree.One(t, viewstest.Region(settings.DangerZoneLandmark))
	holdings := tree.One(t, viewstest.WithID(settings.HoldingsID))
	form := tree.One(t, viewstest.Form(settings.DeleteAccountLandmark))

	assert.True(t, viewstest.Descends(zone, holdings))
	assert.Contains(t, viewstest.Text(holdings), holdingText)

	// Plainly, and before either credential is offered. "cannot be undone" is
	// the sentence FR-013 asks for; the ordering is what "beforehand" means.
	assert.Contains(t, viewstest.Text(zone), "cannot be undone")

	children := viewstest.Elements(zone)
	holdingsAt, formAt := -1, -1

	for index, child := range children {
		if viewstest.Descends(child, holdings) {
			holdingsAt = index
		}

		if viewstest.Descends(child, form) {
			formAt = index
		}
	}

	require.NotEqual(t, -1, holdingsAt)
	require.NotEqual(t, -1, formAt)
	assert.Less(t, holdingsAt, formAt, "the form is offered before the consequence is stated")
}

// FR-013's two proofs, and the phrase spelled by the domain rather than by the
// form. A form asking for "delete my account" against a check comparing
// "DELETE MY ACCOUNT" is a deletion nobody can complete.
func TestTheDeletionFormAsksForThePasswordAndTheExactPhrase(t *testing.T) {
	t.Parallel()

	tree := render(t, props(true, nil))
	form := tree.One(t, viewstest.Form(settings.DeleteAccountLandmark))

	named := make([]string, 0, 2)
	for _, control := range viewstest.Find(form, viewstest.HasAttr("name")) {
		named = append(named, viewstest.Attr(control, "name"))
	}

	assert.ElementsMatch(t, []string{settings.FieldPassword, settings.FieldConfirmation}, named)
	assert.Contains(t, viewstest.Text(form), domainidentity.DeleteConfirmationPhrase)
}

// FR-007 is a surprise unless it is said out loud: signing out here signs the
// person out of every other place they were still signed in, and so does
// changing the password (contracts/auth.md, contracts/account.md).
func TestThePageSaysThatOneSignOutEndsEverySession(t *testing.T) {
	t.Parallel()

	p := props(true, nil)
	tree := render(t, p)
	region := tree.One(t, viewstest.Region(settings.SettingsLandmark))

	signOut := tree.All(viewstest.WithAttr("data-on:click", p.SignOutOn))
	require.Len(t, signOut, 1, "the settings page offers no way to sign out")
	assert.True(t, viewstest.Descends(region, signOut[0]))

	assert.Contains(t, viewstest.Text(region), "every")
	assert.Contains(t, viewstest.Text(tree.One(t, viewstest.Form(settings.PasswordLandmark))),
		"other session")
}

// FR-048, over every control on the page at once: each refusal is its control's
// next sibling and the control names it.
func TestEverySettingsRefusalIsAdjacentToItsControlAndNamedByAriaDescribedby(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(settings.FieldName, domain.CodeRequired, "a display name is required")
	invalid.Add(settings.FieldTheme, domain.CodeInvalidValue, "a theme is one of: system, light, dark")
	invalid.Add(settings.FieldCurrentPassword, "incorrect", "that password is not correct")
	invalid.Add(settings.FieldNewPassword, domain.CodeTooShort, "a password must be at least 8 characters")
	invalid.Add(settings.FieldConfirmation, "mismatch", "type the phrase exactly")

	p := props(true, &invalid)
	tree := render(t, p)

	forms := map[string]string{
		settings.FieldName:            p.Profile.FormID,
		settings.FieldTheme:           p.Profile.FormID,
		settings.FieldCurrentPassword: p.Password.FormID,
		settings.FieldNewPassword:     p.Password.FormID,
		settings.FieldConfirmation:    p.Danger.FormID,
	}

	for _, refusal := range invalid.Fields {
		t.Run(refusal.Field, func(t *testing.T) {
			formID := forms[refusal.Field]

			control := tree.One(t, viewstest.WithID(ids.Field(formID, refusal.Field)))
			messageID := ids.FieldError(formID, refusal.Field)

			assert.Contains(t, viewstest.Attr(control, "aria-describedby"), messageID)
			assert.Equal(t, "true", viewstest.Attr(control, "aria-invalid"))

			message := tree.One(t, viewstest.WithID(messageID))
			assert.NotEmpty(t, viewstest.Text(message))
			assert.Same(t, message, viewstest.NextElement(control),
				"the message is not adjacent to the control it concerns (FR-048)")
		})
	}
}

// The three forms live on one page, so their controls must not collide: two
// password controls sharing an id would make one label point at the other's
// control and one refusal describe the wrong thing.
func TestNoTwoControlsOnThePageShareAnID(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(settings.FieldPassword, "incorrect", "that password is not correct")

	tree := render(t, props(true, &invalid))

	seen := map[string]int{}
	for _, element := range tree.All(viewstest.HasAttr("id")) {
		seen[viewstest.Attr(element, "id")]++
	}

	for id, count := range seen {
		assert.Equalf(t, 1, count, "%q is the id of %d elements", id, count)
	}
}

func TestNoSettingsControlDescribesAMessageThatWasNotRendered(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(settings.FieldNewPassword, domain.CodeTooShort, "a password must be at least 8 characters")

	for name, errs := range map[string]*domain.ValidationError{
		"a clean page":   nil,
		"a refused page": &invalid,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := render(t, props(true, errs))

			for _, control := range tree.All(viewstest.HasAttr("aria-describedby")) {
				for _, described := range describedIDs(viewstest.Attr(control, "aria-describedby")) {
					assert.Lenf(t, tree.All(viewstest.WithID(described)), 1,
						"aria-describedby names %q and nothing has that id", described)
				}
			}
		})
	}
}

// FR-004 and FR-074: a password chosen here is held to the rules registration
// publishes, so the page states them here too rather than letting a person
// discover them by being refused.
func TestTheChangeFormPublishesTheSameRulesRegistrationDoes(t *testing.T) {
	t.Parallel()

	p := props(true, nil)
	tree := render(t, p)

	rules := tree.One(t, viewstest.WithID(settings.PasswordRulesID))
	assert.Contains(t, viewstest.Text(rules), strconv.Itoa(p.Password.Rules.MinLength))

	control := tree.One(t, viewstest.WithID(ids.Field(p.Password.FormID, settings.FieldNewPassword)))
	assert.Contains(t, viewstest.Attr(control, "aria-describedby"), settings.PasswordRulesID)
}
