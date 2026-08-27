# MediKube Frontend Stack Research: templ + Datastar + Tailwind on PocketBase

**Researched 2026-08-26.** Everything below was verified against **actual source**, not blog posts:
the `datastar-go` module zip from `proxy.golang.org`, the shipped `datastar.js` v1.0.2 bundle from
jsDelivr (decompiled/grepped), the PocketBase v0.40.1 module zip, and the templ v0.3.1020 module zip.
Where the official docs are wrong or stale, that is called out explicitly.

---

## 0. Version matrix and the drift traps

| Thing | Correct value (verified) | How verified |
|---|---|---|
| Datastar **Go SDK** module path | `github.com/starfederation/datastar-go` | `proxy.golang.org/.../@latest` |
| Datastar Go SDK version | **v1.2.2** (2026-06-02) | `@latest` = v1.2.2; full tag list ends v1.2.2 |
| Datastar Go SDK import path | `github.com/starfederation/datastar-go/datastar` | package dir in module zip |
| Datastar Go SDK min Go | 1.24 | its `go.mod` |
| Datastar **JS runtime** version | **v1.0.2** | `// Datastar v1.0.2` header in bundle |
| Datastar JS repo (for the bundle) | `starfederation/datastar` | jsDelivr `gh/starfederation/datastar@1.0.2` |
| templ | **v0.3.1020** (2026-05-10) | `@latest` |
| PocketBase | **v0.40.1** (2026-08-24) | `@latest` |
| Tailwind CSS | **v4.3.3** | npm dist-tags |

### Trap 1 — the Go SDK version is NOT the JS version

`datastar-go` is **v1.2.2**. The JS runtime is **v1.0.2**. They live in different repos and are
versioned independently. Do not "align" them; do not assume a `datastar-go v1.2.2` implies a
`datastar.js v1.2.2` — **that bundle does not exist**.

There is also a *legacy* Go module at the old path:

```
github.com/starfederation/datastar/sdk/go   <- OLD, do not use
github.com/starfederation/datastar-go       <- CURRENT
```

The old monorepo path still resolves (`github.com/starfederation/datastar` is tagged v1.0.2 as a Go
module because the JS repo also contains Go code). **Pin the new path.** If you ever see
`import "github.com/starfederation/datastar/sdk/go"` in an example, that example is pre-v1.1 and its
API names are probably wrong too.

### Trap 2 — npm is a dead end for the JS bundle

`@starfederation/datastar` on npm is **abandoned at `1.0.0-beta.11`, last published 2025-03-30**.
The real distribution is GitHub tags / jsDelivr `gh/`. Do not `npm i` Datastar. (MediKube has no Node
in the runtime anyway — see §8.)

### Trap 3 — the official templ Datastar doc page is stale

`https://templ.guide/server-side-rendering/datastar/` still shows **v0.x SDK API**:

```go
// WRONG - v0.x API, from templ.guide as of 2026-08-26
datastar.NewSSE(w, r).MergeFragmentTempl(c)
datastar.NewSSE(w, r).MarshalAndMergeSignals(update)
```

The v1.2.2 equivalents are `PatchElementTempl` and `MarshalAndPatchSignals`. That page *does*
correctly use the new `data-on:click` colon syntax, so it is half-migrated. Trust nothing on it.

### Trap 4 — v0.x → v1.0 renames (the big one)

Verified against the v1.0.0 release notes **and** the shipped bundle's own parser:

| v0.x | v1.x (CURRENT) |
|---|---|
| attribute key delimiter `-` | **`:`** |
| `data-on-click` | **`data-on:click`** |
| `data-signals-foo` | **`data-signals:foo`** |
| `data-bind-foo` | **`data-bind:foo`** |
| `data-class-active` | **`data-class:active`** |
| `data-attr-disabled` | **`data-attr:disabled`** |
| `data-computed-total` | **`data-computed:total`** |
| `data-ref-input` | **`data-ref:input`** |
| `data-indicator-loading` | **`data-indicator:loading`** |
| `data-on-load` | **`data-init`** (renamed outright) |
| `data-fetch-indicator` | **`data-indicator`** |
| SSE `datastar-merge-fragments` | **`datastar-patch-elements`** |
| SSE `datastar-merge-signals` | **`datastar-patch-signals`** |
| SSE `datastar-remove-fragments` | **gone** — use `datastar-patch-elements` + `mode remove` |
| SSE `datastar-remove-signals` | **gone** — patch the signal to `null` to delete it |
| SSE `datastar-execute-script` | **gone** — SDK emits a `<script>` via `datastar-patch-elements` |
| data line `fragments` | **`elements`** |
| data line `mergeMode` | **`mode`** |
| mode `morph` | **`outer`** |
| DOM event `datastar-sse` | **`datastar-fetch`** |
| modifier `.trail` / `.notrail` | **`.trailing` / `.notrailing`** |
| modifier `__trusted` | **removed** |

Other v1.0 semantic changes that will bite:

- Setting a signal to `null`/`undefined` **deletes** it (this replaces `remove-signals`).
- `data-bind` on a file input creates one signal shaped `{name, contents, mime}[]`.
- Fetch requests are **auto-cancelled when the initiating element is removed from the DOM**. If you
  patch away the button that started a stream, the stream dies. This matters a lot for MediKube:
  put long-lived `@get` streams on a container element that never gets patched.

### Trap 5 — `data-persist` is **Datastar Pro (paid)**, not in the free bundle

You listed `data-persist` in the attribute set to enumerate. It is **not in the MIT bundle**. I
enumerated every plugin registered in `datastar.js` v1.0.2 by grepping the bundle's registration
calls. The complete free set is:

```
attr  bind  class  computed  effect  indicator  init  json-signals  on
on-intersect  on-interval  on-signal-patch  ref  show  signals  style  text
```

plus actions `peek`, `setAll`, `toggleAll`, `get`, `post`, `put`, `patch`, `delete`,
plus watchers `datastar-patch-elements`, `datastar-patch-signals`,
plus the bare markers `data-ignore`, `data-ignore-morph`, `data-preserve-attr`,
`data-on-signal-patch-filter` (handled inline, not as plugins).

**Everything else in the "Reference / Attributes" docs page is Pro and requires a commercial
licence:** `data-persist`, `data-query-string`, `data-replace-url`, `data-scroll-into-view`,
`data-view-transition`, `data-custom-validity`, `data-animate`, `data-match-media`, `data-on-raf`,
`data-on-resize`, and actions `@clipboard`, `@fit`.

**Recommendation for MediKube:** treat the free bundle as the hard boundary. Every Pro attribute has a
trivial free replacement:

| Pro attribute | Free replacement for MediKube |
|---|---|
| `data-persist` | server-side: persist in the PB `users` record or a prefs collection, hydrate via `data-signals` on page render. Better anyway — a medical app should not scatter PHI into `localStorage`. |
| `data-query-string` / `data-replace-url` | `sse.ReplaceURL(u)` from the Go SDK (server-driven, works today) |
| `data-custom-validity` | server-side validation + patched error elements (§3 round trip) |
| `data-scroll-into-view` | `sse.ExecuteScript("document.getElementById('x').scrollIntoView()")`, or a 6-line web component |
| `data-view-transition` | `WithUseViewTransitions(true)` on the patch — that is free, it is the *attribute* that is Pro |

Write this boundary into the spec as a lint rule: **no `data-` attribute outside the free list.**

---

## 1. Datastar's mental model

Three ideas, and that is genuinely all of it.

**1. Signals.** A single global reactive store, addressed by `$name` or `$nested.path`. Signals are
created declaratively in markup (`data-signals`, `data-bind`, `data-computed`, `data-ref`,
`data-indicator`) or pushed from the server (`datastar-patch-signals`). There is no component state,
no `useState`, no store file. The DOM *is* the store's declaration.

**2. Expressions.** The value of a `data-*` attribute is a Datastar expression: JS, with `$foo`
rewritten to a signal read/write before evaluation, executed in a sandboxed function. From the
bundle's own plugin definitions, the injected context variables are:

- `el` — the element carrying the attribute (all expressions)
- `evt` — the DOM event (`data-on` only; `argNames:["evt"]`)
- `patch` — the signal-patch delta (`data-on-signal-patch` only; `argNames:["patch"]`)

Multiple statements need **semicolons**; newlines are not separators.

```html
<button data-on:click="$saving = true; @post('/api/v1/medications')">Save</button>
```

Signals whose name — or any dotted path segment — starts with `_` are **client-local**: they are
excluded from every backend request. This is not folklore; it is the literal default in the
bundle's fetch action:

```js
filterSignals: { include: /.*/, exclude: /(^|\.)_/ }
```

So `$_uiDrawerOpen` never leaves the browser, while `$medication.dose` does. **Use `_` for every
piece of pure-UI state.** In a medical app that is also a privacy control.

**3. Patches.** The server sends HTML elements and/or signal JSON; Datastar morphs them in. Elements
are matched **by `id`** by default. This is the whole "backend-driven" story.

### The free attribute set, annotated

| Attribute | What it does | MediKube note |
|---|---|---|
| `data-signals:foo="1"` / `data-signals="{a:1,b:2}"` | Create/patch signals. Modifiers: `__case`, `__ifmissing` | Hydrate initial page state here, from Go structs via `templ.JSONString` |
| `data-bind:foo` / `data-bind="foo"` | Two-way bind an input's value to a signal. Modifiers: `__case`, `__prop`, `__event` | The form workhorse. Since v1.0.2 checkbox/radio default to the `input` event |
| `data-text="$foo"` | Set `textContent` | Auto-escaped by being textContent — safe for patient data |
| `data-show="$foo"` | Toggle `display:none` | Add `style="display:none"` inline to avoid FOUC on the hidden branch |
| `data-class:is-active="$x"` / `data-class="{a: $x}"` | Toggle classes | Works with Tailwind classes; see §7 for the scanner caveat |
| `data-attr:aria-busy="$saving"` / `data-attr="{...}"` | Set any attribute reactively | **This is your a11y lever** (§9) |
| `data-on:click="..."`, `data-on:submit="..."`, `data-on:input="..."` | Event listener. Modifiers: `__once __passive __capture __case __delay __debounce __throttle __viewtransition __window __document __outside __prevent __stop` | Note colon. See form auto-prevent below |
| `data-on-interval="..."` (`__duration.5s`, `.leading`) | Poll on a timer | Hyphenated — it is a *separate plugin*, not a `data-on` key |
| `data-on-intersect="..."` (`__once __half __full __threshold __exit`) | Fire on viewport intersection | Infinite-scroll for long record lists |
| `data-on-signal-patch="..."` + `data-on-signal-patch-filter="{include:/x/}"` | React to any signal patch | Debug/telemetry hook |
| `data-computed:total="$a + $b"` | Read-only derived signal | |
| `data-ref:input` | Signal holding the element reference | |
| `data-indicator:saving` | `true` while a fetch from this element is in flight | Drives spinners *and* `aria-busy` |
| `data-init="..."` (`__delay`, `__viewtransition`) | **Replaces v0's `data-on-load`.** Run once on attach | Where you kick off a long-lived SSE stream |
| `data-effect="..."` | Run on load and on every dependency change | |
| `data-style:display="..."` | Reactive inline styles | New in v1.0 |
| `data-json-signals` (`__terse`) | Dump signals as JSON into the element | Dev only. **Never ship** — it will render PHI into the DOM |
| `data-ignore` (`__self`) | Datastar skips this subtree | Wrap any third-party widget |
| `data-ignore-morph` | Subtree is not morphed | Preserve a chart/canvas across patches |
| `data-preserve-attr="open"` | Keep an attribute across morphs | `<details open>`, dialog state |

**Form submit auto-prevents.** Verified in the bundle's `on` plugin:

```js
e instanceof HTMLFormElement && o === "submit" && c.preventDefault()
```

So `<form data-on:submit="@post('/x')">` needs **no** `__prevent`. Non-form elements still need it.

### Attribute name grammar (from the bundle's own parser)

```js
hn = e => {
  let [t, ...n] = e.split("__");         // modifiers split on __
  let [r, s]    = t.split(/:(.+)/);      // pluginName : key  (first colon only)
  ...                                     // each modifier splits on . for args
  return { pluginName: r, key: s, mods: i };
}
```

Read this carefully, because it explains a confusing pair:

- `data-on:click` → plugin `on`, key `click` ✅
- `data-on-click` → plugin `on-click` → **no such plugin, silently does nothing** ❌
- `data-on-intersect` → plugin `on-intersect` ✅ (genuinely hyphenated plugin name)

Full example with everything: `data-on:input__debounce.300ms__stop="@get('/api/v1/search')"`.

---

## 2. The SSE protocol

Datastar v1 understands exactly **two** event types. Confirmed both in the Go SDK's `consts.go` and
in the JS bundle's watcher registrations:

```go
EventTypePatchElements EventType = "datastar-patch-elements"
EventTypePatchSignals  EventType = "datastar-patch-signals"
```

### `datastar-patch-elements`

```
event: datastar-patch-elements
data: selector #med-list
data: mode inner
data: useViewTransition true
data: viewTransitionSelector #med-list
data: namespace svg
data: elements <li id="med-7">Metformin 500mg</li>
data: elements <li id="med-8">Lisinopril 10mg</li>

```

Data-line keys, defaults, and omission rules (from `elements.go`):

| Key | Default | Emitted when |
|---|---|---|
| `selector` | *(none → match by element `id`)* | non-empty |
| `mode` | `outer` | `mode != outer` |
| `namespace` | `html` | not empty and not `html` |
| `useViewTransition` | `false` | true |
| `viewTransitionSelector` | `document` | set **and** `useViewTransition` is true |
| `elements` | *(required except for `remove`)* | non-empty; **one `data: elements` line per `\n`** |

**Modes** (bundle's own list: `["remove","outer","inner","replace","prepend","append","before","after"]`):

| Mode | Semantics |
|---|---|
| `outer` | **Morph** the target's outer HTML. Default. Preserves focus/scroll/uncontrolled state |
| `inner` | Morph the target's children |
| `replace` | `outerHTML =` — **no morph**, blows away focus, selection, and any element state |
| `prepend` | Insert as first children of target |
| `append` | Insert as last children of target |
| `before` | Insert as previous sibling of target |
| `after` | Insert as next sibling of target |
| `remove` | Delete target elements. `elements` is omitted |

**Selector semantics.** If `selector` is absent, Datastar matches each **top-level element in the
payload** against the DOM **by its `id`**. If `selector` is present, it is a CSS selector and it
targets *all* matches. Consequences:

- No selector + no `id` on the payload's root element → `PatchElementsNoTargetsFound` in the console.
- `mode remove` requires a selector (`PatchElementsExpectedSelector` otherwise).
- Multi-match selectors patch every match — `WithSelector(".med-row")` will hit all rows.

**Prefer the no-selector, id-matched form.** It keeps the target contract in the templ component
where it belongs (§4), and one event can patch several disjoint regions at once.

### `datastar-patch-signals`

```
event: datastar-patch-signals
data: onlyIfMissing true
data: signals {"medication":{"dose":"500mg"},"errors":{"dose":null}}

```

| Key | Default | Notes |
|---|---|---|
| `onlyIfMissing` | `false` | Only create signals that don't exist. Good for hydration-without-clobbering |
| `signals` | required | JSON merge-patch. **`null` deletes the signal** |

### The non-SSE fast path (undocumented in the guide, real in the bundle)

Datastar's fetch actions do **not** require `text/event-stream`. The bundle branches on response
`Content-Type`:

```js
if (ct?.includes("text/html"))        -> treat whole body as datastar-patch-elements "elements"
                                          (honours headers selector/mode/namespace/useViewTransition)
if (ct?.includes("application/json")) -> treat whole body as datastar-patch-signals "signals"
if (ct?.includes("text/javascript"))  -> append as a <script> to <head> and run it
```

**This matters enormously for MediKube.** The overwhelming majority of interactions — save a record,
delete a row, filter a list — are *one* patch and *one* response. For those you can skip SSE
entirely and just write `Content-Type: text/html` + the rendered templ component. That is a plain
PB handler, no streaming, no write-deadline problem (§5), no gzip question, trivially testable.

**Spec rule: SSE is opt-in for genuinely streaming endpoints. Everything else returns `text/html`.**

Also worth knowing: fetch lifecycle is observable as DOM events on `document` named
`datastar-fetch`, with `detail.type` ∈ `started | finished | error | retrying | retries-failed`.
That is your hook for a global error toast and for the Playwright network gate (§9).

Fetch defaults from the bundle: `retryInterval: 1000`, `retryScaler: 2`, `retryMaxWait: 30000`,
`retryMaxCount: 10`, `requestCancellation: "auto"`, and `openWhenHidden` defaults to **`false` for
`@get`** and **`true`** for the other verbs. A long-lived `@get` stream therefore **drops when the
tab is backgrounded** unless you pass `{openWhenHidden: true}`.

---

## 3. The Go SDK (`datastar-go` v1.2.2)

```go
import "github.com/starfederation/datastar-go/datastar"
```

### Complete surface (read off the module source, not pkg.go.dev)

```go
// construction
func NewSSE(w http.ResponseWriter, r *http.Request, opts ...SSEOption) *ServerSentEventGenerator
func WithContext(ctx context.Context) SSEOption            // must derive from r.Context()
func WithCompression(opts ...CompressionOption) SSEOption  // see §5 - DO NOT USE under PB

// generator
func (sse *ServerSentEventGenerator) Context() context.Context
func (sse *ServerSentEventGenerator) IsClosed() bool
func (sse *ServerSentEventGenerator) Send(t EventType, dataLines []string, opts ...SSEEventOption) error

// elements
func (sse *ServerSentEventGenerator) PatchElements(elements string, opts ...PatchElementOption) error
func (sse *ServerSentEventGenerator) PatchElementf(format string, args ...any) error
func (sse *ServerSentEventGenerator) PatchElementTempl(c TemplComponent, opts ...PatchElementOption) error
func (sse *ServerSentEventGenerator) RemoveElement(selector string, opts ...PatchElementOption) error
func (sse *ServerSentEventGenerator) RemoveElementf(format string, args ...any) error
func (sse *ServerSentEventGenerator) RemoveElementByID(id string) error

// signals
func (sse *ServerSentEventGenerator) PatchSignals(json []byte, opts ...PatchSignalsOption) error
func (sse *ServerSentEventGenerator) MarshalAndPatchSignals(v any, opts ...PatchSignalsOption) error
func (sse *ServerSentEventGenerator) MarshalAndPatchSignalsIfMissing(v any, opts ...PatchSignalsOption) error
func (sse *ServerSentEventGenerator) PatchSignalsIfMissingRaw(json string) error

// script-based helpers (all compile down to PatchElements of a <script>)
func (sse *ServerSentEventGenerator) ExecuteScript(js string, opts ...ExecuteScriptOption) error
func (sse *ServerSentEventGenerator) Redirect(url string, opts ...ExecuteScriptOption) error
func (sse *ServerSentEventGenerator) Redirectf(format string, args ...any) error
func (sse *ServerSentEventGenerator) ConsoleLog(msg string, opts ...ExecuteScriptOption) error
func (sse *ServerSentEventGenerator) ConsoleError(err error, opts ...ExecuteScriptOption) error
func (sse *ServerSentEventGenerator) DispatchCustomEvent(name string, detail any, opts ...DispatchCustomEventOption) error
func (sse *ServerSentEventGenerator) ReplaceURL(u url.URL, opts ...ExecuteScriptOption) error
func (sse *ServerSentEventGenerator) ReplaceURLQuerystring(r *http.Request, v url.Values, opts ...ExecuteScriptOption) error
func (sse *ServerSentEventGenerator) Prefetch(urls ...string) error

// reading signals from the request
func ReadSignals(r *http.Request, signals any) error

// options
WithSelector(string) / WithSelectorf(string, ...any) / WithSelectorID(string)
WithMode(ElementPatchMode)
WithModeOuter() WithModeInner() WithModeReplace() WithModePrepend()
WithModeAppend() WithModeBefore() WithModeAfter() WithModeRemove()
WithNamespace(Namespace) / WithNamespaceHTML() WithNamespaceSVG() WithNamespaceMathML()
WithUseViewTransitions(bool) / WithViewTransitions() / WithoutViewTransitions()
WithViewTransitionSelector(string)
WithOnlyIfMissing(bool)
WithRetryDuration(time.Duration)
WithPatchElementsEventID(string) / WithPatchSignalsEventID(string) / WithSSEEventId(string)
```

### API naming corrections against your brief

Your brief listed a few names that do not exist in v1.2.2:

| You wrote | Actual v1.2.2 name |
|---|---|
| `sse.RemoveElements(...)` | **`sse.RemoveElement(selector)`** (singular), plus `RemoveElementf`, `RemoveElementByID` |
| `WithUseViewTransitions` ✅ | correct |
| `sse.PatchElementTempl` ✅ | correct — singular "Element", plural "Elements" everywhere else. Ugly, but that's the API |

### `ReadSignals` — read it BEFORE you open the SSE stream

```go
func ReadSignals(r *http.Request, signals any) error {
	if r.Method == "GET" || r.Method == "DELETE" {
		dsJSON := r.URL.Query().Get("datastar")   // DatastarKey
		if dsJSON == "" { return nil }            // NOTE: silently succeeds, leaves target zeroed
		...
	} else {
		// reads r.Body as JSON
	}
	return json.Unmarshal(dsInput, signals)
}
```

Three things the docs won't tell you:

1. **GET and DELETE read the `?datastar=` query param. Everything else reads the JSON body.**
2. **On GET with no `datastar` param, `ReadSignals` returns `nil` and leaves your struct at zero
   values.** It does not error. Always validate after reading; never treat `err == nil` as
   "signals were present."
3. **Order matters and the SDK will tell you off if you get it wrong.** Its own error text:
   `"body already closed, are you sure you created the SSE ***AFTER*** the ReadSignals?"`
   `NewSSE` flushes headers immediately, which commits the response. Read signals first. Always.

### Full round trip: add a medication

The pattern MediKube should standardise on. Note it uses the **`text/html` fast path** for the happy
path is *not* used here deliberately — this example shows the SSE form because it patches two
disjoint regions plus signals in one response, which is where SSE genuinely earns its keep.

**Component (`internal/ui/medications.templ`)**

```templ
package ui

import "medikube/internal/domain"

// Stable-ID contract: this component OWNS #med-form and never renders it nested
// inside another patch target.
templ MedicationForm(f domain.MedicationForm, errs map[string]string) {
	<form
		id="med-form"
		data-signals={ templ.JSONString(f) }
		data-on:submit="@post('/api/v1/medications')"
		data-indicator:saving
		data-attr:aria-busy="$saving"
		novalidate
	>
		<div>
			<label for="med-name">Medication name</label>
			<input
				id="med-name"
				name="name"
				data-bind:name
				data-attr:aria-invalid={ errs["name"] != "" }
				aria-describedby="med-name-err"
				required
			/>
			<p id="med-name-err" role="alert" data-text="$errors.name" data-show="$errors.name"></p>
		</div>

		<div>
			<label for="med-dose">Dose</label>
			<input id="med-dose" name="dose" data-bind:dose aria-describedby="med-dose-err" required/>
			<p id="med-dose-err" role="alert" data-text="$errors.dose" data-show="$errors.dose"></p>
		</div>

		<button type="submit" data-attr:disabled="$saving">
			<span data-show="!$saving">Save</span>
			<span data-show="$saving">Saving&hellip;</span>
		</button>
	</form>
}

templ MedicationRow(m domain.Medication) {
	<li id={ "med-" + m.ID } class="flex justify-between gap-4 py-2">
		<span>{ m.Name }</span>
		<span>{ m.Dose }</span>
	</li>
}

templ MedicationList(ms []domain.Medication) {
	<ul id="med-list" role="list">
		for _, m := range ms {
			@MedicationRow(m)
		}
	</ul>
}
```

**Handler (`internal/httpapi/medications.go`)**

```go
package httpapi

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/starfederation/datastar-go/datastar"

	"medikube/internal/domain"
	"medikube/internal/ui"
)

// createMedicationSignals mirrors the client-side signal shape exactly.
// Signals prefixed with _ are client-local and never arrive here.
type createMedicationSignals struct {
	Name string `json:"name"`
	Dose string `json:"dose"`
}

func (h *MedicationHandler) Create(e *core.RequestEvent) error {
	// 1. READ SIGNALS FIRST. NewSSE commits the response and closes the body.
	var in createMedicationSignals
	if err := datastar.ReadSignals(e.Request, &in); err != nil {
		return e.BadRequestError("Malformed signals", err)
	}

	// 2. Open the stream only now.
	sse := datastar.NewSSE(e.Response, e.Request)

	// 3. Validate. Errors go back as SIGNALS - the form markup never re-renders,
	//    so focus and caret position survive.
	if errs := domain.ValidateMedicationForm(in.Name, in.Dose); len(errs) > 0 {
		return sse.MarshalAndPatchSignals(map[string]any{
			"errors": errs, // e.g. {"dose": "Dose is required"}
		})
	}

	med, err := h.svc.Create(e.Request.Context(), domain.NewMedication{
		PatientID: currentPatientID(e),
		Name:      in.Name,
		Dose:      in.Dose,
	})
	if err != nil {
		return sse.MarshalAndPatchSignals(map[string]any{
			"errors": map[string]string{"_form": "Could not save. Please try again."},
		})
	}

	// 4. Append the new row. No selector needed for the row itself because we
	//    are explicitly targeting the list container.
	if err := sse.PatchElementTempl(
		ui.MedicationRow(med),
		datastar.WithSelectorID("med-list"),
		datastar.WithModeAppend(),
	); err != nil {
		return err
	}

	// 5. Reset the form's signals and clear errors. null deletes a signal.
	return sse.MarshalAndPatchSignals(map[string]any{
		"name":   "",
		"dose":   "",
		"errors": nil,
	})
}
```

**Wire shape on the wire** (what Playwright / curl will see):

```
event: datastar-patch-elements
data: selector #med-list
data: mode append
data: elements <li id="med-42" class="flex justify-between gap-4 py-2"><span>Metformin</span><span>500mg</span></li>

event: datastar-patch-signals
data: signals {"dose":"","errors":null,"name":""}

```

**Why signals-for-errors instead of re-rendering the form:** patching the form's outer HTML on every
validation failure yanks focus out of the field the user is typing in. Patching only `$errors`
leaves the DOM structurally identical and lets `data-text`/`data-show` do the work. This is the
single biggest UX difference between a good and a bad Datastar form.

---

## 4. templ integration

### `PatchElementTempl` is a 6-line adaptor

```go
type TemplComponent interface {
	Render(ctx context.Context, w io.Writer) error
}

func (sse *ServerSentEventGenerator) PatchElementTempl(c TemplComponent, opts ...PatchElementOption) error {
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)
	if err := c.Render(sse.Context(), buf); err != nil { ... }
	return sse.PatchElements(buf.String(), opts...)
}
```

Two consequences:

1. **The SDK has no compile-time dependency on templ.** It structurally matches `templ.Component`.
   No import cycle, no version coupling. templ can be upgraded independently.
2. **It renders with `sse.Context()`**, which is `r.Context()` unless you passed `WithContext`. Any
   value your templ components pull out of context (current patient, locale, CSRF nonce, `zerolog`
   logger) must already be on the request context. If you need to *add* something for the render,
   use `datastar.WithContext(context.WithValue(e.Request.Context(), k, v))` at `NewSSE` time — the
   SDK explicitly documents that this must derive from the request context.

### Structuring components for partial patches

The rule that makes or breaks a Datastar+templ codebase:

> **Every patch target is a templ component whose root element carries a stable, deterministic
> `id`, and that component is the ONLY thing that renders that `id`.**

```templ
// GOOD - the id lives with the component that owns it. Server and client agree by construction.
templ LabResultCard(r domain.LabResult) {
	<article id={ labResultCardID(r.ID) } aria-labelledby={ labResultCardID(r.ID) + "-title" }>
		<h3 id={ labResultCardID(r.ID) + "-title" }>{ r.TestName }</h3>
		...
	</article>
}
```

```go
// internal/ui/ids.go - one place, used by BOTH the templ render and the Go patch call.
package ui

func LabResultCardID(id string) string { return "lab-result-" + id }
func LabResultListID                    = "lab-result-list"
```

```go
// The handler never types a raw selector string.
sse.PatchElementTempl(ui.LabResultCard(r), datastar.WithSelectorID(ui.LabResultCardID(r.ID)))
```

**Anti-patterns to ban in review:**

- Raw string selectors in handlers (`WithSelector("#lab-result-"+id)`) — drifts from the template.
- A component that renders a different root element depending on a branch (`if x { <div id=..> }
  else { <section id=..> }`) — `outer` morph across differing tag names degrades to a replace and
  destroys state.
- Nesting a patch target inside another patch target. Patching the outer one recreates the inner
  one, orphaning any in-flight fetch bound to it (remember: **fetches auto-cancel when their
  initiating element is removed**).
- Putting `data-init="@get('/stream')"` on an element that is itself inside a patch target.

### Codegen workflow

Verified from `templ generate -help` in v0.3.1020:

```bash
templ generate                 # recurse from ., write *_templ.go
templ generate -f x.templ      # single file
templ generate -watch          # dev; ALSO writes _templ.txt sidecars
templ generate -lazy           # only regenerate when .templ is newer
templ generate -check          # CI: verify up-to-date, non-zero exit if not  <-- the gate
templ generate -include-version=false   # drop the version comment (reduces diff churn)
templ fmt .                    # format in place
templ fmt -fail .              # CI: non-zero exit if reformatting needed
templ lsp                      # language server (wraps gopls)
templ version
```

**Should `*_templ.go` be committed? Yes, for MediKube.** The upstream project's own guidance is
"prefer not to, unless consumers need to build without templ installed" — but MediKube's constraints
push the other way:

- The Docker build must not need the templ binary (keeps the image and CI simple, and matches the
  monorepo's `.dockerignore` allowlist discipline).
- `go build ./...`, `go vet`, `golangci-lint`, and `gopls` all need the generated code present.
- Fresh-clone `go test ./...` must just work.

So: **commit them, and make CI prove they're current.** Combine `-check` with `-include-version=false`
so a templ patch bump doesn't rewrite every file and blow up the diff.

```yaml
# Taskfile.yaml
version: '3'

vars:
  TEMPL_VERSION: v0.3.1020

tasks:
  templ:install:
    status: ['templ version | grep -q "{{.TEMPL_VERSION}}"']
    cmds:
      - go install github.com/a-h/templ/cmd/templ@{{.TEMPL_VERSION}}

  templ:generate:
    deps: [templ:install]
    sources: ['internal/ui/**/*.templ']
    generates: ['internal/ui/**/*_templ.go']
    cmds:
      - templ generate -include-version=false

  templ:fmt:
    deps: [templ:install]
    cmds: ['templ fmt .']

  templ:check:            # CI gate
    deps: [templ:install]
    cmds:
      - templ generate -include-version=false -check
      - templ fmt -fail .
```

```gitignore
# Generated *_templ.go IS committed. These are not:
*_templ.txt
```

**The `_templ.txt` trap:** `templ generate -watch` rewrites `*_templ.go` into a slow,
hot-reload-friendly form that reads literals from `_templ.txt` sidecars, and only restores the
production form when the watcher **exits cleanly**. If you `git commit` while the watcher is
running — or kill it with `SIGKILL` — you commit dev-mode generated code that depends on `.txt`
files you just gitignored. Symptom: works locally, blank/garbled HTML in Docker.

Mitigations, both cheap, put both in:

1. `.gitignore` the `_templ.txt` files (above), so the breakage is loud rather than subtle.
2. Make `templ:check` a pre-push hook and a required CI job — it catches dev-mode output because the
   regenerated production file won't match.

### templ parses Datastar's colon syntax fine

I checked this because it is a real risk with a custom template language. templ's attribute-name
grammar (`parser/v2/elementparser.go`):

```go
attributeNameFirst      = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ:_@"
attributeNameSubsequent = attributeNameFirst + "-.0123456789*"
```

`:`, `-`, `.`, `_`, and digits are all legal in subsequent position, so
`data-on:input__debounce.300ms` parses cleanly. **No escaping, no `templ.Attributes` workaround
needed.** (`templ.Attributes` — `map[string]any` — is still useful for conditionally spreading a
variable set of `data-*` attributes.)

For dynamic expressions use `templ.JSONString` (signature in v0.3.1020 is
`func JSONString(v any) (string, error)`; templ handles the error return in attribute position):

```templ
<div data-signals={ templ.JSONString(initialSignals) }></div>
```

Or the SDK's action helpers, which are just `fmt.Sprintf` wrappers:

```templ
<button data-on:click={ datastar.PostSSE("/api/v1/patients/%s/switch", p.ID) }>Switch</button>
```

---

## 5. Serving from PocketBase routes

### Rendering a templ component to `core.RequestEvent`

`core.RequestEvent` embeds `router.Event`, which exposes `Response http.ResponseWriter` and
`Request *http.Request` as plain fields. So it is just an `io.Writer`:

```go
package httpapi

import (
	"github.com/a-h/templ"
	"github.com/pocketbase/pocketbase/core"
)

// render writes a templ component as a full HTML page response.
func render(e *core.RequestEvent, status int, c templ.Component) error {
	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	e.Response.WriteHeader(status)
	return c.Render(e.Request.Context(), e.Response)
}

// renderPatch writes a component as a Datastar element patch WITHOUT SSE.
// This is the fast path from §2 - use it for every single-patch interaction.
func renderPatch(e *core.RequestEvent, c templ.Component, mode string, selector string) error {
	h := e.Response.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	if selector != "" { h.Set("selector", selector) }
	if mode != ""     { h.Set("mode", mode) }
	return c.Render(e.Request.Context(), e.Response)
}
```

Route registration:

```go
app.OnServe().BindFunc(func(se *core.ServeEvent) error {
	se.Router.GET("/patients/{id}/medications", func(e *core.RequestEvent) error {
		meds, err := medSvc.List(e.Request.Context(), e.Request.PathValue("id"))
		if err != nil {
			return e.InternalServerError("Failed to load medications", err)
		}
		return render(e, 200, ui.MedicationsPage(meds))
	})
	return se.Next()
})
```

Note `return se.Next()` — PocketBase v0.40's hook API requires it or the chain stops dead.

### Running a Datastar SSE stream from a PB handler

There are **three** PocketBase-specific things you must do that no Datastar example will mention.
All three are verified against PB v0.40.1 source; PocketBase's own `realtimeConnect` handler does
exactly this and Datastar's `NewSSE` does none of it.

#### (a) Clear the write deadline — **PB sets `WriteTimeout: 5 * time.Minute`**

`apis/serve.go:152`:

```go
server := &http.Server{
	WriteTimeout:      5 * time.Minute,
	ReadTimeout:       5 * time.Minute,
	ReadHeaderTimeout: 1 * time.Minute,
	...
}
```

`datastar.NewSSE` sets `Cache-Control`, `Content-Type`, `Connection` and flushes — **it never touches
the write deadline.** So a long-lived Datastar stream on a PB route dies at exactly 5 minutes with a
write error, and Datastar's client will silently reconnect in a loop. This is a nasty one because it
passes every test shorter than 5 minutes.

PB's own realtime handler works around it (`apis/realtime.go:44`), and so must yours.

#### (b) Set `X-Accel-Buffering: no`

PB sets it on its realtime stream and links the rationale. Datastar's `NewSSE` does not. Without it,
an nginx reverse proxy in front of MediKube buffers the whole stream and nothing arrives until close.

#### (c) Skip the activity logger

PB's global `activityLogger()` middleware logs on response completion. For a 30-minute stream that
is a useless record written at the end; PB tags its own realtime route with
`Bind(SkipSuccessActivityLog())`. Do the same. (MediKube disables `_logs` anyway, but the middleware
still runs and the zerolog bridge will emit the line.)

**The helper every MediKube SSE handler must go through:**

```go
package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/starfederation/datastar-go/datastar"
)

// newStream upgrades a PocketBase request to a Datastar SSE stream, applying the
// three PocketBase-specific fixes that datastar.NewSSE does not handle.
func newStream(e *core.RequestEvent) (*datastar.ServerSentEventGenerator, error) {
	// (a) PB's http.Server has WriteTimeout: 5m. Clear it for this connection or
	//     the stream is torn down mid-flight after five minutes.
	rc := http.NewResponseController(e.Response)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return nil, e.InternalServerError("Failed to initialise SSE stream", err)
	}

	// (b) Don't let an upstream nginx buffer the stream into oblivion.
	e.Response.Header().Set("X-Accel-Buffering", "no")

	// (c) datastar.NewSSE sets Cache-Control: no-cache; PB's realtime uses no-store.
	//     no-store is the correct choice for a medical app.
	e.Response.Header().Set("Cache-Control", "no-store")

	return datastar.NewSSE(e.Response, e.Request), nil
}
```

Registration, with the activity-log skip:

```go
se.Router.GET("/api/v1/streams/vitals", h.VitalsStream).
	Bind(apis.SkipSuccessActivityLog())
```

`http.NewResponseController` works through PB's wrappers because `router.ResponseWriter` implements
`Unwrap() http.ResponseWriter` (`tools/router/router.go:312`) and PB defines the `RWUnwrapper`
interface for exactly this. So `SetWriteDeadline`, `Flush`, and `Hijack` all reach the real
`net/http` writer. Verified.

### The gzip verdict

**Verdict: there is no conflict, because PocketBase does not gzip your routes.**

I traced every binding of `apis.Gzip()` in v0.40.1. There are exactly two, and both are scoped to
the admin UI static tree:

```
apis/serve.go:99       pbRouter.GET("/_/{path...}", Static(ui.DistDirFS,false)).<...>.Bind(Gzip())
apis/extensions.go:39  se.Router.Group("/_").<...>.Bind(Gzip())
```

The global default middleware chain (`apis/base.go:30-36`) is:
`activityLogger, panicRecover, rateLimit, loadAuthToken, superuserIPsWhitelist, securityHeaders,
BodyLimit` — **no gzip.** So `/api/v1/*` and MediKube's page routes are uncompressed by default and
SSE streams flush straight through.

Three follow-ups, though, because the trap is real:

1. **Do not add `apis.Gzip()` globally.** If somebody later binds it at the router root "for
   performance", it will wrap SSE too. It is *survivable* — PB's `gzipResponseWriter.Flush()` does
   call `gzip.Writer.Flush()` then flushes the underlying writer, so events do get out — but you pay
   a gzip flush block per event, and any proxy or client that mishandles chunked+gzip SSE will break.
   Bind gzip to specific route groups, never the root. Consider a `golangci-lint` forbidigo rule.

2. **Do not use `datastar.WithCompression(...)`.** The SDK can compress the stream itself. Under
   PocketBase that is redundant at best. Worse, if anything else in the chain also sets
   `Content-Encoding`, you get double-encoding and an unreadable stream. Leave SSE uncompressed;
   the payloads are small HTML fragments and the latency matters more than the bytes.

3. **Compress the *static assets* instead**, where it actually pays: serve pre-compressed
   `datastar.js.gz` and `app.css.gz` from the embedded FS, or bind `apis.Gzip()` to just the
   `/assets` group. Never to the group that holds streams.

### Other PB middleware interactions

- **`rateLimit()`** is global. A long-lived SSE connection is one request, so it consumes one token
  and then sits there — fine. But an aggressive per-IP rule plus Datastar's reconnect loop
  (`retryMaxCount: 10`, exponential backoff to 30s) can lock a user out after a server restart.
  Exempt the stream routes, or set the limit generously for them.
- **`securityHeaders()`** sets `X-Frame-Options: SAMEORIGIN`, `X-Content-Type-Options: nosniff`,
  `Cross-Origin-Opener-Policy: same-origin`, `X-XSS-Protection`. It does **not** set a CSP. PB's
  `defaultCSP` is applied only to the `/_` admin UI. **MediKube must set its own CSP** — see §9, and
  note the `ExecuteScript` interaction, which is the one genuine footgun.
- **`BodyLimit(DefaultMaxBodySize)`** applies to the signals JSON body on POST. Datastar sends *all*
  non-`_` signals on every request, so a page with a large hydrated dataset in signals can hit it.
  Another reason to keep bulk data in the DOM and only put *control* state in signals.

---

## 6. Datastar + PocketBase realtime: the honest assessment

### They are NOT protocol-compatible. Not even close.

Two independent incompatibilities, both verified in source.

**(1) The event names don't match, and Datastar ignores everything it doesn't recognise.**

PocketBase's wire format (`tools/subscriptions/message.go`):

```go
func (m *Message) WriteSSE(w io.Writer, eventId string) error {
	parts := [][]byte{
		[]byte("id:" + eventId + "\n"),
		[]byte("event:" + m.Name + "\n"),   // m.Name == "medications/abc123" or "PB_CONNECT"
		[]byte("data:"),
		m.Data,                              // {"action":"update","record":{...}}
		[]byte("\n\n"),
	}
	...
}
```

So a PocketBase record change on the wire is:

```
id:5f2a...
event:medications/abc123
data:{"action":"update","record":{"id":"abc123","name":"Metformin","dose":"850mg"}}

```

Datastar registers watchers for exactly two event names (`datastar-patch-elements`,
`datastar-patch-signals`) and drops everything else on the floor. It will happily hold the
connection open and do absolutely nothing. **Silent no-op, not an error** — which is the worst
possible failure mode.

**(2) PB realtime needs a two-step handshake Datastar cannot perform.**

`apis/realtime.go` binds:

```go
sub.GET("",  realtimeConnect)          // opens the stream, emits PB_CONNECT with a clientId
sub.POST("", realtimeSetSubscriptions) // you must POST {clientId, subscriptions:[...]} to get anything
```

You connect, PB hands you a `clientId` in the `PB_CONNECT` event, and you must then make a *separate
POST* echoing that `clientId` plus your topic list. Until you do, the stream sends nothing but
keepalives. Datastar's `@get` opens a stream and consumes it — it has no mechanism to parse a custom
event, extract an ID, and fire a follow-up POST. You would need hand-written JS, which defeats the
entire point.

**(3) Even if you bridged the format, you'd be bridging JSON, not HTML.** PB broadcasts *record
JSON*. Datastar wants *rendered elements* or *signal patches*. Something has to run templ. That
something is Go, on the server. Which is the bridge.

### Where the locked decision still holds

"Use PB's native realtime where PB natively supports it" remains correct — but be precise about what
that means, because it is narrower than it sounds:

- **PocketBase's admin UI** (`/_`) uses PB realtime internally. Untouched, free, works.
- **Any future non-Datastar client** (a native mobile app, a JS SDK consumer) can use
  `/api/realtime` directly. Keep it enabled.
- **MediKube's own Datastar UI cannot consume it.** For that, bridge.

### The bridge design

**Shape:** PB record hooks → in-process fan-out hub → per-subscriber Datastar SSE stream that
renders templ and emits `datastar-patch-elements`.

```
core.App hooks                    Hub                        HTTP
─────────────────────  ────────────────────────  ─────────────────────────────
OnRecordAfterCreateSuccess ─┐
OnRecordAfterUpdateSuccess ─┼─► hub.Publish(Change) ──► per-subscriber
OnRecordAfterDeleteSuccess ─┘      (non-blocking,        buffered chan
                                    fire-and-forget)          │
                                                              ▼
                                              GET /api/v1/streams/{topic}
                                              ├─ authorise subscriber  ◄── CRITICAL
                                              ├─ render templ component
                                              └─ sse.PatchElementTempl(...)
```

**Hook names** (verified, `core/base.go`): `OnRecordAfterCreateSuccess(tags ...string)`,
`OnRecordAfterUpdateSuccess`, `OnRecordAfterDeleteSuccess`. The variadic tags are collection
names/ids, so you subscribe narrowly. Use the `...AfterXxxSuccess` variants specifically — they fire
**after the transaction commits**, so you never render a row that gets rolled back.

```go
package realtime

import (
	"context"
	"sync"

	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"
)

type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// Change is the internal, transport-agnostic event. Deliberately carries IDs, not
// a *core.Record: the record is re-fetched per subscriber under that subscriber's
// own authorisation. Never fan out a record body you have not authorised.
type Change struct {
	Collection string
	RecordID   string
	PatientID  string
	Action     Action
}

// Hub is the fan-out point. Interface at the seam, per house style.
type Hub interface {
	Publish(Change)
	Subscribe(ctx context.Context, topic string) <-chan Change
}

type hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan Change]struct{} // topic -> set of subscriber channels
	log  zerolog.Logger
}

func NewHub(log zerolog.Logger) Hub {
	return &hub{subs: make(map[string]map[chan Change]struct{}), log: log}
}

func (h *hub) Publish(c Change) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[topicFor(c)] {
		select {
		case ch <- c:
		default:
			// Slow consumer. Drop rather than block: this runs on the request
			// goroutine that just committed a write. NEVER block a PB hook.
			h.log.Warn().Str("collection", c.Collection).Msg("realtime: dropped, slow subscriber")
		}
	}
}

func (h *hub) Subscribe(ctx context.Context, topic string) <-chan Change {
	ch := make(chan Change, 16)
	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = make(map[chan Change]struct{})
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()

	go func() {
		<-ctx.Done()
		h.mu.Lock()
		delete(h.subs[topic], ch)
		if len(h.subs[topic]) == 0 {
			delete(h.subs, topic)
		}
		h.mu.Unlock()
		close(ch)
	}()
	return ch
}

func topicFor(c Change) string { return c.Collection + "/" + c.PatientID }
```

**Wiring the hooks** (once, at boot):

```go
func BindHooks(app core.App, h Hub) {
	watched := []string{"medications", "lab_results", "conditions", "procedures"}

	app.OnRecordAfterCreateSuccess(watched...).BindFunc(func(e *core.RecordEvent) error {
		h.Publish(changeFrom(e.Record, ActionCreate))
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess(watched...).BindFunc(func(e *core.RecordEvent) error {
		h.Publish(changeFrom(e.Record, ActionUpdate))
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess(watched...).BindFunc(func(e *core.RecordEvent) error {
		h.Publish(changeFrom(e.Record, ActionDelete))
		return e.Next()
	})
}

func changeFrom(r *core.Record, a Action) Change {
	return Change{
		Collection: r.Collection().Name,
		RecordID:   r.Id,
		PatientID:  r.GetString("patient"),
		Action:     a,
	}
}
```

**The stream handler** — and this is where the security work lives:

```go
func (h *StreamHandler) Medications(e *core.RequestEvent) error {
	patientID := e.Request.PathValue("patientID")

	// AUTHORISE ONCE AT CONNECT. PocketBase's own realtime re-runs collection API
	// rules per record per subscriber; this bridge bypasses that engine entirely,
	// so authorisation is OUR job. Getting this wrong leaks another patient's
	// records into someone's browser.
	if err := h.authz.RequirePatientAccess(e, patientID); err != nil {
		return e.ForbiddenError("No access to this patient", err)
	}

	sse, err := newStream(e) // the §5 helper: write deadline, X-Accel-Buffering, no-store
	if err != nil {
		return err
	}

	ctx := e.Request.Context()
	changes := h.hub.Subscribe(ctx, "medications/"+patientID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case c, ok := <-changes:
			if !ok {
				return nil
			}
			// RE-AUTHORISE PER EVENT. Access can be revoked mid-stream (a share
			// is withdrawn, a patient is switched). Cheap check, huge payoff.
			if err := h.authz.RequirePatientAccess(e, c.PatientID); err != nil {
				return nil // access revoked: close the stream
			}

			if c.Action == ActionDelete {
				if err := sse.RemoveElementByID(ui.MedicationRowID(c.RecordID)); err != nil {
					return nil // client gone
				}
				continue
			}

			med, err := h.svc.Get(ctx, c.RecordID)
			if err != nil {
				h.log.Warn().Err(err).Str("id", c.RecordID).Msg("realtime: refetch failed")
				continue
			}

			opts := []datastar.PatchElementOption{
				datastar.WithSelectorID(ui.MedicationRowID(med.ID)),
			}
			if c.Action == ActionCreate {
				// New row: there is no #med-<id> to morph onto yet, so append to the list.
				opts = []datastar.PatchElementOption{
					datastar.WithSelectorID(ui.MedicationListID),
					datastar.WithModeAppend(),
				}
			}
			if err := sse.PatchElementTempl(ui.MedicationRow(med), opts...); err != nil {
				return nil // client disconnected; not an error
			}
		}
	}
}
```

**Client side — one attribute:**

```templ
<div
	id="med-stream"
	data-init={ datastar.GetSSE("/api/v1/streams/patients/%s/medications", patientID) }
></div>
```

Put this on an element that is **never itself a patch target**, for the auto-cancel reason in §2.

**Design rules to write into the spec:**

1. `Publish` must never block. Buffered channel + `default:` drop. A PB hook runs on the request
   goroutine that just committed a write; blocking there stalls the writer.
2. Fan out **IDs, not record bodies**. Re-fetch per subscriber. This is what makes per-subscriber
   authorisation possible and is the difference between a bridge and a data leak.
3. Authorise at connect **and** per event.
4. Use `...AfterXxxSuccess` hooks so you never render uncommitted state.
5. `return e.Next()` in every hook, or PB's chain halts.
6. The hub is in-process. **Single-instance only.** MediKube is self-hosted personal medical records,
   so that is fine — but state it explicitly in the spec, because "add a second replica" silently
   breaks realtime for half the users. If horizontal scale ever arrives, the `Hub` interface is the
   seam to swap for Redis/NATS.
7. Keep `/api/realtime` enabled for non-Datastar clients; don't try to make MediKube's UI use it.

---

## 7. Tailwind v4 without Node in the runtime

### The standalone binary

Tailwind ships a self-contained executable — a Bun-compiled single binary with the Oxide engine
inside. No Node, no `node_modules`, no `package.json`. Verified present for **v4.3.3** at:

```
https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-linux-x64
https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-linux-arm64
https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-linux-x64-musl
https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-macos-arm64
https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-macos-x64
https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-windows-x64.exe
```

They are chunky — `tailwindcss-linux-x64` is **~112 MB**, `tailwindcss-macos-arm64` **~80 MB**. That
is fine: it is a **build-time-only** tool. It never enters the runtime image, and it must be in
`.dockerignore`. (Per the monorepo memory note: a new project must be added to `/.dockerignore` and
`build-image.yaml` or the Docker build fails with a misleading "file not found" — the Tailwind
binary and `.cache/` need to be *excluded* there in the same edit.)

### CSS-first config (v4 has no `tailwind.config.js`)

v4 replaces the JS config with CSS directives. Everything lives in one file:

```css
/* internal/ui/assets/app.css */
@import "tailwindcss";

/* v4 auto-detects sources, but it respects .gitignore - and *_templ.go is committed
   while _templ.txt is not. Be explicit rather than depending on the heuristic. */
@source "../**/*.templ";

/* Datastar writes class names into the DOM at runtime via data-class. Tailwind
   cannot see those, so safelist them explicitly. */
@source inline("{hidden,sr-only}");
@source inline("border-{red,green,amber}-{400,500,600}");

@theme {
	--color-brand-50:  oklch(0.97 0.02 250);
	--color-brand-500: oklch(0.62 0.16 250);
	--color-brand-700: oklch(0.48 0.14 250);

	--font-sans: "Inter", ui-sans-serif, system-ui, sans-serif;

	/* Clinical severity scale - one source of truth for lab result flags */
	--color-severity-normal:   oklch(0.72 0.15 150);
	--color-severity-abnormal: oklch(0.78 0.16 80);
	--color-severity-critical: oklch(0.60 0.20 25);
}

@layer components {
	.card { @apply rounded-lg border border-gray-200 bg-white p-4 shadow-sm; }
}
```

### Making Tailwind scan `.templ` files

Two mechanisms, and you want both belt and braces:

1. **Automatic source detection.** v4 scans the project tree for candidate class names, skipping
   anything in `.gitignore` plus binary/asset extensions. `.templ` is an unknown text extension, so
   it *is* picked up. But this silently depends on your `.gitignore`, which is a fragile coupling.
2. **`@source "../**/*.templ";`** — explicit, greppable, survives `.gitignore` edits. **Use this.**

Scan the `.templ` sources, **not** the generated `_templ.go`. Both contain the class strings, but
`.templ` is the source of truth and scanning both just doubles the work.

**The caveat that catches everyone:** Tailwind's scanner is a plain-text scanner. It finds complete
static strings. These do **not** work:

```templ
<div class={ "text-" + severity }>              <!-- BROKEN: Tailwind never sees text-critical -->
<div class={ fmt.Sprintf("bg-%s-500", color) }> <!-- BROKEN -->
```

Do this instead — full class names on every branch:

```templ
templ SeverityBadge(s domain.Severity) {
	<span class={ severityClass(s) }>{ s.Label() }</span>
}
```

```go
// internal/ui/severity.go - every literal appears whole, so the scanner finds them.
func severityClass(s domain.Severity) string {
	switch s {
	case domain.SeverityCritical:
		return "rounded px-2 py-1 text-xs font-medium bg-red-100 text-red-800"
	case domain.SeverityAbnormal:
		return "rounded px-2 py-1 text-xs font-medium bg-amber-100 text-amber-800"
	default:
		return "rounded px-2 py-1 text-xs font-medium bg-green-100 text-green-800"
	}
}
```

templ's `classes` helper and `templ.KV` are also fine — same rule: literals must appear whole.

### Taskfile wiring

```yaml
version: '3'

vars:
  TAILWIND_VERSION: v4.3.3
  TAILWIND_BIN: '{{.ROOT_DIR}}/.cache/tailwindcss'
  CSS_IN:  internal/ui/assets/app.css
  CSS_OUT: internal/ui/assets/dist/app.css

tasks:
  tailwind:install:
    status: ['test -x {{.TAILWIND_BIN}}']
    cmds:
      - mkdir -p {{.ROOT_DIR}}/.cache
      - |
        os=$(uname -s | tr '[:upper:]' '[:lower:]')
        arch=$(uname -m)
        case "$arch" in x86_64) arch=x64 ;; aarch64|arm64) arch=arm64 ;; esac
        case "$os"   in darwin) os=macos ;; esac
        curl -sSfL -o {{.TAILWIND_BIN}} \
          "https://github.com/tailwindlabs/tailwindcss/releases/download/{{.TAILWIND_VERSION}}/tailwindcss-${os}-${arch}"
        chmod +x {{.TAILWIND_BIN}}

  css:
    deps: [tailwind:install]
    sources: ['internal/ui/**/*.templ', '{{.CSS_IN}}']
    generates: ['{{.CSS_OUT}}']
    cmds:
      - '{{.TAILWIND_BIN}} -i {{.CSS_IN}} -o {{.CSS_OUT}} --minify'

  css:watch:
    deps: [tailwind:install]
    cmds:
      - '{{.TAILWIND_BIN}} -i {{.CSS_IN}} -o {{.CSS_OUT}} --watch'

  build:
    deps: [templ:generate, css]
    cmds: ['go build -o bin/medikube ./cmd/medikube']

  dev:
    deps: [templ:install, tailwind:install]
    cmds:
      - |
        {{.TAILWIND_BIN}} -i {{.CSS_IN}} -o {{.CSS_OUT}} --watch &
        templ generate -watch -proxy=http://localhost:8090 -cmd="go run ./cmd/medikube serve"
```

```gitignore
.cache/
```

`.dockerignore` must exclude `.cache/` too, or the 112 MB binary rides into the build context.

**Commit `internal/ui/assets/dist/app.css`?** Yes, same argument as `_templ.go`: `go build` must
work without the Tailwind binary, because `embed.FS` fails at compile time if the file is missing.
Gate it in CI the same way:

```yaml
  css:check:
    deps: [tailwind:install]
    cmds:
      - '{{.TAILWIND_BIN}} -i {{.CSS_IN}} -o {{.CSS_OUT}} --minify'
      - git diff --exit-code -- {{.CSS_OUT}}
```

---

## 8. Datastar asset delivery: embed it, don't CDN it

**Confirmed: vendor and embed. CDN is both unacceptable and unnecessary here.**

For a self-hosted medical records app, a CDN `<script>` tag means every page load leaks the
deployment's existence and the user's IP/User-Agent to a third party, and hands an outside party
script-execution rights inside the origin that renders PHI. It also breaks air-gapped installs,
which is a normal deployment mode for this class of software. Non-starter.

### Sizes (measured, not estimated)

| File | Raw | gzip -9 |
|---|---|---|
| `bundles/datastar.js` | **34,083 B** (33 KB) | **13,280 B** (13 KB) |
| `bundles/datastar-core.js` | 10,254 B (10 KB) | ~4 KB |
| `bundles/datastar-aliased.js` | 34,140 B | ~13 KB |

33 KB raw / 13 KB gzipped for the entire frontend framework. For comparison that is roughly a
twentieth of a minimal React + ReactDOM bundle. Embedding it costs nothing.

**Which bundle:** `datastar.js`. `datastar-core.js` drops the plugins (you lose `data-on`,
`data-bind`, everything) and is for building a custom bundle. `datastar-aliased.js` is byte-identical
except it uses the **`data-star-*`** prefix instead of `data-*` — I diffed them; the only changes are
the literal `star-` in the prefix builder and a `startsWith("star-")` guard. Use it only if `data-*`
collides with other tooling. MediKube has no such collision; use the plain bundle.

It is an **ES module** (it ends in an `export { ... }` block), so it must be loaded with
`type="module"`.

### Vendoring

```yaml
  vendor:datastar:
    vars:
      DATASTAR_VERSION: 1.0.2
    status: ['test -f internal/ui/assets/dist/datastar.js']
    cmds:
      - mkdir -p internal/ui/assets/dist
      - curl -sSfL -o internal/ui/assets/dist/datastar.js
        "https://cdn.jsdelivr.net/gh/starfederation/datastar@{{.DATASTAR_VERSION}}/bundles/datastar.js"
      - |
        echo "$(head -c 40 internal/ui/assets/dist/datastar.js)" | grep -q "Datastar v{{.DATASTAR_VERSION}}" \
          || { echo "version banner mismatch"; exit 1; }
```

The banner check is worth having: the first line of the file is literally `// Datastar v1.0.2`, so a
one-line grep pins the vendored version against silent CDN drift. Commit the file, record its
SHA-256 in the repo, and re-verify in CI.

### Embedding and serving

```go
// internal/ui/assets/assets.go
package assets

import "embed"

//go:embed dist
var FS embed.FS
```

```go
// internal/httpapi/static.go
package httpapi

import (
	"io/fs"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/ui/assets"
)

func BindAssets(se *core.ServeEvent) error {
	sub, err := fs.Sub(assets.FS, "dist")
	if err != nil {
		return err
	}
	se.Router.GET("/assets/{path...}", apis.Static(sub, false)).
		BindFunc(func(e *core.RequestEvent) error {
			// Content-addressed URLs would be better; failing that, a long max-age
			// plus a version query string on the <script>/<link> is fine.
			e.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			return e.Next()
		}).
		Bind(apis.Gzip()) // safe here: static assets only, never streams (see §5)
	return nil
}
```

`apis.Static` already sets `SkipSuccessActivityLog` internally, so asset requests won't flood the
logs. Verified at `apis/base.go:114-121`.

**Layout head:**

```templ
templ head(title string) {
	<head>
		<meta charset="utf-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1"/>
		<title>{ title } &middot; MediKube</title>
		<link rel="stylesheet" href="/assets/app.css?v=1"/>
		<script type="module" src="/assets/datastar.js?v=1.0.2" defer></script>
	</head>
}
```

---

## 9. Accessibility and the Playwright gate

### Datastar's error catalogue — what the console gate will actually catch

I extracted every error identifier from the v1.0.2 bundle. Datastar throws an `Error` whose message
carries a link to `https://data-star.dev/errors/<snake_name>`, and for expression failures it also
`console.error`s the underlying exception. The complete list:

```
BindPropNameMissing            KeyAndValueProvided            PatchElementsInvalidNamespace
ComputedExpectedFunction       KeyNotAllowed                  PatchElementsNoTargetsFound
ExecuteExpression              KeyOrValueRequired             PatchSignalsExpectedSignals
FetchExpectedTextEventStream   KeyRequired                    UndefinedAction
FetchFailed                    PatchElementsExpectedSelector  ValueNotAllowed
FetchFormNotFound              PatchElementsInvalidMode       ValueRequired
FetchInvalidContentType        InvalidBindAdapter             InvalidDataUri
GenerateExpression             InvalidFileResultType
```

The three that will fail your gate in practice:

1. **`PatchElementsNoTargetsFound`** — you patched an `id` that isn't in the DOM. This is *the*
   Datastar bug. Causes: the target was inside a region you replaced earlier; a typo'd id; a
   race where the stream emits before the initial page render lands. **The stable-ID discipline in
   §4 exists to make this structurally impossible.**
2. **`ExecuteExpression` / `GenerateExpression`** — a `data-*` expression referenced a signal that
   was never declared, or threw. Fix: declare every signal a page uses in one `data-signals` on the
   page root, so there is a single manifest.
3. **`FetchExpectedTextEventStream` / `FetchInvalidContentType`** — the handler returned the wrong
   `Content-Type`. Usually a Go error path that fell through to PB's JSON error response. Make your
   error handling explicit (below).

### Error handling that doesn't spew to console

When a Go handler returns `e.BadRequestError(...)`, PocketBase writes a JSON error body. Datastar
sees `application/json` and treats it as a **signal patch** — so PB's `{"message":...,"data":...}`
gets merged into your signals. Confusing, and it silently pollutes state.

Handle it deliberately. After `NewSSE` has been called the response is already committed, so you
cannot switch to an error response — you must patch an error into the UI:

```go
sse := datastar.NewSSE(e.Response, e.Request)
if err != nil {
	h.log.Error().Err(err).Msg("failed to save medication")
	// Patch a visible, announced error. Do NOT return err - the response is committed.
	return sse.PatchElementTempl(ui.ErrorBanner("Could not save. Please try again."),
		datastar.WithSelectorID(ui.ErrorBannerID))
}
```

And a client-side global net, using the `datastar-fetch` DOM event (§2):

```templ
<div
	id="fetch-error"
	role="alert"
	aria-live="assertive"
	data-on:datastar-fetch__document="$_netError = (evt.detail.type === 'error')"
	data-show="$_netError"
	class="sr-only"
>Connection problem. Retrying&hellip;</div>
```

(`$_netError` is underscore-prefixed, so it never gets sent to the server.)

### Landmark structure for stable Playwright selectors

Give every page the same skeleton, and make the landmarks **outside** every patch target so they can
never be morphed away:

```templ
templ Layout(title string, nav NavState) {
	<!DOCTYPE html>
	<html lang="en">
		@head(title)
		<body class="min-h-screen bg-gray-50 text-gray-900" data-signals={ templ.JSONString(nav.Signals()) }>
			<a href="#main" class="sr-only focus:not-sr-only">Skip to main content</a>

			<header id="app-header">
				<nav id="primary-nav" aria-label="Primary">
					@PatientSwitcher(nav.Patients, nav.CurrentPatientID)
				</nav>
			</header>

			<!-- #main is a landmark AND the stable outer boundary. Patch INTO it, never over it. -->
			<main id="main" tabindex="-1">
				{ children... }
			</main>

			<div id="error-banner" role="alert" aria-live="assertive"></div>
			<div id="live-region" role="status" aria-live="polite" class="sr-only"></div>

			<footer id="app-footer">...</footer>
		</body>
	</html>
}
```

Playwright then asserts on roles, not CSS:

```ts
// tests/smoke.spec.ts
const ROUTES = ['/', '/patients', '/medications', '/lab-results', '/conditions', '/settings'];

for (const route of ROUTES) {
  for (const viewport of [{width:1280,height:800}, {width:390,height:844}]) {
    test(`${route} @ ${viewport.width}`, async ({ page }) => {
      const consoleErrors: string[] = [];
      const failedRequests: string[] = [];

      page.on('console', m => { if (m.type() === 'error') consoleErrors.push(m.text()); });
      page.on('pageerror', e => consoleErrors.push(e.message));
      page.on('requestfailed', r => failedRequests.push(`${r.method()} ${r.url()}`));
      page.on('response', r => { if (r.status() >= 400) failedRequests.push(`${r.status()} ${r.url()}`); });

      await page.setViewportSize(viewport);
      const resp = await page.goto(route, { waitUntil: 'networkidle' });
      expect(resp?.status()).toBe(200);

      // Landmarks - role-based, stable across restyles.
      await expect(page.getByRole('banner')).toBeVisible();
      await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
      await expect(page.getByRole('main')).toBeVisible();
      await expect(page.getByRole('contentinfo')).toBeVisible();

      // Datastar must have booted and processed the tree.
      await expect(page.locator('body')).toHaveAttribute('data-signals');

      expect(consoleErrors, `console errors on ${route}`).toEqual([]);
      expect(failedRequests, `failed requests on ${route}`).toEqual([]);
    });
  }
}
```

**Waiting for a patch to land.** `networkidle` does not work for SSE — the stream never goes idle.
Assert on the DOM instead, which is what Playwright's auto-waiting is for:

```ts
await page.getByLabel('Dose').fill('500mg');
await page.getByRole('button', { name: 'Save' }).click();
await expect(page.getByRole('listitem').filter({ hasText: 'Metformin' })).toBeVisible();
```

For SSE pages, scope network-idle waits to the initial document, or use
`page.waitForResponse(r => r.url().includes('/api/v1/medications'))`.

### A11y patterns specific to Datastar

| Concern | Pattern |
|---|---|
| Loading state announced | `data-indicator:saving` + `data-attr:aria-busy="$saving"` on the region |
| Validation errors announced | error `<p role="alert">` bound with `data-text`; error text arrives as a **signal**, so the element is never re-created and screen readers announce the change |
| Don't disable-and-lose-focus | `data-attr:disabled="$saving"` on submit is fine (focus stays); never `data-show` away the focused element |
| Patched content announced | patch into `#live-region` (`role="status" aria-live="polite"`) for "3 results added" style messages |
| Focus after a patch | `outer`/`inner` **morph** preserves focus if the element identity survives. `replace` does not. **Never use `WithModeReplace()` on anything containing an input** |
| `data-show` and screen readers | it sets `display:none`, which correctly removes content from the a11y tree |
| FOUC on hidden branches | ship `style="display:none"` inline on elements whose initial `data-show` is false |

### CSP — the one genuine footgun

MediKube should set a strict CSP (PB only applies its `defaultCSP` to `/_`). But note how the SDK
implements `ExecuteScript`:

```go
sb.WriteString("<script")
// ... attributes ...
sb.WriteString(` data-effect="el.remove()"`)
sb.WriteString(">")
sb.WriteString(scriptContents)
sb.WriteString("</script>")
return sse.PatchElements(sb.String(), WithSelector("body"), WithModeAppend())
```

It appends an **inline `<script>`**. So under `script-src 'self'` (no `'unsafe-inline'`), these all
silently fail and log a CSP violation to the console — which **fails your zero-console-error gate**:

`ExecuteScript`, `ConsoleLog`, `ConsoleError`, `Redirect`, `Redirectf`, `DispatchCustomEvent`,
`ReplaceURL`, `ReplaceURLQuerystring`, `Prefetch`.

**Decision for MediKube: ban that entire family.** They are all avoidable:

- `Redirect` → return a normal `303` from the handler *before* opening the SSE stream; or patch an
  element and let a link/`data-on:click` navigate. Full page navigations are cheap here.
- `ConsoleLog`/`ConsoleError` → zerolog on the server, `role="alert"` banner on the client.
- `ReplaceURL` → server-rendered canonical URLs; MediKube's routes are real pages, not SPA states.
- `DispatchCustomEvent` → patch a signal and use `data-on-signal-patch`.

Banning them keeps `'unsafe-inline'` out of `script-src` permanently, which is the single most
valuable thing the CSP does here — see immediately below for why that matters so much.

### Datastar REQUIRES `script-src 'unsafe-eval'`. This is not negotiable.

I verified this in the bundle rather than guessing. Datastar's expression compiler is literally the
`Function` constructor:

```js
// datastar.js v1.0.2 - the expression compiler
let l = Function("el", "$", "__action", "evt", ...n, s);
```

and its signal parser falls back to it too, so that `data-signals="{foo: 1}"` (non-strict JSON) works:

```js
ce = (e, t = {}) => { try { ... JSON.parse(e) } catch { return Function(`return (${e})`)() } }
```

The `Function` constructor is governed by CSP's `'unsafe-eval'`. **Without it, every single
`data-*` expression on every page throws.** The app does not partially degrade — it is completely
non-functional, and it fills the console with CSP violations, failing the zero-console-error gate on
every route.

So the CSP is:

```go
e.Response.Header().Set("Content-Security-Policy",
	"default-src 'self'; "+
	"script-src 'self' 'unsafe-eval'; "+   // REQUIRED: Datastar compiles expressions via Function()
	"style-src 'self'; "+
	"img-src 'self' data: blob:; "+
	"connect-src 'self'; "+
	"form-action 'self'; "+
	"frame-ancestors 'none'; "+
	"base-uri 'none'; "+
	"object-src 'none'")
```

**This is a real, permanent security trade-off and the spec must record it as an accepted risk, not
bury it.** The honest framing:

- `'unsafe-eval'` means that *if* an attacker achieves script injection, CSP will not stop them from
  evaluating arbitrary strings. It does **not** by itself create an injection vector.
- What actually bounds the risk is that **every Datastar expression is server-authored templ
  output.** Expression text never comes from user input. `data-text="$foo"` renders values as
  `textContent` (escaped), and templ escapes attribute values by default.
- The compensating controls that matter more here: `'unsafe-inline'` is **not** granted, so injected
  `<script>` tags still don't run; `connect-src 'self'` and `form-action 'self'` block exfiltration
  to an attacker-controlled origin; `object-src 'none'` and `base-uri 'none'` close the classic
  bypasses.

**Two rules that must be lint-enforced, because they are what keeps this safe:**

1. **Never interpolate user data into a `data-*` expression.** Not `data-text`, not `data-on:*`, not
   `data-signals`. User data reaches the client as a *signal value* (`MarshalAndPatchSignals`) or as
   escaped text content — never as expression source. A grep-based CI check for templ interpolation
   `{ ... }` inside a `data-on:` or `data-computed:` attribute value is cheap and worth having.
2. **Keep `'unsafe-inline'` out of `script-src` permanently.** That is what the `ExecuteScript` ban
   above buys you — do not trade it back for the convenience of `sse.Redirect()`.

If `'unsafe-eval'` is unacceptable to a stakeholder, the honest answer is that **Datastar is the
wrong choice**, not that it can be configured around. Surface that now, while the decision is still
cheap to revisit.

---

## 10. Progressive enhancement / no-JS: state it plainly

**MediKube's UI will NOT work with JavaScript disabled. Datastar does not degrade.**

No hedging on this. The reasons are structural, not incidental:

1. Every interaction is a `data-on:*` attribute evaluated by the Datastar runtime. With JS off,
   nothing is bound and nothing happens.
2. `data-bind` is the only thing populating signals from inputs. With JS off there are no signals,
   so a form submit sends **nothing** — `@post` sends the signal JSON as the request body, and
   there is no signal JSON.
3. The `on` plugin calls `preventDefault()` on form submit and takes over. With JS off, the browser
   would do a native submit — but to an endpoint that expects a Datastar signals JSON body, not
   `application/x-www-form-urlencoded`.
4. Server responses are `datastar-patch-elements` events or bare HTML fragments. With JS off there
   is nothing to apply them.
5. `data-show` needs `style="display:none"` seeded server-side to avoid FOUC — with JS off, whatever
   you seeded is what the user is stuck with, permanently.

This is inherent to the hypermedia-runtime model, exactly as it is for HTMX and Alpine. It is not a
gap you can close with effort; closing it means writing a second, form-post-based application.

### What the spec should say

> **MediKube requires JavaScript.** The UI is server-rendered HTML enhanced by the Datastar runtime
> (33 KB, embedded and served from the application origin; no CDN, no third-party requests). It
> targets current evergreen browsers. There is no no-JS fallback and none is planned: the interaction
> model is server-driven DOM patching, which has no meaningful degraded mode.
>
> This is acceptable for MediKube's deployment context: a self-hosted personal medical records
> application accessed by authenticated users on their own devices. It is not a public content site;
> there is no SEO requirement, no crawler requirement, and no anonymous-reader requirement.
>
> **Required mitigations, since JS is mandatory:**
>
> 1. A `<noscript>` block on every page stating plainly that JavaScript is required, styled to be
>    readable without the stylesheet.
> 2. All content is real server-rendered HTML in the initial response — never an empty shell
>    hydrated by a client fetch. A page must be readable the instant the HTML lands, before
>    `datastar.js` executes. **This is what makes the app usable on a slow connection and it is what
>    makes the Playwright landmark assertions meaningful.**
> 3. Semantic HTML and ARIA landmarks throughout (§9), so assistive technology works fully. "Requires
>    JS" must not become "inaccessible".
> 4. Every destructive action is a real `<form>` with a real `action` and `method`, so that if the
>    runtime fails to load, the browser's native submit still hits a handler that understands
>    `application/x-www-form-urlencoded`. This is cheap insurance for the paths where silent failure
>    is worst (delete a record, revoke a share).

Point 4 is worth doing even though full no-JS support isn't a goal: it costs one
`r.ParseForm()` fallback branch in a handful of handlers and it converts "silent nothing happens"
into "it worked" for the destructive paths.

```go
// Accept both shapes on the destructive endpoints.
func (h *MedicationHandler) Delete(e *core.RequestEvent) error {
	var in struct{ ID string `json:"id"` }
	if err := datastar.ReadSignals(e.Request, &in); err != nil || in.ID == "" {
		// No signals: native form post fallback.
		if err := e.Request.ParseForm(); err == nil {
			in.ID = e.Request.PostFormValue("id")
		}
	}
	if in.ID == "" {
		return e.BadRequestError("Missing medication id", nil)
	}
	if err := h.svc.Delete(e.Request.Context(), in.ID); err != nil {
		return e.InternalServerError("Failed to delete", err)
	}
	// Datastar client: patch it away. Native form post: redirect.
	if e.Request.Header.Get("Accept") == "text/event-stream" || isDatastarRequest(e.Request) {
		sse := datastar.NewSSE(e.Response, e.Request)
		return sse.RemoveElementByID(ui.MedicationRowID(in.ID))
	}
	return e.Redirect(303, "/medications")
}
```

---

## 11. Spec rules distilled

1. Pin `github.com/starfederation/datastar-go v1.2.2`; vendor `datastar.js` **v1.0.2**. They are
   different version numbers on purpose. Never use `github.com/starfederation/datastar/sdk/go`.
2. **Free Datastar attributes only.** No `data-persist`, `data-query-string`, `data-replace-url`,
   `data-scroll-into-view`, `data-view-transition`, `data-custom-validity`, `data-animate`,
   `data-match-media`, `data-on-raf`, `data-on-resize`, `@clipboard`, `@fit` — all Pro. Lint it.
3. Colon syntax everywhere: `data-on:click`, not `data-on-click`. `data-init`, not `data-on-load`.
   Hyphenated only for the genuinely-separate plugins `data-on-intersect`, `data-on-interval`,
   `data-on-signal-patch`.
4. `_`-prefix every client-local signal. They are excluded from requests by default. No PHI in
   signals that don't need to travel. Never ship `data-json-signals`.
5. **Prefer the `text/html` fast path over SSE.** SSE only for genuinely streaming endpoints.
6. Every SSE handler goes through `newStream(e)`: clear the write deadline (PB's `WriteTimeout` is
   5 minutes), set `X-Accel-Buffering: no`, `Cache-Control: no-store`, and
   `Bind(apis.SkipSuccessActivityLog())`.
7. **Never bind `apis.Gzip()` at the router root.** Assets group only. Never use
   `datastar.WithCompression`.
8. `ReadSignals` **before** `NewSSE`. Always. And validate, because a missing `?datastar=` on GET
   returns `nil` with a zeroed struct.
9. One `id` per patch target, generated by a shared helper in `internal/ui/ids.go`, used by both the
   templ component and the Go patch call. No raw selector strings in handlers.
10. Validation errors travel as **signals**, not re-rendered form HTML. Preserves focus.
11. Never `WithModeReplace()` on anything containing an input.
12. PB realtime and Datastar are **not** protocol-compatible. Bridge via hooks → hub → templ →
    `datastar-patch-elements`. Fan out IDs, re-fetch and re-authorise per subscriber, per event.
    Non-blocking publish. Single-instance only — write that constraint down.
13. Commit `*_templ.go` and `dist/app.css`; gitignore `*_templ.txt` and `.cache/`; gate with
    `templ generate -check`, `templ fmt -fail`, and a `git diff --exit-code` on the built CSS.
14. Ban the `ExecuteScript` family (`Redirect`, `ConsoleLog`, `DispatchCustomEvent`, `ReplaceURL`,
    `Prefetch`) — they inject inline `<script>` and will trip a strict CSP and the console gate.
15. Datastar compiles every expression with the `Function` constructor, so
    **`script-src 'self' 'unsafe-eval'` is mandatory** — without it the app is entirely
    non-functional. Record it as an accepted risk with its compensating controls (§9), keep
    `'unsafe-inline'` out permanently, and lint that no user data is ever interpolated into a
    `data-*` expression.
16. Tailwind class names must appear as **whole literals**. No string concatenation into `class`.
17. State plainly in the spec: **JavaScript is required**, with the four mitigations in §10.
