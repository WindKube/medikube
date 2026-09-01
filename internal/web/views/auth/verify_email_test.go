package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/viewstest"
)

// T223h, contracts/pages.md P9. A region rather than a form, which is what the
// gate resolves: <section aria-label="Email confirmation">.

func verifyProps(usable, mailable bool) auth.VerifyEmailProps {
	return auth.VerifyEmailProps{
		FormID:    ids.ConfirmAddressForm,
		OnConfirm: "@post('/api/v1/auth/verify-email/confirm')",
		LoginHref: "/login",
		Token:     tokenFromTheLink,
		Usable:    usable,
		Mailable:  mailable,
	}
}

func TestTheConfirmationLandmarkIsThereWhetherOrNotTheLinkStillWorks(t *testing.T) {
	t.Parallel()

	for name, usable := range map[string]bool{"a link that works": true, "a link that does not": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.VerifyEmail(verifyProps(usable, true)), "div")

			region := tree.One(t, viewstest.Region(auth.EmailConfirmationLandmark))
			assert.NotEmpty(t, viewstest.Text(region),
				"the landmark rendered empty, which passes a presence check and fails the person")
		})
	}
}

// Opening the link changes nothing by itself: the page carries a control that
// posts, so a scanner following links in somebody's mail cannot spend the token
// before they have read the message.
func TestTheConfirmationIsAControlToPressAndNotSomethingTheVisitDoes(t *testing.T) {
	t.Parallel()

	props := verifyProps(true, true)
	tree := viewstest.Render(t, auth.VerifyEmail(props), "div")
	region := tree.One(t, viewstest.Region(auth.EmailConfirmationLandmark))

	control := tree.One(t, viewstest.WithAttr("data-on:click", props.OnConfirm))
	assert.True(t, viewstest.Descends(region, control))
	assert.NotEmpty(t, viewstest.Text(control), "the control has no label to press")

	token := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldToken)))
	assert.Equal(t, props.Token, viewstest.Attr(token, "value"))
	assert.Equal(t, "hidden", viewstest.Attr(token, "type"))
	assert.Equal(t, auth.FieldToken, viewstest.Attr(token, "data-bind"),
		"the token is in the document and not in the submission")

	named := map[string]string{}
	for _, element := range viewstest.Find(region, viewstest.HasAttr("name")) {
		named[viewstest.Attr(element, "name")] = element.Data
	}

	assert.Equal(t, map[string]string{auth.FieldToken: "input"}, named,
		"the confirmation asks the person for something, and there is nothing to ask")
}

// FR-074's state, on FR-075's page. A confirmation link that has expired or has
// already been used is explained inside the landmark, with the one thing that
// can be done about it: sign in and ask for another.
func TestADeadConfirmationLinkIsExplainedInsideTheLandmarkWithTheOfferToAskAgain(t *testing.T) {
	t.Parallel()

	props := verifyProps(false, true)
	tree := viewstest.Render(t, auth.VerifyEmail(props), "div")

	region := tree.One(t, viewstest.Region(auth.EmailConfirmationLandmark))
	explanation := tree.One(t, viewstest.WithID(auth.LinkDeadID))

	assert.True(t, viewstest.Descends(region, explanation),
		"the explanation renders outside the landmark, where the gate cannot see it")
	require.NotEmpty(t, viewstest.Text(explanation))

	offer := tree.One(t, viewstest.And(viewstest.Tag("a"), viewstest.WithAttr("href", props.LoginHref)))
	assert.True(t, viewstest.Descends(explanation, offer),
		"the offer to ask for another is not part of the explanation that says one is needed")

	assert.Empty(t, tree.All(viewstest.HasAttr("data-on:click")),
		"a dead link still offered the control it was going to refuse")
	assert.Empty(t, tree.All(viewstest.HasAttr("name")),
		"a dead link still carried the token it cannot use")

	working := viewstest.Render(t, auth.VerifyEmail(verifyProps(true, true)), "div")
	assert.Empty(t, working.All(viewstest.WithID(auth.LinkDeadID)))
}

// FR-076 again: asking for another confirmation is only an offer on an instance
// that can send one.
func TestADeadConfirmationOnAnInstanceWithNoMailSaysSoRatherThanOfferingALinkNobodyCouldSend(t *testing.T) {
	t.Parallel()

	props := verifyProps(false, false)
	tree := viewstest.Render(t, auth.VerifyEmail(props), "div")

	explanation := tree.One(t, viewstest.WithID(auth.LinkDeadID))
	unconfigured := tree.One(t, viewstest.WithID(auth.MailUnconfiguredID))

	assert.True(t, viewstest.Descends(explanation, unconfigured))
	require.NotEmpty(t, viewstest.Text(unconfigured))

	assert.Empty(t, tree.All(viewstest.And(viewstest.Tag("a"), viewstest.WithAttr("href", props.LoginHref))),
		"the page offers another confirmation on an instance that cannot send one")
}
