package auth_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/identity"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/components"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/viewstest"
)

// T203 and T206, contracts/pages.md P2.

func registerProps(open bool, errs *domain.ValidationError) auth.RegisterProps {
	return auth.RegisterProps{
		FormID:    ids.CreateAccountForm,
		OnSubmit:  "@post('/api/v1/auth/register')",
		LoginHref: "/login",
		Open:      open,
		Rules:     identity.PublishedPasswordRules(),
		Errors:    components.NewFieldErrors(errs),
	}
}

// The landmark is present in BOTH configurations. contracts/pages.md fixes one
// landmark per page and the route is registered unconditionally, so a landmark
// that appeared only when the operator had opened registration would be a page
// the browser gate cannot check on the default configuration (defect D15).
func TestTheSignUpLandmarkIsThereWhicheverWayTheOperatorSetIt(t *testing.T) {
	t.Parallel()

	for name, open := range map[string]bool{"open": true, "closed": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.Register(registerProps(open, nil)), "div")

			form := tree.One(t, viewstest.Form(auth.CreateAccountLandmark))
			assert.NotEmpty(t, viewstest.Text(form),
				"the landmark rendered empty, which passes a presence check and fails the person")
		})
	}
}

// FR-002. Closed, the explanation renders INSIDE the landmark and the controls
// are gone — an instance that still offered the form would collect a password
// it was always going to refuse.
func TestAClosedInstanceExplainsItselfInsideTheLandmarkAndOffersNoForm(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, auth.Register(registerProps(false, nil)), "div")

	form := tree.One(t, viewstest.Form(auth.CreateAccountLandmark))
	explanation := tree.One(t, viewstest.WithID(auth.RegistrationClosedID))

	assert.True(t, viewstest.Descends(form, explanation),
		"the explanation renders outside the landmark, where the gate cannot see it")
	require.NotEmpty(t, viewstest.Text(explanation))

	assert.Empty(t, tree.All(viewstest.HasAttr("name")),
		"a closed instance still rendered controls to fill in")
	assert.Empty(t, tree.All(viewstest.WithAttr("type", "submit")))

	// And the way back to the only thing an existing account holder can do.
	assert.NotEmpty(t, tree.All(viewstest.And(viewstest.Tag("a"), viewstest.WithAttr("href", "/login"))))
}

// FR-001 and FR-012. Three controls, and no member a permission tier or an
// account status could arrive in: the API refuses those by the shape of
// RegisterRequest, and a form that offered them would be inviting a 422.
func TestTheSignUpFormAsksForTheThreeThingsFRoneNamesAndNothingElse(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, auth.Register(registerProps(true, nil)), "div")
	form := tree.One(t, viewstest.Form(auth.CreateAccountLandmark))

	named := map[string]string{}
	for _, control := range viewstest.Find(form, viewstest.HasAttr("name")) {
		named[viewstest.Attr(control, "name")] = control.Data
	}

	assert.Equal(t, map[string]string{
		auth.FieldEmail:    "input",
		auth.FieldName:     "input",
		auth.FieldPassword: "input",
	}, named)
}

// FR-004: the rules are PUBLISHED, before the person chooses. A rule discovered
// by violating it is not published, and the numbers come from the domain value
// the check itself reads, so the sentence and the refusal cannot state
// different lengths.
func TestThePasswordRulesAreStatedBeforeThePersonChoosesOne(t *testing.T) {
	t.Parallel()

	props := registerProps(true, nil)
	tree := viewstest.Render(t, auth.Register(props), "div")

	control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldPassword)))
	rules := tree.One(t, viewstest.WithID(auth.PasswordRulesID))

	assert.Contains(t, viewstest.Text(rules), strconv.Itoa(props.Rules.MinLength))
	assert.Contains(t, viewstest.Attr(control, "aria-describedby"), auth.PasswordRulesID,
		"the control does not point at the rules it is held to")

	if props.Rules.RejectsEmail {
		assert.Contains(t, viewstest.Text(rules), "email")
	}

	if props.Rules.RejectsName {
		assert.Contains(t, viewstest.Text(rules), "name")
	}
}

// FR-048, the same relationship the sign-in form is held to, over every field
// the registration rules can refuse at once.
func TestEverySignUpRefusalIsAdjacentToItsControlAndNamedByAriaDescribedby(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	require.Error(t, identity.ValidatePassword("short", "", ""))

	invalid.Add(auth.FieldEmail, identity.CodeInvalidEmail, "that is not an email address")
	invalid.Add(auth.FieldName, domain.CodeRequired, "a display name is required")
	invalid.Addf(auth.FieldPassword, domain.CodeTooShort, "a password must be at least %d characters",
		identity.PublishedPasswordRules().MinLength)

	props := registerProps(true, &invalid)
	tree := viewstest.Render(t, auth.Register(props), "div")

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

func TestNoSignUpControlDescribesAMessageThatWasNotRendered(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(auth.FieldName, domain.CodeRequired, "a display name is required")

	for name, errs := range map[string]*domain.ValidationError{
		"a clean form":   nil,
		"a refused form": &invalid,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, auth.Register(registerProps(true, errs)), "div")

			for _, control := range tree.All(viewstest.HasAttr("aria-describedby")) {
				for _, described := range splitIDs(viewstest.Attr(control, "aria-describedby")) {
					assert.Lenf(t, tree.All(viewstest.WithID(described)), 1,
						"aria-describedby names %q and nothing has that id", described)
				}
			}
		})
	}
}

// FR-027 again: what was typed comes back, and the password does not.
func TestARefusedSignUpKeepsWhatWasTypedExceptThePassword(t *testing.T) {
	t.Parallel()

	props := registerProps(true, nil)
	props.Email = "new@example.test"
	props.Name = "New Person"

	tree := viewstest.Render(t, auth.Register(props), "div")

	assert.Equal(t, props.Email,
		viewstest.Attr(tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldEmail))), "value"))
	assert.Equal(t, props.Name,
		viewstest.Attr(tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldName))), "value"))
	assert.Empty(t,
		viewstest.Attr(tree.One(t, viewstest.WithID(ids.Field(props.FormID, auth.FieldPassword))), "value"))
}
