package page_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// T033 (US2-1, US2-2; Edge Cases: region). The three pages a browser reaches
// before it has a session — sign-in, sign-up and the recovery entry point —
// follow D-04's Accept-Language match exactly as a signed-in page does.
func TestAnonymousPagesFollowAcceptLanguage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path         string
		polishTitle  string
		englishTitle string
	}{
		{"/login", "Zaloguj się", "Sign in"},
		{"/register", "Utwórz konto", "Create account"},
		{"/forgot-password", "Zresetuj hasło", "Reset password"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			t.Run("Accept-Language: pl renders Polish", func(t *testing.T) {
				t.Parallel()

				rig := newBrowser(t).anonymous()
				_, _, body := rig.getWithHeader(tc.path, "Accept-Language", "pl")

				assert.Contains(t, body, `lang="pl"`)
				assert.Contains(t, body, "<title>"+tc.polishTitle)
			})

			t.Run("Accept-Language: de falls back to English", func(t *testing.T) {
				t.Parallel()

				rig := newBrowser(t).anonymous()
				_, _, body := rig.getWithHeader(tc.path, "Accept-Language", "de")

				assert.Contains(t, body, `lang="en"`)
				assert.Contains(t, body, "<title>"+tc.englishTitle)
			})

			t.Run("Accept-Language: pl-PL;q=0.9,en;q=0.8 renders Polish", func(t *testing.T) {
				t.Parallel()

				rig := newBrowser(t).anonymous()
				_, _, body := rig.getWithHeader(tc.path, "Accept-Language", "pl-PL;q=0.9,en;q=0.8")

				assert.Contains(t, body, `lang="pl"`)
				assert.Contains(t, body, "<title>"+tc.polishTitle)
			})
		})
	}
}
