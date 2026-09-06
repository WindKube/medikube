package api_test

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/obs"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/apitest"
)

// T226, FR-033 and SC-006.
//
// The byte comparison itself is not new: testsupport.RunOwnershipMatrix already
// compares a stranger's refusal with a genuine miss body-for-body and
// header-for-header, and records_authz_test.go runs it over the six record
// operations. What was missing is the second clause — that the two are emitted
// by the SAME response constructor, one call site reached by both, so they
// cannot drift apart under a later edit. Two responses that happen to agree
// today agree only until somebody writes a bespoke 404 in a handler.
//
// That clause is asserted twice over, because neither half is sufficient on its
// own:
//
//   - at run time, by watching the request from inside the chain: the handler
//     writes nothing at all, the error it returns is recorded as THE occurrence
//     for the request, and the bytes on the wire are exactly what
//     web.NewEnvelope produces from that recorded error. A handler that answered
//     for itself would be caught whether it returned the error afterwards or
//     not.
//   - at source level, by counting the construction sites of the envelope in
//     the whole HTTP edge. One is the claim; anything else is two things that
//     have to be kept in step by review.
//
// There is NO wall-clock assertion anywhere in this file. Latency is reported
// by the non-gating benchmark T202a: a tolerance nothing defines is exactly the
// flaky gate Constitution VIII forbids (ANALYSIS N13).

// notFoundSurface is one way this application can answer "not found" on the
// record surface. Every one of them must be indistinguishable from every other,
// because an attacker chooses which to send and learns from the difference.
type notFoundSurface struct {
	name string

	// refusal marks a surface that exists and was withheld, as opposed to one
	// that genuinely is not there. The comparison is only worth anything when
	// both families are in it: a set of six genuine misses agreeing with each
	// other proves nothing about a refusal.
	refusal bool

	send func(owner, stranger *caller, version string) response
}

func notFoundSurfaces() []notFoundSurface {
	real := recordURL(testsupport.NameOnlyMedicationID)
	gone := recordURL(missingID)

	return []notFoundSurface{
		{
			name: "a stranger reads a record that exists", refusal: true,
			send: func(_, stranger *caller, _ string) response { return stranger.get(real) },
		},
		{
			name: "the owner reads an id that never existed",
			send: func(owner, _ *caller, _ string) response { return owner.get(gone) },
		},
		{
			name: "a stranger changes a record that exists", refusal: true,
			send: func(_, stranger *caller, version string) response {
				return stranger.patch(real, `{"dosage":"1 g"}`, version)
			},
		},
		{
			name: "the owner changes an id that never existed",
			send: func(owner, _ *caller, version string) response {
				return owner.patch(gone, `{"dosage":"1 g"}`, version)
			},
		},
		{
			name: "a stranger deletes a record that exists", refusal: true,
			send: func(_, stranger *caller, version string) response { return stranger.delete(real, version) },
		},
		{
			name: "the owner deletes an id that never existed",
			send: func(owner, _ *caller, version string) response { return owner.delete(gone, version) },
		},
		{
			// A kind this instance does not serve. It shares no code with the
			// two above — records.Handler.Dispatch answers it before any
			// repository is reached — and it is the answer that tells a caller
			// which kinds are registered if it differs by so much as a header.
			name: "a kind this instance does not serve",
			send: func(owner, _ *caller, _ string) response { return owner.get(unregisteredKindURL + "/" + missingID) },
		},
		{
			name: "a list of a kind this instance does not serve",
			send: func(owner, _ *caller, _ string) response { return owner.get(unregisteredKindURL) },
		},
		{
			// The mux's own miss: no route matches, so no MediKube handler
			// runs at all and the answer is produced by PocketBase's catch-all
			// (tools/router/router.go:68).
			name: "a path that matches no route",
			send: func(owner, _ *caller, _ string) response { return owner.get(recordURL(missingID) + "/x/y") },
		},
		{
			// The lockdown's 404, which originates in PocketBase's own
			// NewNotFoundError inside a middleware no handler can see. It is
			// the same collection the operations above serve, so a difference
			// here tells a caller that MediKube stores its medications in a
			// PocketBase collection of that name.
			name: "the locked PocketBase record API for the same collection", refusal: true,
			send: func(owner, _ *caller, _ string) response {
				return owner.get("/api/collections/" + kind.Medication.Collection() + "/records")
			},
		},
		{
			name: "the locked PocketBase record API for the accounts collection", refusal: true,
			send: func(owner, _ *caller, _ string) response {
				return owner.get("/api/collections/users/records/" + testsupport.AccountAID)
			},
		},
	}
}

// unregisteredKindURL addresses a kind segment nothing registers. It is spelled
// as a path rather than built from internal/domain/kind on purpose: every value
// that package publishes IS registered, so there is nothing there to name.
const unregisteredKindURL = "/api/v1/records/nosuchkind"

// TestEveryNotFoundOnTheRecordSurfaceIsTheSameResponse is the byte comparison
// over the whole family rather than over one pair.
//
// The ownership matrix compares a refusal with the genuine miss for the same
// operation. This compares every not-found the record surface can produce with
// every other one — including the two that no MediKube handler produces at all,
// the mux's catch-all and the lockdown's middleware — because a caller picks
// which request to send and learns from any difference between the answers.
func TestEveryNotFoundOnTheRecordSurfaceIsTheSameResponse(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)

	// The version the owner is holding, sent on both write legs. Without it
	// every write is 422 on the missing precondition — for the stranger too —
	// and this file would be comparing two identical precondition failures.
	current := owner.get(recordURL(testsupport.NameOnlyMedicationID))
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	version := current.etag(t)

	surfaces := notFoundSurfaces()

	var reference *response

	var refusals, misses int

	for _, surface := range surfaces {
		answer := surface.send(owner, stranger, version)

		t.Run(surface.name, func(t *testing.T) {
			require.Equal(t, http.StatusNotFound, answer.Status, answer.Body)

			assert.NotContains(t, answer.Body, testsupport.NameOnlyMedicationID)
			assert.NotContains(t, answer.Body, testsupport.AccountAID)

			if reference == nil {
				return
			}

			assert.Equal(t, withoutCorrelationID(reference.Body), withoutCorrelationID(answer.Body),
				"this answer is distinguishable from the others by its body, so which of them was sent is recoverable")
			assert.Equal(t, comparableHeaders(*reference), comparableHeaders(answer),
				"this answer is distinguishable from the others by its headers alone")
		})

		if surface.refusal {
			refusals++
		} else {
			misses++
		}

		if reference == nil {
			kept := answer
			reference = &kept
		}
	}

	// The guard on the guard, and it is two-sided on purpose. A comparison set
	// made entirely of genuine misses agrees with itself trivially; so does one
	// made entirely of refusals. Only a set holding both says anything.
	require.Greater(t, refusals, 4, "the family has stopped holding refusals, so it compares misses with misses")
	require.Greater(t, misses, 4, "the family has stopped holding genuine misses, so it compares refusals with refusals")
	require.Greater(t, len(surfaces), 9, "the family has stopped covering the record surface")
}

// comparableHeaders is one answer's headers with the one member FR-033 permits
// two otherwise identical refusals to differ in removed by name.
func comparableHeaders(answer response) map[string][]string {
	headers := make(map[string][]string, len(answer.Header))

	for name, values := range answer.Header {
		if http.CanonicalHeaderKey(name) == http.CanonicalHeaderKey(obs.CorrelationHeader) {
			continue
		}

		headers[name] = values
	}

	return headers
}

// watched is what one request looked like from inside the chain.
type watched struct {
	// handlerWrote is whether anything inside the router had already written
	// the response by the time the innermost observer saw the handler return.
	// A handler that answered a refusal for itself sets this, whether or not it
	// went on to return the error as well.
	handlerWrote bool

	// handlerErr is what the handler returned, before the error middleware
	// touched it.
	handlerErr error

	// fault is the occurrence obs.Report recorded for the request. The error
	// middleware records it at the top of its own body, so a non-nil fault is
	// proof the middleware — and nothing else — is what answered.
	fault error
}

// observedInstance is an assembled MediKube with two observers bound around the
// error middleware.
//
// Both are bound on the root router, which is where every middleware in this
// application is bound (pb.BindServe documents why a group is wrong), and
// neither changes a byte: each calls e.Next() and returns what it got. The
// order is by priority, exactly as PocketBase orders every other middleware
// (tools/hook/hook.go:98-101), so the two brackets are unambiguous:
//
//	obs.RequestLogger      -1050   outermost
//	  outer observer       -1032   sees the recorded occurrence
//	    web.Errors         -1031   THE constructor
//	      … route middlewares, apis.RequireAuth …
//	        inner observer  1<<30  sees the handler's own return
//	          the handler
type observedInstance struct {
	caller *caller
	last   *watched
}

func newObservedInstance(t *testing.T, email string) *observedInstance {
	t.Helper()

	instance := apitest.New(t)

	last := new(watched)

	instance.App.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id: "medikubeTestOuterObserver",
			// Outside the error middleware, so obs.Report has already run.
			Priority: web.ErrorsMiddlewarePriority - 1,
			Func: func(e *core.RequestEvent) error {
				err := e.Next()
				last.fault = obs.Fault(e)

				return err
			},
		})

		se.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id: "medikubeTestInnerObserver",
			// Innermost: a priority above every middleware PocketBase or
			// MediKube binds, so nothing but the handler itself runs inside
			// it.
			Priority: 1 << 30,
			Func: func(e *core.RequestEvent) error {
				err := e.Next()
				last.handlerWrote = e.Written()
				last.handlerErr = err

				return err
			},
		})

		return se.Next()
	})

	handler := testsupport.NewEdgeHandler(t, instance.App)

	return &observedInstance{
		caller: &caller{
			t:       t,
			app:     instance.App,
			handler: handler,
			token:   testsupport.UserToken(t, instance.App, email),
		},
		last: last,
	}
}

// TestBothTheRefusalAndTheGenuineMissAreBuiltByTheOneConstructor is T226's
// second clause, observed rather than argued.
//
// For each surface: nothing inside the router wrote the response, the handler
// returned an error, that error is the occurrence recorded for the request, and
// the bytes the client received are exactly what web.NewEnvelope produces from
// it. All four together say the answer was constructed once, by the error
// middleware, from the error the handler returned — which is what "one notFound
// call site reached by both" means as an assertion.
//
// The last of the four is the load-bearing one. A handler that wrote its own
// 404 and returned nil records no occurrence; one that wrote its own 404 and
// returned the error afterwards leaves the middleware unable to write, so the
// bytes on the wire are the handler's and not the constructor's. Both are
// caught, and both are what "drift apart under a later edit" looks like.
func TestBothTheRefusalAndTheGenuineMissAreBuiltByTheOneConstructor(t *testing.T) {
	t.Parallel()

	// Signed in as the stranger, so the refusal leg is a real refusal: the
	// records below belong to account A.
	observed := newObservedInstance(t, testsupport.AccountBEmail)

	// The version the owner is holding, read from the database rather than
	// from a response: the stranger cannot ask for it, and a write sent
	// without one is 422 on the precondition for everybody — which would
	// satisfy every assertion below while saying nothing about ownership.
	// web.ETag is what the handler puts in the header, so the precondition is
	// spelled by the same function rather than by a second opinion about
	// quoting.
	version := web.ETag(storedVersion(t, observed.caller, testsupport.NameOnlyMedicationID))

	cases := []struct {
		name    string
		method  string
		url     string
		body    string
		refusal bool

		// precondition sends the owner's own If-Match, so the refusal is about
		// who is asking rather than about a header nobody supplied.
		precondition bool
	}{
		{name: "a stranger reads a record that exists", method: http.MethodGet, refusal: true,
			url: recordURL(testsupport.NameOnlyMedicationID)},
		{name: "an id that never existed", method: http.MethodGet,
			url: recordURL(missingID)},
		{name: "a stranger changes a record that exists", method: http.MethodPatch, refusal: true,
			url: recordURL(testsupport.NameOnlyMedicationID), body: `{"dosage":"1 g"}`, precondition: true},
		{name: "a stranger deletes a record that exists", method: http.MethodDelete, refusal: true,
			url: recordURL(testsupport.NameOnlyMedicationID), precondition: true},
		{name: "a kind this instance does not serve", method: http.MethodGet,
			url: unregisteredKindURL + "/" + missingID},
	}

	var refusals, misses int

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			*observed.last = watched{}

			headers := map[string]string(nil)
			if one.precondition {
				headers = ifMatch(version)
			}

			answer := observed.caller.do(one.method, one.url, one.body, headers)
			seen := *observed.last

			require.Equal(t, http.StatusNotFound, answer.Status, answer.Body)

			assert.False(t, seen.handlerWrote,
				"something inside the router answered this for itself, so its bytes are not the shared constructor's")
			require.Error(t, seen.handlerErr,
				"the handler answered without returning an error, so the error middleware never saw this request")
			require.Error(t, seen.fault,
				"no occurrence was recorded for this request, so the error middleware did not construct its answer (FR-057)")
			assert.Equal(t, seen.handlerErr, seen.fault,
				"the error that reached the constructor is not the one the handler returned")

			assert.True(t, errors.Is(seen.fault, domain.ErrNotFound),
				"the refusal reached the one constructor as something other than a miss, so it is one edit away from a 403")

			status, code := web.Classify(seen.fault)
			assert.Equal(t, http.StatusNotFound, status)
			assert.Equal(t, web.CodeNotFound, code)

			// The whole claim, in one comparison: the bytes the client got are
			// the bytes the shared constructor makes from the recorded error.
			assert.Equal(t, constructed(t, seen.fault, answer.Header.Get(obs.CorrelationHeader)), answer.Body,
				"the answer on the wire is not what web.NewEnvelope builds from the error that was recorded for it")
		})

		if one.refusal {
			refusals++
		} else {
			misses++
		}
	}

	require.Greater(t, refusals, 2, "no refusal is observed here, so the file watches genuine misses only")
	require.Greater(t, misses, 1, "no genuine miss is observed here, so there is nothing for a refusal to be identical to")
}

// constructed is the response body the one constructor produces for err,
// marshalled exactly as web.WriteJSON marshals it.
func constructed(t *testing.T, err error, requestID string) string {
	t.Helper()

	raw, marshalErr := json.Marshal(web.NewEnvelope(context.Background(), err, requestID),
		json.Deterministic(true), jsontext.AllowInvalidUTF8(true))
	require.NoError(t, marshalErr)

	return string(raw)
}

// envelopeConstructors are the functions that build an error response body.
// Reaching any of them is constructing an answer, so a second call site is a
// second answer that has to be kept in step with the first by review.
//
// PocketBase's own two are here as well as MediKube's: router.NewNotFoundError
// and a request event's NotFoundError method both produce a 404 directly, and
// one of those in a handler is exactly the bespoke refusal this file exists to
// make impossible.
var envelopeConstructors = map[string]string{
	"NewFailure":       "builds the inside of the envelope",
	"NewEnvelope":      "builds the whole body",
	"WriteError":       "writes the envelope to the response",
	"NewNotFoundError": "PocketBase's own 404 constructor",
	"NotFoundError":    "a request event's own 404 shortcut",
}

// envelopeConstructionExempt is the one door out, keyed by the exact call site
// — path, and the function it sits in — with the reason and the task that owns
// it. A path prefix is refused as a key on purpose: internal/store/filter_test.go:697
// documents a real bug where a prefix exemption silently covered four more
// packages than anybody meant.
var envelopeConstructionExempt = map[string]string{
	"internal/web/errors.go#NewEnvelope": "T226: NewEnvelope IS the constructor; this is its own body calling NewFailure",
	"internal/web/errors.go#Errors": "T226: THE call site. Every non-2xx in the application is built here, which is " +
		"the property this test exists to keep true",
	"internal/web/etag.go#NewVersionMismatch": "T226: contracts/README.md's 412 is the one response that adds a member " +
		"beside the envelope — the current representation — so it builds a Failure and wraps it. It is a 412 and never " +
		"a 404, so it cannot be a second answer to `does this record exist`",
}

// TestTheErrorEnvelopeIsConstructedInExactlyOnePlace is the source half of
// T226's second clause.
//
// The run-time half above proves that today's refusal and today's miss are both
// built by the error middleware. It cannot prove that tomorrow's are: a handler
// that grew its own web.WriteError call would satisfy every assertion in this
// file that looks at a response, right up until the two bodies drifted. This is
// the assertion that a second constructor cannot appear at all.
//
// Generated files are walked like every other file, deliberately. A .templ
// source can hold an arbitrary Go expression, so a *_templ.go is as capable of
// calling a constructor as anything else (internal/store/filter_test.go:715-722
// documents the same rule for the filter DSL); they get a counter of their own
// so that widening the walk cannot silently disable half of it.
func TestTheErrorEnvelopeIsConstructedInExactlyOnePlace(t *testing.T) {
	t.Parallel()

	root := repoRootOf(t)

	var handwritten, generated, calls int

	hit := make(map[string]bool, len(envelopeConstructionExempt))

	var offenders []string

	// The HTTP edge and the generic record handler: everything between a
	// request arriving and an answer being written. internal/service and
	// internal/domain are excluded because they return errors and cannot
	// import a response type at all — the [PB] boundary and the depguard rule
	// on internal/domain are what enforce that, not this walk.
	for _, tree := range []string{"internal/web", "internal/records"} {
		walkPackageFiles(t, root, tree, func(rel string, file *ast.File) {
			if strings.HasSuffix(rel, "_templ.go") {
				generated++
			} else {
				handwritten++
			}

			ast.Inspect(file, func(node ast.Node) bool {
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}

				calls++

				name := calleeName(call)
				if _, constructs := envelopeConstructors[name]; !constructs {
					return true
				}

				site := rel + "#" + enclosingFunc(file, call.Pos())

				if _, exempt := envelopeConstructionExempt[site]; exempt {
					hit[site] = true

					return true
				}

				offenders = append(offenders, site+" calls "+name)

				return true
			})
		})
	}

	sort.Strings(offenders)

	assert.Empty(t, offenders,
		"the error envelope is built somewhere other than the one constructor, so a refusal and a genuine miss "+
			"are now two responses that have to be kept byte-identical by review (T226, FR-033)")

	for site, reason := range envelopeConstructionExempt {
		assert.Truef(t, hit[site],
			"%s is exempt (%s) and no longer constructs anything: strike it out of envelopeConstructionExempt", site, reason)
	}

	// The guard on the guard. The two file counters are separate so that a
	// walk which started skipping handwritten code could not be covered by the
	// generated files it still found, and the call counter is what fails when
	// the walk parses files and stops descending into them.
	require.Greater(t, handwritten, 50, "the walk has stopped finding handwritten files in the HTTP edge")
	require.Greater(t, generated, 15, "the walk has stopped finding generated views, where a .templ can call anything")
	require.Greater(t, calls, 2000, "the walk parses files and inspects almost no calls in them")
	require.GreaterOrEqual(t, len(envelopeConstructionExempt), 3,
		"the exemption table is what makes this walk narrow enough to be true; an empty one means it is checking nothing")
}

// calleeName is the function a call names, ignoring how it was qualified.
//
// A name rather than a resolved symbol, and the limit is stated rather than
// hidden: a method called NotFoundError on some unrelated type would be flagged
// and would need an exemption saying so. That is the direction to be wrong in —
// the alternative reads `e.NotFoundError()` and `web.WriteError()` as different
// things depending on an import alias, and internal/logging/singlestream_test.go:280
// exists because that is a real hazard.
func calleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}

	return ""
}

// enclosingFunc names the function a position sits in, so an exemption is
// anchored to a call site rather than to a whole file. A file-level key would
// licence every future call in that file too.
func enclosingFunc(file *ast.File, pos token.Pos) string {
	name := "<file>"

	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}

		if pos >= fn.Pos() && pos <= fn.End() {
			name = fn.Name.Name
		}
	}

	return name
}

// walkPackageFiles parses every non-test .go file under one tree.
//
// It is deliberately narrower than internal/store/filter_test.go's walkGoFiles
// and internal/architecture/forbidden_deps_test.go's walkRepo, which walk the
// whole repository and are duplicates of each other. This walks two named trees
// and parses as it goes, so it is a different thing rather than a third copy of
// the same one — but the duplication between those two is real and still worth
// somebody's attention.
func walkPackageFiles(t *testing.T, root, tree string, visit func(rel string, file *ast.File)) {
	t.Helper()

	base := filepath.Join(root, filepath.FromSlash(tree))

	fset := token.NewFileSet()

	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			if path != base && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}

			return nil
		}

		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		visit(filepath.ToSlash(rel), parsed)

		return nil
	})
	require.NoError(t, err)
}

// repoRootOf finds the module root by walking up to go.mod, the same way
// internal/store/filter_test.go:977 does.
func repoRootOf(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "walked to the filesystem root without finding go.mod")
		dir = parent
	}
}
