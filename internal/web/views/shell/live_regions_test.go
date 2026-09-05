package shell_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"

	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/shell"
	"medikube/internal/web/views/viewstest"
)

// T258, FR-047. #error-banner and #toast are the two general-purpose patch
// targets contracts/pages.md fixes, rendered empty on every page — an element
// that does not exist cannot be patched — and with the roles and live-region
// politeness FR-047's feedback depends on: an alert interrupts, a status does
// not.
func TestTheTwoLiveRegionsCarryTheirRolesOnEveryPage(t *testing.T) {
	t.Parallel()

	var buffer strings.Builder
	require.NoError(t, shell.Document(shell.DocumentProps{
		Title:    "Medications",
		SignedIn: true,
		Main:     plainFragment(`<section aria-label="Medications">content</section>`),
	}).Render(context.Background(), &buffer))

	root, err := xhtml.Parse(strings.NewReader(buffer.String()))
	require.NoError(t, err)

	errorBanner := viewstest.Find(root, viewstest.WithID(ids.ErrorBanner))
	require.Len(t, errorBanner, 1)
	assert.Equal(t, "alert", viewstest.Attr(errorBanner[0], "role"))
	assert.Equal(t, "assertive", viewstest.Attr(errorBanner[0], "aria-live"))

	toast := viewstest.Find(root, viewstest.WithID(ids.Toast))
	require.Len(t, toast, 1)
	assert.Equal(t, "status", viewstest.Attr(toast[0], "role"))
	assert.Equal(t, "polite", viewstest.Attr(toast[0], "aria-live"))
}
