package shell_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"

	"medikube/internal/domain/identity"
	"medikube/internal/web/views/shell"
	"medikube/internal/web/views/viewstest"
)

// T251, FR-045, research D-36. The theme class is on <html>, server-rendered
// from the stored preference, with no inline script anywhere: the CSP bans
// one and data-persist is Datastar Pro.
func TestTheThemeClassIsResolvedFromThePreference(t *testing.T) {
	t.Parallel()

	cases := []struct {
		theme identity.Theme
		class string
	}{
		{identity.ThemeDark, "dark"},
		{identity.ThemeLight, "light"},
		{identity.ThemeSystem, ""},
	}

	for _, testCase := range cases {
		t.Run(string(testCase.theme), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.class, shell.ThemeClass(testCase.theme))
		})
	}
}

// An unrecognised value — which never happens through the validated preference
// but could through a stored value an earlier vocabulary allowed — resolves the
// same as "system": no class, following the device, rather than a fourth state
// nothing renders correctly.
func TestAnUnrecognisedThemeFollowsTheDevice(t *testing.T) {
	t.Parallel()

	assert.Empty(t, shell.ThemeClass(identity.Theme("auto")))
}

// plainFragment is a minimal templ.Component: a fixed byte string, nothing
// more, for a Main that only needs to exist.
type plainFragment string

func (f plainFragment) Render(_ context.Context, w io.Writer) error {
	_, err := w.Write([]byte(f))
	return err
}

// The class actually lands on <html>, and never as an inline script anywhere
// in the document — the render gate's CSP assertion depends on there being
// none to violate it.
func TestTheClassIsWrittenOnHTMLWithNoInlineScript(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"dark class":  "dark",
		"light class": "light",
		"no class":    "",
	}

	for name, class := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer
			require.NoError(t, shell.Document(shell.DocumentProps{
				Title:      "Settings",
				SignedIn:   true,
				ThemeClass: class,
				Main:       plainFragment(`<section aria-label="Settings">content</section>`),
			}).Render(context.Background(), &buffer))

			markup := buffer.String()

			root, err := xhtml.Parse(strings.NewReader(markup))
			require.NoError(t, err)

			htmlElement := viewstest.Find(root, viewstest.Tag("html"))
			require.Len(t, htmlElement, 1, "no <html> element in the rendered document")
			assert.Equal(t, class, viewstest.Attr(htmlElement[0], "class"))

			assert.NotContains(t, markup, "<script>",
				"an inline <script> is refused by the CSP; the theme must never need one")
			assert.NotContains(t, strings.ToLower(markup), "onload=",
				"an inline event-handler attribute is refused by the CSP the same way an inline <script> is")
		})
	}
}
