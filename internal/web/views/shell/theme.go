package shell

import "medikube/internal/domain/identity"

// ThemeClass resolves D-36's <html> class. "system" and anything unrecognised
// both answer "": Tailwind's dark variant then follows prefers-color-scheme
// instead (assets/input.css's @custom-variant).
func ThemeClass(theme identity.Theme) string {
	switch theme {
	case identity.ThemeDark:
		return "dark"
	case identity.ThemeLight:
		return "light"
	default:
		return ""
	}
}
