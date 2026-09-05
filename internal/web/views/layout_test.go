package views_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"

	"medikube/internal/web/views/shell"
	"medikube/internal/web/views/viewstest"
)

// T249, FR-043. Constitution Principle VIII's fast layer: render to buffer and
// ask the parse tree the question the browser gate asks the DOM — never a
// substring match, because "banner" appears in a class name too.
func TestTheShellCarriesAllFourLandmarksInOrderOnEveryPage(t *testing.T) {
	t.Parallel()

	cases := map[string]shell.DocumentProps{
		"signed in": {
			Title:    "Medications",
			SignedIn: true,
			Nav: []shell.NavLink{
				{Label: "Medications", Href: "/records", Current: true},
				{Label: "Settings", Href: "/settings"},
			},
			Main: fragment(`<section aria-label="Records">content</section>`),
		},
		"signed out": {
			Title: "Sign in",
			Nav: []shell.NavLink{
				{Label: "Sign in", Href: "/login", Current: true},
				{Label: "Create account", Href: "/register"},
			},
			Main: fragment(`<form aria-label="Sign in">content</form>`),
		},
	}

	for name, props := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := render(t, props)

			landmarks := []struct {
				name  string
				match viewstest.Matcher
			}{
				{"banner", viewstest.WithAttr("role", "banner")},
				{"navigation", viewstest.And(viewstest.WithAttr("role", "navigation"), viewstest.WithAttr("aria-label", "Primary"))},
				{"main", viewstest.WithAttr("role", "main")},
				{"contentinfo", viewstest.WithAttr("role", "contentinfo")},
			}

			order := make([]int, 0, len(landmarks))
			for _, landmark := range landmarks {
				found := viewstest.Find(root, landmark.match)
				require.Lenf(t, found, 1, "expected exactly one %s landmark", landmark.name)
				order = append(order, docPosition(root, found[0]))
			}

			assert.IsIncreasingf(t, order, "the four landmarks are not in contracts/pages.md's order: %v", order)
		})
	}
}

// T249. The skip link is the point of FR-043: a keyboard user's first Tab
// press must reach it before anything else, which is a question about
// document order and not about presence.
func TestTheSkipLinkIsTheFirstFocusableElement(t *testing.T) {
	t.Parallel()

	root := render(t, shell.DocumentProps{
		Title:    "Medications",
		SignedIn: true,
		Nav:      []shell.NavLink{{Label: "Medications", Href: "/records", Current: true}},
		Main:     fragment(`<section aria-label="Records">content</section>`),
	})

	focusable := viewstest.Find(root, func(n *xhtml.Node) bool {
		switch n.Data {
		case "a", "button", "input", "select", "textarea":
			return true
		default:
			return false
		}
	})

	require.NotEmpty(t, focusable, "the rendered shell has no focusable element at all")

	first := focusable[0]
	require.Equal(t, "a", first.Data)
	assert.Equal(t, "#main", viewstest.Attr(first, "href"))
	assert.Contains(t, viewstest.Text(first), "Skip to content")
}

// fragment is a minimal templ.Component, the same shape render_test.go and
// errors_test.go both use for a Main that only needs to exist.
type fragment string

func (f fragment) Render(_ context.Context, w io.Writer) error {
	_, err := w.Write([]byte(f))
	return err
}

func render(t *testing.T, props shell.DocumentProps) *xhtml.Node {
	t.Helper()

	var buffer bytes.Buffer
	require.NoError(t, shell.Document(props).Render(t.Context(), &buffer))

	root, err := xhtml.Parse(strings.NewReader(buffer.String()))
	require.NoErrorf(t, err, "the shell rendered markup that does not parse:\n%s", buffer.String())

	return root
}

// docPosition is a node's index in the document's own pre-order walk, which is
// what "in order" means for landmarks that can each appear only once.
func docPosition(root *xhtml.Node, target *xhtml.Node) int {
	position := -1
	count := 0

	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n == target {
			position = count
		}
		count++
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	return position
}
