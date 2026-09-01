// Package viewstest renders a templ component to a buffer and lets a test ask
// structural questions of the result.
//
// Constitution Principle VIII makes render-to-buffer the fast layer of the UI
// gate, and a test that only greps the output cannot see the two things the
// contracts are actually about: whether the empty state is INSIDE the region
// rather than instead of it, and whether a field's error message is ADJACENT
// to the field it concerns. Both are relationships between elements, so the
// assertions are made against a parse tree rather than against a string.
package viewstest

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Component is templ.Component's method set, declared here for the same reason
// internal/records and internal/web declare it: a test helper does not need the
// rendering library in its import graph to hold one of its products.
type Component interface {
	Render(ctx context.Context, w io.Writer) error
}

// Tree is one rendered component, both as the markup a person would read in a
// diff and as the nodes an assertion walks.
type Tree struct {
	Markup string

	roots []*html.Node
}

// Render renders the component and parses it in the element it will really sit
// inside.
//
// The context tag is not decoration. html.Parse builds a whole document, and
// the tokeniser drops a <tr> that is not inside a table — so a row asserted
// through html.Parse is asserted against an empty document, and every assertion
// about it passes or fails for the wrong reason.
func Render(t testing.TB, component Component, contextTag string) Tree {
	t.Helper()

	var buffer bytes.Buffer
	require.NoError(t, component.Render(context.Background(), &buffer))

	markup := buffer.String()

	roots, err := html.ParseFragment(strings.NewReader(markup), &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Lookup([]byte(contextTag)),
		Data:     contextTag,
	})
	require.NoErrorf(t, err, "the component rendered markup that does not parse:\n%s", markup)

	return Tree{Markup: markup, roots: roots}
}

// Matcher selects elements.
type Matcher func(*html.Node) bool

func Tag(name string) Matcher {
	return func(node *html.Node) bool { return node.Data == name }
}

func WithID(id string) Matcher {
	return func(node *html.Node) bool { return Attr(node, "id") == id }
}

func WithAttr(name, value string) Matcher {
	return func(node *html.Node) bool { return Attr(node, name) == value }
}

func HasAttr(name string) Matcher {
	return func(node *html.Node) bool {
		for _, attribute := range node.Attr {
			if attribute.Key == name {
				return true
			}
		}
		return false
	}
}

func And(matchers ...Matcher) Matcher {
	return func(node *html.Node) bool {
		for _, match := range matchers {
			if !match(node) {
				return false
			}
		}
		return true
	}
}

// Region is contracts/pages.md's region[name="X"], spelled as that document
// spells it: a <section aria-label="X">. Article and Form below are the other
// two landmark selectors, and a Playwright getByRole resolves all three the
// same way.
func Region(name string) Matcher  { return And(Tag("section"), WithAttr("aria-label", name)) }
func Article(name string) Matcher { return And(Tag("article"), WithAttr("aria-label", name)) }
func Form(name string) Matcher    { return And(Tag("form"), WithAttr("aria-label", name)) }

// All returns every matching element in document order.
func (t Tree) All(match Matcher) []*html.Node {
	found := make([]*html.Node, 0, len(t.roots))
	for _, root := range t.roots {
		found = append(found, Find(root, match)...)
	}
	return found
}

// One requires exactly one match, because "the landmark is present" and "the
// landmark is present twice" are different bugs and only one of them is caught
// by asking for the first.
func (t Tree) One(tb testing.TB, match Matcher) *html.Node {
	tb.Helper()

	found := t.All(match)
	require.Lenf(tb, found, 1, "expected exactly one match in:\n%s", t.Markup)

	return found[0]
}

func (t Tree) Count(match Matcher) int { return len(t.All(match)) }

// Find walks a subtree, the node itself included.
func Find(node *html.Node, match Matcher) []*html.Node {
	found := make([]*html.Node, 0, 4)

	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && match(current) {
			found = append(found, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)

	return found
}

func Attr(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

// Text is every text node under this element, with the whitespace collapsed so
// an assertion does not depend on where the generator put its newlines.
func Text(node *html.Node) string {
	var builder strings.Builder

	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)

	return strings.Join(strings.Fields(builder.String()), " ")
}

// NextElement is the following sibling element, skipping the whitespace between
// them. It is what "adjacent to its field" means (FR-048).
func NextElement(node *html.Node) *html.Node {
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == html.ElementNode {
			return sibling
		}
	}
	return nil
}

// Elements are the element children, in order.
func Elements(node *html.Node) []*html.Node {
	var children []*html.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			children = append(children, child)
		}
	}
	return children
}

// Descends reports whether the node is inside the ancestor. FR-029's empty
// state is "inside the region, never instead of it", which is this question and
// not a substring one.
func Descends(ancestor, node *html.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current == ancestor {
			return true
		}
	}
	return false
}
