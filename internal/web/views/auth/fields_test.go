package auth_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/api"
	"medikube/internal/web/views/auth"
)

// The mechanical tie between the control names this package spells and the
// members the API decodes, in the settings package's shape and for its reason.
//
// The view does not import the API — a view that imported the edge would be a
// view that could answer a request — so the agreement is asserted here. A
// rename on either side is a failing test rather than a form posting a member
// nobody reads, which reaches the person as a 422 against a control they cannot
// see.
func TestEveryAuthControlNameIsAMemberTheAPIAccepts(t *testing.T) {
	t.Parallel()

	for name, pair := range map[string]struct {
		body  any
		field string
		tag   string
	}{
		"the address a sign-in gives":     {api.LoginRequest{}, "Email", auth.FieldEmail},
		"the password a sign-in gives":    {api.LoginRequest{}, "Password", auth.FieldPassword},
		"the display name a sign-up puts": {api.RegisterRequest{}, "Name", auth.FieldName},
		"the address recovery asks for":   {api.PasswordResetRequest{}, "Email", auth.FieldEmail},
		"the recovery link's token":       {api.PasswordResetConfirm{}, "Token", auth.FieldToken},
		"the new password":                {api.PasswordResetConfirm{}, "Password", auth.FieldPassword},
		"the new password again":          {api.PasswordResetConfirm{}, "PasswordConfirm", auth.FieldPasswordConfirm},
		"the confirmation link's token":   {api.EmailVerificationConfirm{}, "Token", auth.FieldToken},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, pair.tag, jsonName(t, pair.body, pair.field))
		})
	}
}

// jsonName reads the member's wire spelling out of the struct tag, so the
// assertion is against what encoding/json actually reads rather than against a
// second copy of the tag.
func jsonName(t *testing.T, value any, field string) string {
	t.Helper()

	member, found := reflect.TypeOf(value).FieldByName(field)
	require.Truef(t, found, "%T has no %s member", value, field)

	var tag string
	require.NoError(t, json.Unmarshal([]byte(`"`+member.Tag.Get("json")+`"`), &tag))

	for index, char := range tag {
		if char == ',' {
			return tag[:index]
		}
	}

	return tag
}
