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

// T223h, contracts/pages.md P7. The landmark is a Playwright selector, so it is
// asserted as the ARIA pair the gate resolves — <form aria-label="Reset
// password"> — rather than as a substring of the markup.

func forgotProps(mailable bool, errs *domain.ValidationError) auth.ForgotPasswordProps {
	return auth.ForgotPasswordProps{
		FormID:    ids.ForgotPasswordForm,
		OnSubmit:  "@post('/api/v1/auth/password-reset')",
		LoginHref: "/login",
		Mailable:  mailable,
		Errors:    components.NewFieldErrors(errs),
	}
}

// The landmark is present in BOTH configurations, for the reason P2's is: the
// route is registered unconditionally, so a landmark that appeared only on an
// instance with mail would be a page the browser gate cannot check on the
// default one — and the default one has no mail configured.
func TestTheRecoveryLandmarkIsThereWhetherOrNotTheInstanceCanSendMail(t *testing.T) {
	t.Parallel()

	for name, mailable := range map[string]bool{"with mail": true, "with no mail": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.ForgotPassword(forgotProps(mailable, nil)), "div")

			form := tree.One(t, viewstest.Form(auth.ResetPasswordLandmark))
			assert.NotEmpty(t, viewstest.Text(form),
				"the landmark rendered empty, which passes a presence check and fails the person")
		})
	}
}

// FR-073's one control. An address is all this asks for: anything else would be
// a second thing to answer differently for an address that has an account.
func TestTheRecoveryFormAsksForAnAddressAndNothingElse(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, auth.ForgotPassword(forgotProps(true, nil)), "div")
	form := tree.One(t, viewstest.Form(auth.ResetPasswordLandmark))

	named := map[string]string{}
	for _, control := range viewstest.Find(form, viewstest.HasAttr("name")) {
		named[viewstest.Attr(control, "name")] = control.Data
	}

	assert.Equal(t, map[string]string{auth.FieldEmail: "input"}, named)
}

// FR-076. An instance that cannot send mail says so INSIDE the landmark and
// offers no control — collecting an address it can do nothing with is accepting
// the request as though it had succeeded, which is the thing FR-076 forbids.
func TestAnInstanceThatCannotSendMailExplainsItselfInsideTheLandmarkAndOffersNoForm(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, auth.ForgotPassword(forgotProps(false, nil)), "div")

	form := tree.One(t, viewstest.Form(auth.ResetPasswordLandmark))
	explanation := tree.One(t, viewstest.WithID(auth.MailUnconfiguredID))

	assert.True(t, viewstest.Descends(form, explanation),
		"the explanation renders outside the landmark, where the gate cannot see it")
	require.NotEmpty(t, viewstest.Text(explanation))

	assert.Empty(t, tree.All(viewstest.HasAttr("name")),
		"an instance with no mail still asked for an address")
	assert.Empty(t, tree.All(viewstest.WithAttr("type", "submit")))

	// And the way back, which is the only thing left to do here.
	assert.NotEmpty(t, tree.All(viewstest.And(viewstest.Tag("a"), viewstest.WithAttr("href", "/login"))))

	// The control and the explanation are alternatives, not a form with a
	// warning above it: an instance that can send mail says none of this.
	mailable := viewstest.Render(t, auth.ForgotPassword(forgotProps(true, nil)), "div")
	assert.Empty(t, mailable.All(viewstest.WithID(auth.MailUnconfiguredID)))
}

// FR-048, the relationship the other two forms are held to. The message is the
// control's next sibling and the control names it through aria-describedby.
func TestTheRecoveryRefusalIsAdjacentToItsControlAndNamedByAriaDescribedby(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(auth.FieldEmail, domain.CodeRequired, "an email address is required")

	props := forgotProps(true, &invalid)
	tree := viewstest.Render(t, auth.ForgotPassword(props), "div")

	control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldEmail)))
	messageID := ids.FieldError(props.FormID, auth.FieldEmail)

	assert.Equal(t, messageID, viewstest.Attr(control, "aria-describedby"))
	assert.Equal(t, "true", viewstest.Attr(control, "aria-invalid"))

	message := tree.One(t, viewstest.WithID(messageID))
	assert.NotEmpty(t, viewstest.Text(message))
	assert.Same(t, message, viewstest.NextElement(control),
		"the message is not adjacent to the control it concerns (FR-048)")
}

func TestNoRecoveryControlDescribesAMessageThatWasNotRendered(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(auth.FieldEmail, domain.CodeRequired, "an email address is required")

	for name, errs := range map[string]*domain.ValidationError{
		"a clean form":   nil,
		"a refused form": &invalid,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.ForgotPassword(forgotProps(true, errs)), "div")

			for _, control := range tree.All(viewstest.HasAttr("aria-describedby")) {
				for _, described := range splitIDs(viewstest.Attr(control, "aria-describedby")) {
					assert.Lenf(t, tree.All(viewstest.WithID(described)), 1,
						"aria-describedby names %q and nothing has that id", described)
				}
			}
		})
	}
}

// FR-027. The address comes back, because the person who mistyped a password
// rule here did not mistype their own address.
func TestARefusedRecoveryRequestKeepsTheAddressThatWasTyped(t *testing.T) {
	t.Parallel()

	props := forgotProps(true, nil)
	props.Email = "amara@example.test"

	tree := viewstest.Render(t, auth.ForgotPassword(props), "div")

	control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldEmail)))
	assert.Equal(t, props.Email, viewstest.Attr(control, "value"))
}
