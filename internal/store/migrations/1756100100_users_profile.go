package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"medikube/internal/domain/identity"
)

// data-model §1's column bounds. The domain enforces the same numbers in
// identity.User.Validate; a column that disagreed with the entity would refuse
// a value the person was told was acceptable, at the storage layer, after the
// form said yes.
const (
	usersNameMin = 1
	usersNameMax = 120

	// A two-letter language and an optional two-letter region: `pt-BR` is five.
	usersLocaleMax = 10

	// FR-004's published floor, enforced at the storage layer as well as in the
	// domain. data-model §1 calls this PasswordAuth.MinPasswordLength; that
	// member does not exist in PocketBase v0.40.1 — the floor is the password
	// field's own Min, which is where this writes it.
	usersPasswordMin = 8

	// FR-008's session lifetime, seven days. data-model §1 sources this from
	// MEDIKUBE_AUTH_SESSION_TTL, but a migration runs once and would then pin
	// the value of whatever environment first booted the instance. The constant
	// is the schema's baseline; the boot-time settings writer is what applies
	// the operator's configured TTL on every start.
	usersAuthTokenDuration int64 = 7 * 24 * 60 * 60
)

// The stock values PocketBase's own initial migration wrote, restored verbatim
// by the down. A reversal that leaves the collection in a state PocketBase
// never created is not a reversal (data-model §5).
const (
	stockUsersOwnerRule    = "id = @request.auth.id"
	stockUsersNameMax      = 255
	stockUsersAvatarField  = "avatar"
	stockUsersTokenSeconds = 432000
)

var stockUsersAvatarMimeTypes = []string{
	"image/jpeg",
	"image/png",
	"image/svg+xml",
	"image/gif",
	"image/webp",
}

// The seven MediKube columns of data-model §1, in the order that document lists
// them, which is the order the profile form renders.
const (
	usersFieldName       = "name"
	usersFieldRole       = "role"
	usersFieldUnitSystem = "unit_system"
	usersFieldLocale     = "locale"
	usersFieldDateFormat = "date_format"
	usersFieldTheme      = "theme"
	usersFieldDisabledAt = "disabled_at"
)

// usersAddedFields is what the down removes. `name` is not in it: PocketBase
// ships a `name` column and this migration narrows it rather than adding it, so
// the down restores it instead of dropping it.
var usersAddedFields = []string{
	usersFieldRole,
	usersFieldUnitSystem,
	usersFieldLocale,
	usersFieldDateFormat,
	usersFieldTheme,
	usersFieldDisabledAt,
}

// usersEmailLowerIndex is FR-003: two addresses differing only in letter case
// are the same address. PocketBase's own idx_email__pb_users_auth_ is a unique
// index on the raw column and stays; dbutils only recognises a single-column
// unique index by its literal column name, so LOWER(email) is an additional
// index rather than a replacement for it.
const usersEmailLowerIndex = "idx_users_email_lower"

func init() {
	register(usersProfileUp, usersProfileDown)
}

func usersProfileUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	lockRules(users)

	// Not one of the five. nil here disables authentication altogether —
	// password, OAuth2, OTP, all of it — and FR-005 depends on this staying an
	// empty string.
	//
	// Rewriting it as a fresh pointer to the same value does not rotate the
	// collection's auth-token secret: core/collection_model.go:862 compares the
	// dereferenced strings as well as the pointers.
	users.AuthRule = types.Pointer("")
	users.ManageRule = nil

	// data-model §0: zero FileFields exist in this phase, and the boot
	// assertion refuses to start on an unprotected one. PocketBase ships
	// `avatar` unprotected, so it goes — with the OAuth2 mapping that points at
	// it, which PocketBase would otherwise clear on save behind our back.
	users.Fields.RemoveByName(stockUsersAvatarField)
	users.OAuth2.MappedFields.AvatarURL = ""

	users.Fields.Add(&core.TextField{
		Name:     usersFieldName,
		Required: true,
		Min:      usersNameMin,
		Max:      usersNameMax,
	})
	users.Fields.Add(&core.SelectField{
		Name:      usersFieldRole,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(identity.Roles()),
	})
	users.Fields.Add(&core.SelectField{
		Name:      usersFieldUnitSystem,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(identity.UnitSystems()),
	})
	users.Fields.Add(&core.TextField{
		Name:     usersFieldLocale,
		Required: true,
		Max:      usersLocaleMax,
	})
	users.Fields.Add(&core.SelectField{
		Name:      usersFieldDateFormat,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(identity.DateFormats()),
	})
	users.Fields.Add(&core.SelectField{
		Name:      usersFieldTheme,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(identity.Themes()),
	})

	// Non-empty means sign-in is refused. The column is a date and therefore
	// TEXT DEFAULT '' NOT NULL, so "not disabled" is the empty string and never
	// SQL NULL — every reader of this column compares against the zero date.
	users.Fields.Add(&core.DateField{Name: usersFieldDisabledAt})

	if err := setUsersPasswordMin(users, usersPasswordMin); err != nil {
		return err
	}

	users.PasswordAuth.Enabled = true
	users.PasswordAuth.IdentityFields = []string{core.FieldNameEmail}

	// Phase 006 owns external sign-in. Nothing in this phase turns it on.
	users.OAuth2.Enabled = false
	users.MFA.Enabled = false

	users.AuthToken.Duration = usersAuthTokenDuration

	users.AddIndex(usersEmailLowerIndex, true, "LOWER(email)", "")

	if err := app.Save(users); err != nil {
		return fmt.Errorf("saving %s: %w", usersCollection, err)
	}

	return nil
}

func usersProfileDown(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	users.ListRule = types.Pointer(stockUsersOwnerRule)
	users.ViewRule = types.Pointer(stockUsersOwnerRule)
	users.CreateRule = types.Pointer("")
	users.UpdateRule = types.Pointer(stockUsersOwnerRule)
	users.DeleteRule = types.Pointer(stockUsersOwnerRule)

	for _, field := range usersAddedFields {
		users.Fields.RemoveByName(field)
	}

	users.Fields.Add(&core.TextField{Name: usersFieldName, Max: stockUsersNameMax})

	// Back in the position PocketBase put it: immediately after `name`, before
	// the two autodate columns. Appending it at the end would leave a field
	// order the stock collection never had.
	users.Fields.AddAt(
		stockAvatarPosition(users),
		&core.FileField{
			Name:      stockUsersAvatarField,
			MaxSelect: 1,
			MimeTypes: stockUsersAvatarMimeTypes,
		},
	)
	users.OAuth2.MappedFields.AvatarURL = stockUsersAvatarField

	if err := setUsersPasswordMin(users, usersPasswordMin); err != nil {
		return err
	}

	users.AuthToken.Duration = stockUsersTokenSeconds

	users.RemoveIndex(usersEmailLowerIndex)

	if err := app.Save(users); err != nil {
		return fmt.Errorf("saving %s: %w", usersCollection, err)
	}

	return nil
}

// stockAvatarPosition is the index of the first autodate column, which is where
// PocketBase's initial migration left `avatar`. Computed rather than written as
// 7, so a future PocketBase that adds a system field ahead of it does not
// silently move the restore one slot.
func stockAvatarPosition(users *core.Collection) int {
	for i, field := range users.Fields {
		if _, autodate := field.(*core.AutodateField); autodate {
			return i
		}
	}

	return len(users.Fields)
}

// setUsersPasswordMin writes FR-004's floor onto the system password field.
// PocketBase creates that field, so this is a narrowing of something that
// exists and its absence is a PocketBase change, not a MediKube one.
func setUsersPasswordMin(users *core.Collection, minimum int) error {
	field := users.Fields.GetByName(core.FieldNamePassword)
	if field == nil {
		return fmt.Errorf("%s has no %s field", usersCollection, core.FieldNamePassword)
	}

	password, ok := field.(*core.PasswordField)
	if !ok {
		return fmt.Errorf("%s.%s is a %s field", usersCollection, core.FieldNamePassword, field.Type())
	}

	password.Min = minimum

	return nil
}
