package identity

import "slices"

// Role is MediKube's application permission tier, and it is not a PocketBase
// superuser: an admin here administers accounts through MediKube's own
// endpoints, while the collection rules stay superuser-only (data-model §1).
//
// It is absent from every request DTO. FR-012 makes the tier the server's to
// set, and a person who could name their own would be an admin by asking.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// UnitSystem drives the display of every measurement. Load-bearing from phase
// 002 onward, where a value in the wrong system is a clinical error and not a
// presentation preference (FR-011).
type UnitSystem string

const (
	UnitSystemMetric   UnitSystem = "metric"
	UnitSystemImperial UnitSystem = "imperial"
)

// DateFormat governs presentation only. The stored and transported form of a
// calendar date is domain.Date's single spelling whatever this says, so a
// person reading 03/04 as April the third cannot move a dose (FR-019).
type DateFormat string

const (
	DateFormatISO DateFormat = "iso"
	DateFormatDMY DateFormat = "dmy"
	DateFormatMDY DateFormat = "mdy"
)

// Theme is stored on the account so it follows the person to another device,
// and is rendered by the server as a class on <html> at first paint — never
// read back from localStorage after the fact (FR-045, research D-36).
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// The defaults data-model §1 gives each column, named once so that the
// migration writing them into a select field, the registration that fills a new
// account and the fixtures cannot each pick their own.
const (
	DefaultRole       = RoleUser
	DefaultUnitSystem = UnitSystemMetric
	DefaultDateFormat = DateFormatISO
	DefaultTheme      = ThemeSystem

	// English is the only text this phase ships; the value governs date and
	// number presentation, not translation.
	DefaultLocale = "en"
)

// One declaration per vocabulary, in the order the form offers it. Valid() and
// the accessor read the same slice, so a value cannot be accepted without being
// offered or offered without being accepted.
var (
	roles       = []Role{RoleUser, RoleAdmin}
	unitSystems = []UnitSystem{UnitSystemMetric, UnitSystemImperial}
	dateFormats = []DateFormat{DateFormatISO, DateFormatDMY, DateFormatMDY}
	themes      = []Theme{ThemeSystem, ThemeLight, ThemeDark}
)

// Roles is the published vocabulary, in the order the form offers it. It
// clones, as the three accessors below do, because a caller that sorted the
// result for one display would otherwise reorder every form, every OpenAPI enum
// and every Valid() along with it.
func Roles() []Role             { return slices.Clone(roles) }
func UnitSystems() []UnitSystem { return slices.Clone(unitSystems) }
func DateFormats() []DateFormat { return slices.Clone(dateFormats) }
func Themes() []Theme           { return slices.Clone(themes) }

// Valid is false for the empty string on all four. Each of these columns is
// required and carries a default, so an empty value is a record built without
// one rather than a field somebody left blank, and Validate reports it as a
// value outside the vocabulary (data-model §1).
func (r Role) Valid() bool       { return slices.Contains(roles, r) }
func (u UnitSystem) Valid() bool { return slices.Contains(unitSystems, u) }
func (d DateFormat) Valid() bool { return slices.Contains(dateFormats, d) }
func (t Theme) Valid() bool      { return slices.Contains(themes, t) }
