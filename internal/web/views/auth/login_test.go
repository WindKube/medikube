package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/components"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/viewstest"
)

// T203, contracts/pages.md P1. The landmark is a Playwright selector, so it is
// asserted as the ARIA pair the gate resolves — <form aria-label="Sign in"> —
// rather than as a substring of the markup.

func loginProps(errs *domain.ValidationError) auth.LoginProps {
	return auth.LoginProps{
		FormID:       ids.SignInForm,
		OnSubmit:     "@post('/api/v1/auth/login')",
		RegisterHref: "/register",
		ForgotHref:   "/forgot-password",
		Errors:       components.NewFieldErrors(errs),
	}
}

func TestTheSignInPageRendersTheLandmarkTheBrowserGateLooksFor(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, auth.Login(loginProps(nil)), "div")

	form := tree.One(t, viewstest.Form(auth.SignInLandmark))
	assert.NotEmpty(t, viewstest.Text(form),
		"the landmark rendered empty, which passes a presence check and fails the person")
}

// FR-005's two controls and nothing else that could carry a privilege. A form
// with a `role` control would be a privilege escalation the API's shape refuses
// and the page offers.
func TestTheSignInFormAsksForAnAddressAndAPasswordAndNothingElse(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, auth.Login(loginProps(nil)), "div")
	form := tree.One(t, viewstest.Form(auth.SignInLandmark))

	named := map[string]string{}
	for _, control := range viewstest.Find(form, viewstest.HasAttr("name")) {
		named[viewstest.Attr(control, "name")] = control.Data
	}

	assert.Equal(t, map[string]string{
		auth.FieldEmail:    "input",
		auth.FieldPassword: "input",
	}, named)
}

// FR-027, applied to the one form where getting it wrong is most annoying: the
// address is typed back into the control, and the password never is.
func TestARefusedSignInKeepsTheAddressAndNeverThePassword(t *testing.T) {
	t.Parallel()

	props := loginProps(nil)
	props.Email = "amara@example.test"

	tree := viewstest.Render(t, auth.Login(props), "div")

	email := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldEmail)))
	assert.Equal(t, props.Email, viewstest.Attr(email, "value"))

	password := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldPassword)))
	assert.Empty(t, viewstest.Attr(password, "value"),
		"the password was rendered back into the document")
}

// FR-048. "Adjacent" is a relationship between elements: the message is the
// control's next sibling and aria-describedby on the control names it. A form
// that renders every message in a block at the top passes a substring check and
// fails the person using a screen reader.
func TestEverySignInRefusalIsAdjacentToItsControlAndNamedByAriaDescribedby(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(auth.FieldEmail, domain.CodeRequired, "an email address is required")
	invalid.Add(auth.FieldPassword, domain.CodeRequired, "a password is required")

	props := loginProps(&invalid)
	tree := viewstest.Render(t, auth.Login(props), "div")

	for _, refusal := range invalid.Fields {
		t.Run(refusal.Field, func(t *testing.T) {
			control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, refusal.Field)))
			messageID := ids.FieldError(props.FormID, refusal.Field)

			assert.Equal(t, messageID, viewstest.Attr(control, "aria-describedby"))
			assert.Equal(t, "true", viewstest.Attr(control, "aria-invalid"))

			message := tree.One(t, viewstest.WithID(messageID))
			assert.Contains(t, viewstest.Text(message), refusal.Message)
			assert.Same(t, message, viewstest.NextElement(control),
				"the message is not adjacent to the control it concerns (FR-048)")
		})
	}
}

// The other half of aria-describedby: a control that names a message nothing
// rendered is a dangling reference, and assistive technology announces silence.
func TestNoSignInControlDescribesAMessageThatWasNotRendered(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(auth.FieldEmail, domain.CodeRequired, "an email address is required")

	for name, errs := range map[string]*domain.ValidationError{
		"a clean form":   nil,
		"a refused form": &invalid,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.Login(loginProps(errs)), "div")

			for _, control := range tree.All(viewstest.HasAttr("aria-describedby")) {
				described := viewstest.Attr(control, "aria-describedby")
				assert.Lenf(t, tree.All(viewstest.WithID(described)), 1,
					"aria-describedby names %q and nothing has that id", described)
			}
		})
	}
}

// FR-008. A session that ran out is explained, and an ordinary visit says
// nothing — otherwise every first visit would claim a session had expired.
func TestAnExpiredSessionIsExplainedInsideTheLandmarkAndOnlyWhenItHappened(t *testing.T) {
	t.Parallel()

	props := loginProps(nil)
	props.SessionExpired = true

	expired := viewstest.Render(t, auth.Login(props), "div")
	form := expired.One(t, viewstest.Form(auth.SignInLandmark))

	explanation := expired.One(t, viewstest.WithID(auth.SessionExpiredID))
	assert.True(t, viewstest.Descends(form, explanation),
		"the explanation renders outside the landmark, where the gate cannot see it")
	require.NotEmpty(t, viewstest.Text(explanation))

	ordinary := viewstest.Render(t, auth.Login(loginProps(nil)), "div")
	assert.Empty(t, ordinary.All(viewstest.WithID(auth.SessionExpiredID)),
		"an ordinary visit is told a session expired that never existed")
}

// The two ways off this page. A sign-in form with no route to registration or
// to recovery is a dead end for everybody who cannot sign in, which is the
// entire audience for FR-073.
func TestTheSignInPageOffersRegistrationAndRecovery(t *testing.T) {
	t.Parallel()

	props := loginProps(nil)
	tree := viewstest.Render(t, auth.Login(props), "div")

	for _, href := range []string{props.RegisterHref, props.ForgotHref} {
		assert.Lenf(t, tree.All(viewstest.And(viewstest.Tag("a"), viewstest.WithAttr("href", href))), 1,
			"the page offers no link to %s", href)
	}
}
