package auth_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/identity"
	serviceidentity "medikube/internal/service/identity"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/components"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/viewstest"
)

// T223h, contracts/pages.md P8.

// tokenFromTheLink stands in for what a real recovery message carries. Nothing
// asserts its shape: the page is handed a token and renders it, and whether it
// resolves is the page handler's question, not this component's.
const tokenFromTheLink = "a-token-from-a-recovery-message"

func resetProps(usable, mailable bool, errs *domain.ValidationError) auth.ResetPasswordProps {
	return auth.ResetPasswordProps{
		FormID:     ids.NewPasswordForm,
		OnSubmit:   "@post('/api/v1/auth/password-reset/confirm')",
		ForgotHref: "/forgot-password",
		Token:      tokenFromTheLink,
		Usable:     usable,
		Mailable:   mailable,
		Rules:      identity.PublishedPasswordRules(),
		Errors:     components.NewFieldErrors(errs),
	}
}

// The landmark is there in BOTH states, and that is contracts/pages.md's whole
// reason for pointing the smoke URL at a deliberately dead token: the page a
// too-late link opens is this page, and the gate opens it to find this
// landmark.
func TestTheNewPasswordLandmarkIsThereWhetherOrNotTheLinkStillWorks(t *testing.T) {
	t.Parallel()

	for name, usable := range map[string]bool{"a link that works": true, "a link that does not": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.ResetPassword(resetProps(usable, true, nil)), "div")

			form := tree.One(t, viewstest.Form(auth.NewPasswordLandmark))
			assert.NotEmpty(t, viewstest.Text(form),
				"the landmark rendered empty, which passes a presence check and fails the person")
		})
	}
}

// FR-074. The refusal is explained INSIDE the landmark, with the offer to ask
// for another — not as an error view, and not as a form that would collect a
// password against a link it was always going to refuse.
func TestADeadRecoveryLinkIsExplainedInsideTheLandmarkWithTheOfferToAskAgain(t *testing.T) {
	t.Parallel()

	props := resetProps(false, true, nil)
	tree := viewstest.Render(t, auth.ResetPassword(props), "div")

	form := tree.One(t, viewstest.Form(auth.NewPasswordLandmark))
	explanation := tree.One(t, viewstest.WithID(auth.LinkDeadID))

	assert.True(t, viewstest.Descends(form, explanation),
		"the explanation renders outside the landmark, where the gate cannot see it")
	require.NotEmpty(t, viewstest.Text(explanation))

	offer := tree.One(t, viewstest.And(viewstest.Tag("a"), viewstest.WithAttr("href", props.ForgotHref)))
	assert.True(t, viewstest.Descends(explanation, offer),
		"the offer to ask for another link is not part of the explanation that says one is needed")

	assert.Empty(t, tree.All(viewstest.HasAttr("name")),
		"a dead link still offered controls to fill in")
	assert.Empty(t, tree.All(viewstest.WithAttr("type", "submit")))

	// And the working link says none of it, or every visit would be told the
	// link it just followed was dead.
	working := viewstest.Render(t, auth.ResetPassword(resetProps(true, true, nil)), "div")
	assert.Empty(t, working.All(viewstest.WithID(auth.LinkDeadID)))
}

// FR-074 and FR-076 together. "Ask for another" is only an offer on an instance
// that can send one; on one that cannot, the page says that instead of linking
// to a page that would say it a click later.
func TestADeadLinkOnAnInstanceWithNoMailSaysSoRatherThanOfferingALinkNobodyCouldSend(t *testing.T) {
	t.Parallel()

	props := resetProps(false, false, nil)
	tree := viewstest.Render(t, auth.ResetPassword(props), "div")

	explanation := tree.One(t, viewstest.WithID(auth.LinkDeadID))
	unconfigured := tree.One(t, viewstest.WithID(auth.MailUnconfiguredID))

	assert.True(t, viewstest.Descends(explanation, unconfigured))
	require.NotEmpty(t, viewstest.Text(unconfigured))

	assert.Empty(t, tree.All(viewstest.And(viewstest.Tag("a"), viewstest.WithAttr("href", props.ForgotHref))),
		"the page offers another link on an instance that cannot send one")
}

// The token travels back with the new password, and the old password is never
// asked for: somebody who has one does not need this page.
func TestTheNewPasswordFormCarriesTheTokenAndAsksForTheTwoPasswordsAndNothingElse(t *testing.T) {
	t.Parallel()

	props := resetProps(true, true, nil)
	tree := viewstest.Render(t, auth.ResetPassword(props), "div")
	form := tree.One(t, viewstest.Form(auth.NewPasswordLandmark))

	named := map[string]string{}
	for _, control := range viewstest.Find(form, viewstest.HasAttr("name")) {
		named[viewstest.Attr(control, "name")] = control.Data
	}

	assert.Equal(t, map[string]string{
		auth.FieldToken:           "input",
		auth.FieldPassword:        "input",
		auth.FieldPasswordConfirm: "input",
	}, named)

	token := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldToken)))
	assert.Equal(t, props.Token, viewstest.Attr(token, "value"))
	assert.Equal(t, "hidden", viewstest.Attr(token, "type"))
	assert.Equal(t, auth.FieldToken, viewstest.Attr(token, "data-bind"),
		"the token is in the document and not in the submission")

	// Neither password is rendered back, ever: there is nothing here that was
	// typed on a previous attempt worth returning, and a password in the
	// document is a password in the browser's history.
	for _, field := range []string{auth.FieldPassword, auth.FieldPasswordConfirm} {
		control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, field)))
		assert.Empty(t, viewstest.Attr(control, "value"))
	}
}

// FR-004 and FR-074: one rule set, stated wherever a password is chosen, from
// the domain value the check itself reads.
func TestTheRecoveryPasswordRulesAreStatedBeforeThePersonChoosesOne(t *testing.T) {
	t.Parallel()

	props := resetProps(true, true, nil)
	tree := viewstest.Render(t, auth.ResetPassword(props), "div")

	control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldPassword)))
	rules := tree.One(t, viewstest.WithID(auth.NewPasswordRulesID))

	assert.Contains(t, viewstest.Text(rules), strconv.Itoa(props.Rules.MinLength))
	assert.Contains(t, viewstest.Attr(control, "aria-describedby"), auth.NewPasswordRulesID,
		"the control does not point at the rules it is held to")

	// The same sentences the sign-up form states, because they are the same
	// rules and a person who met them once should not have to read them twice
	// in two different sets of words.
	assert.Equal(t, auth.RegisterProps{Rules: props.Rules}.RuleSentences(t.Context()), props.RuleSentences(t.Context()))
}

// FR-048, over both controls the confirmation can refuse at once.
func TestEveryNewPasswordRefusalIsAdjacentToItsControlAndNamedByAriaDescribedby(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Addf(auth.FieldPassword, domain.CodeTooShort, "a password must be at least %d characters",
		identity.PublishedPasswordRules().MinLength)
	invalid.Add(auth.FieldPasswordConfirm, serviceidentity.CodeMismatch, "the two passwords are not the same")

	props := resetProps(true, true, &invalid)
	tree := viewstest.Render(t, auth.ResetPassword(props), "div")

	for _, refusal := range invalid.Fields {
		t.Run(refusal.Field, func(t *testing.T) {
			control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, refusal.Field)))
			messageID := ids.FieldError(props.FormID, refusal.Field)

			assert.Contains(t, viewstest.Attr(control, "aria-describedby"), messageID)
			assert.Equal(t, "true", viewstest.Attr(control, "aria-invalid"))

			message := tree.One(t, viewstest.WithID(messageID))
			assert.NotEmpty(t, viewstest.Text(message))
			assert.Same(t, message, viewstest.NextElement(control),
				"the message is not adjacent to the control it concerns (FR-048)")
		})
	}
}

func TestNoNewPasswordControlDescribesAMessageThatWasNotRendered(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(auth.FieldPasswordConfirm, serviceidentity.CodeMismatch, "the two passwords are not the same")

	for name, errs := range map[string]*domain.ValidationError{
		"a clean form":   nil,
		"a refused form": &invalid,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.ResetPassword(resetProps(true, true, errs)), "div")

			for _, control := range tree.All(viewstest.HasAttr("aria-describedby")) {
				for _, described := range splitIDs(viewstest.Attr(control, "aria-describedby")) {
					assert.Lenf(t, tree.All(viewstest.WithID(described)), 1,
						"aria-describedby names %q and nothing has that id", described)
				}
			}
		})
	}
}
