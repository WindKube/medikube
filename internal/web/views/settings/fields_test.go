package settings_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainidentity "medikube/internal/domain/identity"
	serviceidentity "medikube/internal/service/identity"
	"medikube/internal/web/api"
	"medikube/internal/web/views/settings"
)

// The mechanical tie between the control names this package spells and the
// members the API accepts and the service raises its refusals against.
//
// The view does not import either — a view that imported a service would be a
// view that could call one — so the agreement is asserted here instead. A
// rename in the DTO or in the service is a failing test rather than a form that
// posts a member nobody reads and displays a refusal against a control nobody
// can see.

func TestEveryControlNameIsAMemberTheAPIAccepts(t *testing.T) {
	t.Parallel()

	for name, pair := range map[string]struct {
		body  any
		field string
		tag   string
	}{
		"display name":     {api.MePatch{}, "Name", settings.FieldName},
		"measurement":      {api.MePatch{}, "UnitSystem", settings.FieldUnitSystem},
		"language":         {api.MePatch{}, "Locale", settings.FieldLocale},
		"date format":      {api.MePatch{}, "DateFormat", settings.FieldDateFormat},
		"appearance":       {api.MePatch{}, "Theme", settings.FieldTheme},
		"current password": {api.ChangePasswordRequest{}, "CurrentPassword", settings.FieldCurrentPassword},
		"new password":     {api.ChangePasswordRequest{}, "NewPassword", settings.FieldNewPassword},
		"the password":     {api.DeleteAccountRequest{}, "Password", settings.FieldPassword},
		"the phrase":       {api.DeleteAccountRequest{}, "Confirmation", settings.FieldConfirmation},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, pair.tag, jsonName(t, pair.body, pair.field))
		})
	}
}

func TestEveryControlNameIsWhatTheServiceRaisesItsRefusalAgainst(t *testing.T) {
	t.Parallel()

	assert.Equal(t, serviceidentity.FieldCurrentPassword, settings.FieldCurrentPassword)
	assert.Equal(t, serviceidentity.FieldConfirmation, settings.FieldConfirmation)
	assert.Equal(t, domainidentity.FieldNewPassword, settings.FieldNewPassword)
	assert.Equal(t, domainidentity.FieldPassword, settings.FieldPassword)
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
