// Package views holds the page shell every MediKube page is rendered into.
//
// Components receive everything they render as arguments and rely on templ's
// escaping: with 'unsafe-eval' in the Content Security Policy, that escaping is
// load-bearing and raw HTML injection is not available to work around.
package views
