package httproute_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"

	"medikube/internal/httproute"
)

// T095. Two registrations of one method and path is the failure mode with no
// symptom: http.ServeMux would panic deep inside BuildMux with a message
// naming neither route, and a registry that merely kept the last one would
// serve one handler and describe the other. Both are silent to every gate
// downstream, so the refusal happens here, at registration.

func duplicatePair() (httproute.Route, httproute.Route) {
	first := httproute.Route{
		OpID:    "firstOperation",
		Method:  http.MethodGet,
		Path:    "/api/v1/duplicate",
		Kind:    httproute.KindAPI,
		Auth:    httproute.AuthPublic,
		Summary: "the route registered first",
	}

	second := first
	second.OpID = "secondOperation"
	second.Summary = "a different operation on the same method and path"

	return first, second
}

func TestRegisteringTheSameMethodAndPathTwicePanics(t *testing.T) {
	t.Parallel()

	first, second := duplicatePair()

	registry := httproute.Empty()
	registry.Handle(first, func(e *core.RequestEvent) error { return nil })

	message := panicMessage(t, func() {
		registry.Handle(second, func(e *core.RequestEvent) error { return nil })
	})

	assert.Contains(t, message, "GET /api/v1/duplicate")
	assert.Contains(t, message, first.OpID, "the panic must name the route already holding the pattern")
	assert.Contains(t, message, second.OpID, "and the one that tried to take it")
}

// A documented external is not served, but it is still an identity in the
// inventory and still a path an operator reads out of `medikube routes`. Two
// rows for one pattern would be two answers to one question.
func TestDocumentingTheSameMethodAndPathTwicePanics(t *testing.T) {
	t.Parallel()

	first, second := duplicatePair()
	first.Kind, second.Kind = httproute.KindExternal, httproute.KindExternal

	registry := httproute.Empty()
	registry.Document(first)

	message := panicMessage(t, func() { registry.Document(second) })

	assert.Contains(t, message, "GET /api/v1/duplicate")
}

func TestADocumentedExternalAndAServedRouteCannotShareAPattern(t *testing.T) {
	t.Parallel()

	first, second := duplicatePair()
	second.Kind = httproute.KindExternal

	registry := httproute.Empty()
	registry.Handle(first, func(e *core.RequestEvent) error { return nil })

	message := panicMessage(t, func() { registry.Document(second) })

	assert.Contains(t, message, "GET /api/v1/duplicate")
}

// The operationId is the join key of the whole Principle IX gate: it is what
// api/openapi.json is asserted against and what a handler is looked up by. Two
// routes sharing one makes the gate pass while half the wiring is wrong.
func TestRegisteringTheSameOpIDTwicePanics(t *testing.T) {
	t.Parallel()

	first, second := duplicatePair()
	second.OpID = first.OpID
	second.Path = "/api/v1/elsewhere"

	registry := httproute.Empty()
	registry.Handle(first, func(e *core.RequestEvent) error { return nil })

	message := panicMessage(t, func() {
		registry.Handle(second, func(e *core.RequestEvent) error { return nil })
	})

	assert.Contains(t, message, first.OpID)
	assert.Contains(t, message, "twice")
}

// Same path, different method, is the ordinary REST shape — /api/v1/me is a
// GET, a PATCH and a DELETE — so the duplicate check must key on the pattern
// and not on the path alone.
func TestTheSamePathUnderDifferentMethodsIsFine(t *testing.T) {
	t.Parallel()

	first, second := duplicatePair()
	second.Method = http.MethodDelete

	registry := httproute.Empty()
	registry.Handle(first, func(e *core.RequestEvent) error { return nil })

	assert.NotPanics(t, func() {
		registry.Handle(second, func(e *core.RequestEvent) error { return nil })
	})
	assert.Len(t, registry.Routes(), 2)
}
