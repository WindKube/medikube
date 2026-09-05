package shell_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"

	"medikube/internal/web/views/shell"
	"medikube/internal/web/views/viewstest"
)

// T252, FR-049. Every page carries a <noscript> block INSIDE main, stating
// plainly that MediKube needs scripting — so a visitor without it sees an
// explanation rather than a blank rectangle.
func TestEveryPageCarriesANoscriptBlockInsideMain(t *testing.T) {
	t.Parallel()

	var buffer strings.Builder
	require.NoError(t, shell.Document(shell.DocumentProps{
		Title:    "Medications",
		SignedIn: true,
		Main:     plainFragment(`<section aria-label="Medications">content</section>`),
	}).Render(context.Background(), &buffer))

	root, err := xhtml.Parse(strings.NewReader(buffer.String()))
	require.NoError(t, err)

	main := viewstest.Find(root, viewstest.WithAttr("role", "main"))
	require.Len(t, main, 1)

	noscript := viewstest.Find(root, viewstest.Tag("noscript"))
	require.Len(t, noscript, 1, "the document must carry exactly one <noscript> block")

	assert.True(t, viewstest.Descends(main[0], noscript[0]), "the <noscript> block must be inside main, not beside it")
	assert.Contains(t, strings.ToLower(viewstest.Text(noscript[0])), "javascript",
		"the block must say plainly that MediKube needs scripting")
}
