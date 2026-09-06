package openapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"medikube/internal/httproute"
)

const (
	documentTitle = "MediKube API"

	// The version string is load-bearing beyond documentation. kin-openapi's
	// version switch is a hardcoded list of spellings; an unrecognised one —
	// "3.1.3", say — yields an empty major/minor and the document is then
	// VALIDATED AS 3.0 with no error at all. So this is one of the recognised
	// spellings, and document_test.go asserts IsOpenAPI31OrLater rather than a
	// prefix of the string.
	openAPIVersion = "3.1.0"

	// sessionScheme is the credential every non-public operation requires.
	sessionScheme = "session"
)

// Input is everything the document is generated from.
//
// Routes is the inventory, Kinds is what the record family serves, and Envelope
// is the Go type internal/web/errors.go writes — reflected rather than
// described here, so the published error shape cannot drift from the one the
// application actually sends.
type Input struct {
	Version  string
	Routes   []httproute.Route
	Kinds    []Kind
	Envelope any

	// SearchResponse is contracts/search.md §2's grouped envelope, reflected
	// the same way Envelope is (SchemaOf) and for the same reason: this
	// package cannot import internal/web/api's own type without a cycle
	// (api already imports openapi for Kind).
	SearchResponse any
}

// Documented reports whether a route belongs in api/openapi.json.
//
// Pages and assets do not: a page has no operationId a client could call and no
// DTO, and its coverage is the browser gate's inventory. Documented externals
// do, because contracts/README.md leaves those paths reachable on purpose and a
// reachable path nobody wrote down is one somebody discovers by accident.
func Documented(route httproute.Route) bool {
	switch route.Kind {
	case httproute.KindAPI, httproute.KindStream, httproute.KindExternal:
		return true
	case httproute.KindPage, httproute.KindAsset:
		return false
	default:
		return false
	}
}

// Generate builds the document from the route inventory.
//
// It reports BOTH halves of a mismatch, because either one means the published
// interface and the served one have parted company: a registered route this
// package documents nothing about, and documentation for an operation nothing
// serves (FR-065, SC-011). Neither is visible from the other side.
func Generate(in Input) (*openapi3.T, error) {
	if in.Version == "" {
		return nil, errors.New("openapi: the document needs a version; api/openapi.json carries the build stamp")
	}

	if err := validateKinds(in.Kinds); err != nil {
		return nil, err
	}

	if in.Envelope == nil {
		return nil, errors.New(
			"openapi: the error envelope type is required; a document that describes its own would be a second " +
				"description of internal/web/errors.go and would be wrong the first time either changed")
	}

	envelope, err := SchemaOf(in.Envelope)
	if err != nil {
		return nil, fmt.Errorf("openapi: the error envelope: %w", err)
	}

	schemas, err := recordComponents(in.Kinds)
	if err != nil {
		return nil, fmt.Errorf("openapi: the record family: %w", err)
	}

	schemas[ErrorEnvelopeSchema] = &openapi3.SchemaRef{Value: envelope}

	if in.SearchResponse != nil {
		search, searchErr := SchemaOf(in.SearchResponse)
		if searchErr != nil {
			return nil, fmt.Errorf("openapi: the search response: %w", searchErr)
		}

		schemas[SearchResponseSchema] = &openapi3.SchemaRef{Value: search}
	}

	segments := make([]any, 0, len(in.Kinds))
	for _, k := range in.Kinds {
		segments = append(segments, k.Segment)
	}

	paths, err := buildPaths(in.Routes, segments)
	if err != nil {
		return nil, err
	}

	return &openapi3.T{
		OpenAPI: openAPIVersion,
		Info: &openapi3.Info{
			Title:       documentTitle,
			Version:     in.Version,
			Description: documentDescription,
		},
		Paths: paths,
		Components: &openapi3.Components{
			Schemas:         schemas,
			SecuritySchemes: securitySchemes(),
		},
	}, nil
}

// RoundTrip marshals the document, loads it back through openapi3.NewLoader and
// validates the result. It is the gate, and the two errors it can return are
// not interchangeable.
//
// A document built in memory holds SchemaRefs that carry a Ref and no Value, so
// validating it IN PLACE reports an unresolved ref for a document that is
// correct. The dangerous case is the mirror image: a ref carrying both a Ref and
// a Value whose target is missing from Components passes an in-place Validate —
// it walks the Value — and fails only at LOAD, because marshalling writes just
// the $ref. A check that keeps the reloaded Validate and drops the loader's
// error therefore catches nothing (VERIFIED FACT 9, risk R1).
func RoundTrip(ctx context.Context, doc *openapi3.T) (*openapi3.T, []byte, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal the document: %w", err)
	}

	// IsExternalRefsAllowed stays false, so no $ref reaches the network or the
	// filesystem. The loader's own documentation carries the SSRF warning for
	// anyone who changes that.
	loader := openapi3.NewLoader()

	loaded, err := loader.LoadFromData(raw)
	if err != nil {
		return nil, raw, fmt.Errorf("load the document back: %w", err)
	}

	// EnableMultiError so one run reports every structural fault rather than
	// the first. Note that operationId uniqueness is the exception: kin-openapi
	// returns on the first duplicate even here, which is why the gate counts
	// them itself.
	if err := loaded.Validate(ctx, openapi3.EnableMultiError()); err != nil {
		return nil, raw, fmt.Errorf("validate the reloaded document: %w", err)
	}

	return loaded, raw, nil
}

// Marshal renders the document the way api/openapi.json holds it: indented, so
// a change to one operation is a reviewable diff rather than one enormous line,
// and newline-terminated, so it is a well-formed text file.
func Marshal(doc *openapi3.T) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal the document: %w", err)
	}

	return append(raw, '\n'), nil
}

const documentDescription = "The published interface of a MediKube instance, generated from the route registry " +
	"rather than written by hand. Every non-2xx response carries the same error envelope, and every response " +
	"carries the request id that appears on the log lines for the same request."

func validateKinds(kinds []Kind) error {
	if len(kinds) == 0 {
		return errors.New("openapi: no record kind was supplied; the record family would document a union with no branches")
	}

	enums := make(map[string]struct{}, len(kinds))
	segments := make(map[string]struct{}, len(kinds))

	for _, k := range kinds {
		if k.Enum == "" || k.Segment == "" {
			return fmt.Errorf("openapi: a kind was supplied with enum %q and segment %q; both spellings are required", k.Enum, k.Segment)
		}

		if k.Summary == nil || k.Detail == nil || k.Create == nil || k.Patch == nil {
			return fmt.Errorf("openapi: kind %q does not supply all four DTOs", k.Enum)
		}

		if _, duplicate := enums[k.Enum]; duplicate {
			return fmt.Errorf("openapi: %q is the enum spelling of two kinds", k.Enum)
		}

		if _, duplicate := segments[k.Segment]; duplicate {
			return fmt.Errorf("openapi: %q is the path segment of two kinds", k.Segment)
		}

		enums[k.Enum] = struct{}{}
		segments[k.Segment] = struct{}{}
	}

	return nil
}

func buildPaths(routes []httproute.Route, segments []any) (*openapi3.Paths, error) {
	docs := operationDocs()
	paths := openapi3.NewPaths()

	var problems []error

	for _, route := range routes {
		if !Documented(route) {
			continue
		}

		doc, described := docs[route.OpID]
		if !described {
			problems = append(problems, fmt.Errorf(
				"%s (%s) is registered but this package documents no such operation", route.OpID, route.Pattern()))

			continue
		}

		delete(docs, route.OpID)

		operation, err := buildOperation(route, doc, segments)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", route.OpID, err))

			continue
		}

		path := documentPath(route.Path)

		item := paths.Value(path)
		if item == nil {
			item = &openapi3.PathItem{}
			paths.Set(path, item)
		}

		if item.GetOperation(route.Method) != nil {
			problems = append(problems, fmt.Errorf("%s: %s is documented twice", route.OpID, route.Pattern()))

			continue
		}

		item.SetOperation(route.Method, operation)
	}

	// Sorted, because ranging a map is not: an error message that reorders
	// itself between runs is one nobody can diff.
	stray := make([]string, 0, len(docs))
	for opID := range docs {
		stray = append(stray, opID)
	}

	sort.Strings(stray)

	for _, opID := range stray {
		problems = append(problems, fmt.Errorf("%s is documented here but no route is registered under it", opID))
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("generate the MediKube OpenAPI document: %w", errors.Join(problems...))
	}

	return paths, nil
}

// documentPath is the one translation between the registry's paths and the
// document's. Go's ServeMux spells a trailing wildcard `{path...}`; OpenAPI has
// no such form and would read the parameter's name as `path...`.
func documentPath(routePath string) string {
	return strings.ReplaceAll(routePath, "...}", "}")
}

func buildOperation(route httproute.Route, doc operationDoc, segments []any) (*openapi3.Operation, error) {
	parameters, err := pathParameters(route.Path, segments)
	if err != nil {
		return nil, err
	}

	for _, header := range doc.headers {
		parameters = append(parameters, parameterRef(header, openapi3.ParameterInHeader))
	}

	for _, query := range doc.query {
		parameters = append(parameters, parameterRef(query, openapi3.ParameterInQuery))
	}

	responses, err := buildResponses(doc)
	if err != nil {
		return nil, err
	}

	operation := &openapi3.Operation{
		OperationID: route.OpID,
		Summary:     route.Summary,
		Description: describe(route, doc),
		Parameters:  parameters,
		Responses:   responses,
		Security:    security(route),
	}

	if doc.requestBody != "" {
		operation.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().
			WithRequired(true).
			WithJSONSchemaRef(&openapi3.SchemaRef{Ref: componentRef + doc.requestBody})}
	}

	return operation, nil
}

// The description opens with the authorization rule, because that is the
// sentence a client author most often has to guess at and most often guesses
// wrong.
func describe(route httproute.Route, doc operationDoc) string {
	parts := []string{authorizationRule(route, doc)}

	if doc.notes != "" {
		parts = append(parts, doc.notes)
	}

	return strings.Join(parts, "\n\n")
}

func authorizationRule(route httproute.Route, doc operationDoc) string {
	switch route.Auth {
	case httproute.AuthPublic:
		return "Authorization: public. No session is required, and the response names no account."

	case httproute.AuthAdmin:
		return "Authorization: requires a PocketBase superuser session."

	case httproute.AuthUser:
		if doc.ownerScoped {
			return "Authorization: requires a session. Owner-scoped: the owner is taken from the stored record or " +
				"from the authenticated actor and never from a caller-supplied parameter, and anything the " +
				"signed-in account does not own answers 404 — byte-identical apart from `request_id` to an id " +
				"that never existed (FR-032, FR-033)."
		}

		return "Authorization: requires a session."

	default:
		return "Authorization: requires a session."
	}
}

func security(route httproute.Route) *openapi3.SecurityRequirements {
	// Explicitly empty rather than absent: an operation with no security member
	// inherits the document's, and "inherits nothing" and "requires nothing"
	// are not the same statement.
	if route.Auth == httproute.AuthPublic {
		return &openapi3.SecurityRequirements{}
	}

	return &openapi3.SecurityRequirements{openapi3.SecurityRequirement{sessionScheme: []string{}}}
}

func securitySchemes() openapi3.SecuritySchemes {
	return openapi3.SecuritySchemes{
		sessionScheme: &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
			Type:   "http",
			Scheme: "bearer",
			Description: "The session token, in the Authorization header. A browser carries the same token in an " +
				"HttpOnly, SameSite cookie that the server turns back into this header before anything else in " +
				"the stack sees the request, so both callers behave identically from here on.",
		}},
	}
}

var pathParameterDescriptions = map[string]string{
	KindPathParameter: "A registered record kind, spelled as its path segment. A value outside the enum is 404 and " +
		"not 400: an unregistered kind is indistinguishable from a path that does not exist.",
	"id":   "The record's id.",
	"path": "Everything below the prefix. PocketBase serves it.",
}

func pathParameters(path string, segments []any) (openapi3.Parameters, error) {
	var parameters openapi3.Parameters

	rest := path

	for {
		opened := strings.Index(rest, "{")
		if opened < 0 {
			return parameters, nil
		}

		closed := strings.Index(rest[opened:], "}")
		if closed < 0 {
			return nil, fmt.Errorf("the path %q has an unclosed parameter", path)
		}

		// ServeMux's trailing wildcard is {name...}; the name is what precedes
		// the dots.
		name := strings.TrimSuffix(rest[opened+1:opened+closed], "...")
		rest = rest[opened+closed+1:]

		schema := primitive("string")

		description, known := pathParameterDescriptions[name]
		if !known {
			description = "The " + name + "."
		}

		// The {kind} parameter carries the SEGMENT vocabulary. The
		// discriminator inside the body carries the enum spelling. They share a
		// name and never a value.
		if name == KindPathParameter {
			schema.Enum = segments
		}

		parameters = append(parameters, &openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name:        name,
			In:          openapi3.ParameterInPath,
			Required:    true,
			Description: description,
			Schema:      &openapi3.SchemaRef{Value: schema},
		}})
	}
}

func parameterRef(p param, in string) *openapi3.ParameterRef {
	return &openapi3.ParameterRef{Value: &openapi3.Parameter{
		Name:        p.name,
		In:          in,
		Required:    p.required,
		Description: p.description,
		Schema:      &openapi3.SchemaRef{Value: p.schema},
	}}
}

func buildResponses(doc operationDoc) (*openapi3.Responses, error) {
	responses := openapi3.NewResponses()

	success := &openapi3.Response{Description: &doc.successNote}

	switch {
	case doc.successBody != "":
		success.Content = openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Ref: componentRef + doc.successBody})

	case doc.successType != "":
		success.Content = openapi3.Content{
			doc.successType: openapi3.NewMediaType().WithSchema(primitive("string")),
		}
	}

	if len(doc.successHeaders) > 0 {
		success.Headers = openapi3.Headers{}

		for _, name := range doc.successHeaders {
			description, known := responseHeaderDescriptions[name]
			if !known {
				return nil, fmt.Errorf("the response header %q has no documented meaning", name)
			}

			// A Header object carries no name and no location of its own — the
			// map key is the name — and kin-openapi refuses one that does.
			success.Headers[name] = &openapi3.HeaderRef{Value: &openapi3.Header{Parameter: openapi3.Parameter{
				Description: description,
				Schema:      &openapi3.SchemaRef{Value: primitive("string")},
			}}}
		}
	}

	responses.Set(strconv.Itoa(doc.successStatus), &openapi3.ResponseRef{Value: success})

	for _, status := range doc.errors {
		if _, excepted := doc.nonEnvelope[status]; excepted {
			return nil, fmt.Errorf("%d is documented both as an envelope error and as an exception to it", status)
		}

		description, known := errorDescriptions[status]
		if !known {
			return nil, fmt.Errorf("the status %d has no documented meaning in contracts/README.md's table", status)
		}

		responses.Set(strconv.Itoa(status), &openapi3.ResponseRef{Value: &openapi3.Response{
			Description: &description,
			Content:     openapi3.NewContentWithJSONSchemaRef(&openapi3.SchemaRef{Ref: componentRef + ErrorEnvelopeSchema}),
		}})
	}

	excepted := make([]int, 0, len(doc.nonEnvelope))
	for status := range doc.nonEnvelope {
		excepted = append(excepted, status)
	}

	slices.Sort(excepted)

	for _, status := range excepted {
		reason := doc.nonEnvelope[status]
		responses.Set(strconv.Itoa(status), &openapi3.ResponseRef{Value: &openapi3.Response{Description: &reason}})
	}

	return responses, nil
}
