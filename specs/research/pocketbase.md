# PocketBase v0.40.1 as an embedded Go framework — MediGo technical dossier

**Method note.** Everything below was read directly out of the v0.40.1 source in the local
module cache (`~/go/pkg/mod/github.com/pocketbase/pocketbase@v0.40.1`), not from the website.
Where the published docs disagree with the source, **the source wins** and I say so. No Go
toolchain is installed in this sandbox, so nothing here was compile-verified; signatures are
transcribed from the actual v0.40.1 declarations.

**Version-note convention.** Anything marked `⚠️ v0.23 CHANGE` is where pre-v0.23 material
you find on the internet (StackOverflow, blog posts, LLM training data) is actively wrong.
v0.23 ripped out `echo` and rewrote the router, merged `daos`/`models` into `core`, and turned
admins into `_superusers` auth records.

---

## 0. Hard version facts (verify these before anything else)

From `go.mod` of v0.40.1:

```
module github.com/pocketbase/pocketbase

go 1.27

require (
	github.com/spf13/cobra v1.10.2
	github.com/spf13/cast v1.10.0
	github.com/pocketbase/dbx v1.12.0
	github.com/pocketbase/ozzo-validation/v4 v4.3.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	modernc.org/sqlite v1.57.0
	...
)
require (
	github.com/stretchr/testify v1.8.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
)
```

- **Cobra is `v1.10.2`** (question 2 answered). `pflag v1.0.10` comes with it.
- **testify is already an (indirect) dependency at v1.8.0.** MediGo will promote it to direct
  and almost certainly to a newer version — that's a normal MVS upgrade, no conflict.
- **`go 1.27`.**

### 🚨 COLLISION #0 — Go 1.26.5 cannot build PocketBase v0.40.1

This is the one that stops the project on day one, so it goes first.

CHANGELOG for v0.40.0 says verbatim:

> Bumped the min Go version to 1.27.0 and migrated to the new `encoding/json/v2` package.
> ⚠️ Please note that Go 1.27.0 retrofitted `encoding/json` to use the v2 package under the
> hood but unfortunately is not fully backward compatible.

And it is not just a `go.mod` directive you can shrug off — **120 files import
`encoding/json/v2` / `encoding/json/jsontext` unguarded**, with no build tags:

```
$ grep -rl '"encoding/json/v2"' --include=*.go . | wc -l
120
```

including `tools/router/router.go`, `tools/router/event.go` (i.e. `BindBody`), `core/*`,
`tools/auth/*`. There is no fallback path. Under Go 1.26.5 the build fails with
`package encoding/json/v2 is not in std`.

**Recommended resolution:** move the locked toolchain to **Go 1.27.x** and put
`toolchain go1.27.x` in MediGo's `go.mod`. The alternatives are all worse:
- pin PocketBase to v0.39.x (loses v0.40's backup improvements, `Record.GetInt64`, the log
  data-size cap, the `filesystem.NewWriter`/`OnNewWriter`/`OnDelete` hooks, and puts you on a
  branch that upstream will stop patching) — this is the only real fallback if 1.27 is
  immovable;
- vendor and patch 120 files — absurd.

Also note the second half of that changelog warning: Go 1.27 retrofitted the *v1*
`encoding/json` onto v2 and it is **not fully backward compatible**. MediGo's own DTO
marshalling will be running on the retrofitted v1 package. Budget a day for JSON edge cases
(nil-vs-empty slices, `json.RawMessage` handling, duplicate keys, case-insensitive field
matching) across the whole app, not just PocketBase.

> The published docs at pocketbase.io/docs/go-overview still say "Go 1.23+". **That page is
> stale.** Trust `go.mod` and the CHANGELOG.

### Minor version facts worth knowing

- `-tags no_ui` strips the embedded superuser dashboard from the binary (`ui/embed_no_ui.go`
  leaves `ui.DistDirFS` nil and `apis.Serve` then skips registering `/_/{path...}`). MediGo
  *wants* the admin UI, so don't use it — but it exists if you ever want a headless build.
- v0.40.0 changed console-command error propagation: `RunE` errors now reach `app.Start()` and
  the process exits non-zero. Good for MediGo's Taskfile/CI. It's listed upstream as a
  possible breaking change for shell chaining.

---

## 1. Bootstrapping, `core.App`, and the binary lifecycle

### `pocketbase.New()` vs `pocketbase.NewWithConfig()`

`New()` is literally `NewWithConfig(Config{DefaultDev: isUsingGoRun})`. That's the only
difference — it flips dev mode on when it detects `go run`. Everything else is defaults.

```go
type Config struct {
	HideStartBanner bool

	DefaultDev           bool
	DefaultDataDir       string        // default: <exe dir>/pb_data
	DefaultEncryptionEnv string
	DefaultQueryTimeout  time.Duration // default: core.DefaultQueryTimeout

	DataMaxOpenConns int
	DataMaxIdleConns int
	AuxMaxOpenConns  int
	AuxMaxIdleConns  int
	DBConnect        core.DBConnectFunc
}
```

**For MediGo use `NewWithConfig`.** You want `HideStartBanner: true` (the banner is
`fmt.Print`-ed colour output that will pollute structured logs), an explicit `DefaultDataDir`
fed from your `caarlos0/env` config, and `DefaultEncryptionEnv` set so app settings
(SMTP creds, OAuth2 client secrets) are encrypted at rest in `pb_data`.

```go
cfg := config.Load() // caarlos0/env/v11

app := pocketbase.NewWithConfig(pocketbase.Config{
	DefaultDataDir:       cfg.DataDir,
	DefaultDev:           cfg.Dev,
	DefaultEncryptionEnv: "MEDIGO_ENCRYPTION_KEY", // name of the env var, not the value
	HideStartBanner:      true,
	DefaultQueryTimeout:  cfg.QueryTimeout,
})
```

> ⚠️ `DefaultEncryptionEnv` is the **name of an environment variable** whose value must be
> exactly 32 characters. It is not the key itself.

### What `core.App` is

`core.App` is a ~200-method interface in `core/app.go`. It is the entire PocketBase surface:
DB access, model CRUD, collection/record queries, filesystem, cron, mailer, settings,
subscriptions broker, migrations, and every hook accessor.

`core.BaseApp` (`core/base.go`) is the single concrete implementation. `pocketbase.PocketBase`
**embeds the `core.App` interface** and adds the cobra root command:

```go
type PocketBase struct {
	core.App                 // <-- embedded INTERFACE, field name is "App"

	devFlag           bool
	dataDirFlag       string
	encryptionEnvFlag string
	queryTimeout      int
	hideStartBanner   bool

	RootCmd *cobra.Command
}

var _ core.App = (*PocketBase)(nil)
```

That `core.App` is an embedded **interface field named `App`**, not a struct, is the single
most important structural fact in this document. It is what makes the logger workaround in
§13 possible, and it's why `pb` itself satisfies `core.App` and can be handed to your service
constructors.

**MediGo's seam:** define your own narrow interfaces and accept `core.App` only at the
composition root. Do not let `core.App` leak into service signatures — it's a 200-method god
interface and mocking it is hopeless.

```go
// internal/store/store.go
type RecordStore interface {
	FindByID(ctx context.Context, collection, id string) (*core.Record, error)
	Save(ctx context.Context, r *core.Record) error
	InTx(ctx context.Context, fn func(RecordStore) error) error
}
```

### What `app.Start()` does

```go
func (pb *PocketBase) Start() error {
	pb.RootCmd.AddCommand(cmd.NewSuperuserCommand(pb))
	pb.RootCmd.AddCommand(cmd.NewServeCommand(pb, !pb.hideStartBanner))
	return pb.Execute()
}
```

That's the whole thing: register `superuser` + `serve`, then `Execute()`. Note it does **not**
register `migrate` — that comes from the `migratecmd` plugin (§5).

`Execute()` is where the lifecycle actually lives:

```go
func (pb *PocketBase) Execute() error {
	if !pb.skipBootstrap() {
		if err := pb.Bootstrap(); err != nil { return err }
	}

	execCh := make(chan error, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() { execCh <- routine.SafeWrap(pb.RootCmd.Execute)() }()

	var execErr error
	select {
	case <-sigCh:
	case execErr = <-execCh:
	}
	signal.Stop(sigCh)

	event := new(core.TerminateEvent)
	event.App = pb
	return pb.OnTerminate().Trigger(event, func(e *core.TerminateEvent) error {
		return errors.Join(e.App.ResetBootstrapState(), execErr)
	})
}
```

**Lifecycle, in order:**

1. `New`/`NewWithConfig` — constructs `BaseApp`, builds `RootCmd`, eagerly parses the
   persistent flags (`--dir`, `--dev`, `--encryptionEnv`, `--queryTimeout`) so they're
   available before cobra runs. **No DB, no settings, no migrations yet.**
2. You register plugins, custom commands, and hooks.
3. `Start()` → `Execute()` → `Bootstrap()`: opens `data.db` + `auxiliary.db`, runs
   **system** migrations, loads settings, initialises the logger, fires `OnBootstrap`.
4. `RootCmd.Execute()` runs the selected subcommand in a goroutine.
   - `serve` → `apis.Serve()` → `RunAllMigrations()` (app migrations run **here**, not at
     bootstrap) → build router → fire `OnServe` → `http.Server.ListenAndServe`.
5. SIGINT/SIGTERM **or** command completion unblocks the `select`.
6. `OnTerminate` fires, then `ResetBootstrapState()` closes DBs and flushes the log batch.

`skipBootstrap()` returns true for `-h/--help/-v/--version` and for **unknown commands**. A
command you registered is a *known* command, so **your custom subcommands get a fully
bootstrapped app** — DB open, settings loaded, migrations applied. That's what makes
`medigo seed` trivial (§2).

⚠️ **v0.23 CHANGE:** pre-v0.23 code used `app.OnBeforeServe()` and an `*echo.Echo` on the
event. Both are gone. It's `OnServe()` with `e.Router`.

---

## 2. Cobra: custom subcommands

`app.RootCmd` is a plain `*cobra.Command` (cobra **v1.10.2**). Register subcommands with
`AddCommand` any time before `Start()`.

```go
func newSeedCmd(app core.App) *cobra.Command {
	var patients int
	var force bool

	c := &cobra.Command{
		Use:          "seed",
		Short:        "Seed the database with demo clinical data",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// app is already bootstrapped here: DB open, settings loaded, migrations applied
			return seed.Run(cmd.Context(), app, seed.Options{
				Patients: patients,
				Force:    force,
			})
		},
	}

	c.Flags().IntVar(&patients, "patients", 3, "number of demo patients")
	c.Flags().BoolVar(&force, "force", false, "wipe existing demo data first")

	return c
}

func newOpenAPICmd(app core.App) *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:          "openapi",
		Short:        "Generate the OpenAPI 3.1 document from the Go route/DTO registry",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return openapi.Generate(api.Routes(), out)
		},
	}
	c.Flags().StringVar(&out, "out", "docs/openapi.json", "output path")
	return c
}
```

Wiring:

```go
app := pocketbase.NewWithConfig(...)

migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
	TemplateLang: migratecmd.TemplateLangGo,
	Automigrate:  cfg.Dev,
	Dir:          "internal/migrations",
})

app.RootCmd.AddCommand(newSeedCmd(app))
app.RootCmd.AddCommand(newOpenAPICmd(app))

if err := app.Start(); err != nil {   // adds serve + superuser, then Execute()
	log.Fatal(err)
}
```

### How PB's own commands coexist

They're just sibling subcommands on the same root:

| Command | Registered by | Notes |
|---|---|---|
| `serve [domain...]` | `Start()` → `cmd.NewServeCommand` | flags `--http`, `--https`, `--origins` |
| `superuser <sub>` | `Start()` → `cmd.NewSuperuserCommand` | subs: `upsert`, `create`, `update`, `delete`, `otp`, `ips` |
| `migrate <sub>` | `migratecmd.MustRegister` (opt-in) | subs: `up`, `down [n]`, `create <name>`, `collections`, `history-sync` |

Global persistent flags on the root (`--dir`, `--dev`, `--encryptionEnv`, `--queryTimeout`)
apply to every subcommand including yours.

Three gotchas:

1. **`FParseErrWhitelist{UnknownFlags: true}` is set on the root.** Unknown flags are silently
   tolerated at the root level. A typo'd `--paitents` will not error the way you expect.
   Validate inside `RunE`.
2. **The default `completion` command is disabled** and the `help` command is hidden
   (`--help` still works).
3. **`RootCmd.SetErr(&nopWrite{})`** — cobra's own error output is discarded because errors
   are propagated to `Start()`. If you want the error printed, print it yourself in `main`.
   Use `SilenceUsage: true` on your commands so a runtime error doesn't dump the whole usage
   block.

`Execute()` (as opposed to `Start()`) exists if you want to register serve/superuser
yourself or omit them — e.g. a `medigo-worker` binary with no HTTP server.

---

## 3. Routing (the v0.23+ router)

⚠️ **v0.23 CHANGE — this is where most stale material will bite you.** `echo` is gone. The
router (`tools/router`) is a thin generic wrapper over Go 1.22+ `http.ServeMux` pattern
matching. Consequences:

- Path params are `{id}`, **not** `:id`. Wildcard is `{path...}`, **not** `*`.
- Read params with `e.Request.PathValue("id")` (std lib), **not** `c.Param("id")`.
- Handlers are `func(e *core.RequestEvent) error`, **not** `echo.HandlerFunc`.
- Middleware is a handler + explicit `e.Next()`, **not** `echo.MiddlewareFunc`.
- **No trailing-slash-strip middleware any more.** `/api/v1/patients/` ≠ `/api/v1/patients`.

### Registering routes

```go
app.OnServe().BindFunc(func(se *core.ServeEvent) error {
	v1 := se.Router.Group("/api/v1")
	v1.Bind(apis.RequireAuth())          // group-wide

	v1.GET("/patients", h.ListPatients)
	v1.POST("/patients", h.CreatePatient)
	v1.GET("/patients/{patientId}", h.GetPatient)
	v1.PATCH("/patients/{patientId}", h.UpdatePatient)
	v1.DELETE("/patients/{patientId}", h.DeletePatient)

	// nested group with extra middleware
	labs := v1.Group("/patients/{patientId}/labs")
	labs.Bind(mw.RequirePatientAccess(svc))
	labs.GET("", h.ListLabResults)
	labs.POST("", h.CreateLabResult).Bind(apis.BodyLimit(32 << 20))

	// public, no auth — Unbind removes an inherited middleware by id
	v1.GET("/health", h.Health).Unbind(apis.DefaultRequireAuthMiddlewareId)

	return se.Next()   // MANDATORY
})
```

Available verbs on `*router.RouterGroup[T]`: `GET POST PUT PATCH DELETE HEAD OPTIONS SEARCH`,
plus `Any(path, h)` and `Route(method, path, h)`. Each returns `*Route[T]`, so `.Bind(...)` /
`.BindFunc(...)` / `.Unbind(ids...)` chain per-route.

### `core.RequestEvent`

```go
type RequestEvent struct {
	App  App           // the app instance passed to apis.Serve
	Auth *Record       // authenticated record, nil for guests
	router.Event       // embedded: Response, Request, store, response helpers
	// unexported: cachedRequestInfo, mu
}
```

⚠️ **v0.23 CHANGE:** the authenticated user is `e.Auth` (a `*core.Record`).
`e.AuthRecord` — the old echo-era name — **does not exist in v0.40**. Superusers are also
records now: `e.Auth.IsSuperuser()` / `e.HasSuperuserAuth()`. There is no separate "admin"
type; `/api/admins/*` was deleted in v0.23.

From `router.Event` you get:

- `e.Request *http.Request`, `e.Response http.ResponseWriter`
- `e.BindBody(dst any) error` — content-type aware: JSON, `multipart/form-data`,
  `application/x-www-form-urlencoded`, XML. The body is wrapped in a `RereadableReadCloser`,
  so it's safe to bind more than once (that's how `RequestInfo()` and API rules coexist with
  your handler).
- `e.JSON(status, data)`, `e.String`, `e.HTML`, `e.Blob`, `e.Stream`, `e.XML`,
  `e.NoContent(status)`, `e.Redirect(status, url)`, `e.FileFS(fsys, name)`
- `e.Get(key)` / `e.Set(key, v)` / `e.GetAll()` — per-request store
- `e.RemoteIP()`, `e.RealIP()` (trusted-proxy aware), `e.IsTLS()`, `e.SetCookie()`,
  `e.Written()`, `e.Status()`, `e.Flush()`
- `e.FindUploadedFiles(key) ([]*filesystem.File, error)`
- `e.RequestInfo() (*core.RequestInfo, error)` — the `@request.*` view used by API rules

### Errors and status codes

Return an error; the router converts it. `router.ApiError` carries the status.

```go
func (e *Event) Error(status int, message string, errData any) *ApiError
func (e *Event) BadRequestError(message string, errData any) *ApiError      // 400
func (e *Event) UnauthorizedError(message string, errData any) *ApiError    // 401
func (e *Event) ForbiddenError(message string, errData any) *ApiError       // 403
func (e *Event) NotFoundError(message string, errData any) *ApiError        // 404
func (e *Event) TooManyRequestsError(message string, errData any) *ApiError // 429
func (e *Event) InternalServerError(message string, errData any) *ApiError  // 500
```

Package-level equivalents (`apis.NewBadRequestError(...)`, etc. — aliased in
`apis/api_error_aliases.go`) for use outside a handler. `router.ToApiError(err)` normalises
any error; a non-`ApiError` becomes a 500 with a generic message and the original is logged,
**not** leaked to the client. That's the right default for a medical records app.

⚠️ **v0.23 CHANGE:** the JSON error envelope renamed the top-level `code` to `status`:

```json
{ "status": 400, "message": "...", "data": { "field": { "code": "validation_required", "message": "..." } } }
```

### A real handler

```go
package handler

type CreateLabResultRequest struct {
	PatientID  string    `json:"patientId"`
	TestCode   string    `json:"testCode"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	CollectedAt time.Time `json:"collectedAt"`
	Notes      string    `json:"notes"`
}

type LabResultResponse struct {
	ID          string    `json:"id"`
	PatientID   string    `json:"patientId"`
	TestCode    string    `json:"testCode"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	Flag        string    `json:"flag"`
	CollectedAt time.Time `json:"collectedAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type LabHandler struct {
	labs   service.LabService
	logger zerolog.Logger
}

func (h *LabHandler) CreateLabResult(e *core.RequestEvent) error {
	var req CreateLabResultRequest
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Malformed request body.", err)
	}

	if err := req.Validate(); err != nil {
		// ozzo validation.Errors implements SafeErrorItem -> field errors survive to the client
		return e.BadRequestError("Validation failed.", err)
	}

	// e.Auth is the PocketBase-authenticated user record
	actor := authctx.FromRecord(e.Auth)

	out, err := h.labs.Create(e.Request.Context(), actor, service.CreateLabInput{
		PatientID:   req.PatientID,
		TestCode:    req.TestCode,
		Value:       req.Value,
		Unit:        req.Unit,
		CollectedAt: req.CollectedAt,
		Notes:       req.Notes,
	})
	switch {
	case errors.Is(err, service.ErrPatientNotFound):
		return e.NotFoundError("Patient not found.", nil)
	case errors.Is(err, service.ErrForbidden):
		return e.ForbiddenError("You do not have access to this patient.", nil)
	case err != nil:
		return e.InternalServerError("Failed to create lab result.", err) // err is logged, not returned
	}

	return e.JSON(http.StatusCreated, LabResultResponse{
		ID:          out.ID,
		PatientID:   out.PatientID,
		TestCode:    out.TestCode,
		Value:       out.Value,
		Unit:        out.Unit,
		Flag:        out.Flag,
		CollectedAt: out.CollectedAt,
		CreatedAt:   out.CreatedAt,
	})
}
```

Note `e.Request.Context()` — that's your context propagation into services, OTel, and
`SaveWithContext`/`DeleteWithContext`. `apis.Serve` installs a base context that is cancelled
on shutdown.

---

## 4. Middleware

A middleware is a `*hook.Handler[*core.RequestEvent]` — the *same* type as a route handler.
The only distinction is that a middleware calls `e.Next()`.

```go
type Handler[T Resolver] struct {
	Func     func(T) error
	Id       string  // unique; re-Binding the same Id REPLACES the handler
	Priority int     // lower runs first; 0 = registration order
}
```

### Ordering / priority semantics (from `tools/hook/hook.go`)

```go
sort.SliceStable(h.handlers, func(i, j int) bool {
	return h.handlers[i].Priority < h.handlers[j].Priority
})
```

- **Lower `Priority` runs earlier.** Negative numbers run before positive.
- Stable sort → equal priorities preserve registration order.
- Priority `0` (the default via `BindFunc`) means "in registration order", interleaved with
  everything else at 0.
- **Same `Id` = replacement, not addition.** This is deliberate and is how `Unbind` works.

Composition across nesting (`tools/router/router.go` `loadMux`): for each route the final
chain is **parent-group middlewares → own-group middlewares → route middlewares**, with
`excludedMiddlewares` (populated by `Unbind`) checked at every level. So `Unbind` on a child
suppresses an ancestor's middleware for that subtree only.

### Built-in middlewares and their real priorities

`DefaultRateLimitMiddlewarePriority = -1000` is the anchor everything else is expressed
against. Actual values:

| Middleware | Id const | Priority | Registered by default? |
|---|---|---|---|
| `wwwRedirect` | `pbWWWRedirect` | `-99999` | only with TLS domains |
| CORS (`apis.CORS`) | `pbCors` | `-1041` (`activityLogger-1`) | yes, by `apis.Serve` |
| `activityLogger` | `pbActivityLogger` | `-1040` | yes |
| `panicRecover` | `pbPanicRecover` | `-1030` | yes |
| `loadAuthToken` | `pbLoadAuthToken` | `-1020` | yes |
| `securityHeaders` | `pbSecurityHeaders` | `-1010` | yes |
| `superuserIPsWhitelist` | `pbSuperuserIPsWhitelist` | `-1015` | yes |
| `rateLimit` | `pbRateLimit` | `-1000` | yes |
| `BodyLimit` | `pbBodyLimit` | `-990` | yes (`DefaultMaxBodySize`) |
| `RequireAuth` etc. | `pbRequireAuth`, ... | `0` | no — opt-in |
| `Gzip` | `pbGzip` | `0` | no — opt-in |

**The number that matters for MediGo: `loadAuthToken` is `-1020`.** Any middleware of yours
that needs `e.Auth` populated must have `Priority > -1020` (the default `0` is fine).

Public constructors:

```go
apis.RequireAuth(optCollectionNames ...string) *hook.Handler[*core.RequestEvent]
apis.RequireSuperuserAuth() *hook.Handler[*core.RequestEvent]
apis.RequireSuperuserOrOwnerAuth(ownerIdPathParam string) *hook.Handler[*core.RequestEvent]
apis.RequireSameCollectionContextAuth(collectionPathParam string) *hook.Handler[*core.RequestEvent]
apis.RequireGuestOnly() *hook.Handler[*core.RequestEvent]
apis.BodyLimit(limitBytes int64) *hook.Handler[*core.RequestEvent]
apis.Gzip() / apis.GzipWithConfig(apis.GzipConfig{Level, MinLength}) *hook.Handler[...]
apis.CORS(apis.CORSConfig{...}) *hook.Handler[...]
apis.SkipSuccessActivityLog() *hook.Handler[...]
```

`RequireAuth` is simply:

```go
if e.Auth == nil {
	return e.UnauthorizedError("The request requires valid record authorization token.", nil)
}
if len(optCollectionNames) > 0 && !slices.Contains(optCollectionNames, e.Auth.Collection().Name) {
	return e.ForbiddenError("The authorized record is not allowed to perform this action.", nil)
}
return e.Next()
```

`RequireSuperuserAuth()` is `requireAuth(core.CollectionNameSuperusers)` — i.e. it just
checks the auth record belongs to `_superusers`.

**`apis.RequireAuth("users")` is what MediGo wants everywhere on `/api/v1`** — it pins the
token to the `users` collection so a superuser token or some future service-account
collection can't be silently accepted by user-facing endpoints.

### Rate limiting

Rate limiting is **settings-driven, not code-driven**, and **`RateLimits.Enabled` defaults to
`false`** — PocketBase ships it off. Turn it on explicitly in MediGo's bootstrap settings hook;
a self-hosted medical app exposed to the internet wants it on, especially for the auth routes.
`app.Settings().RateLimits` holds `Enabled` plus a list of rules with `Label` (route tag, exact path, or prefix), `MaxRequests`,
`Duration`, `Audience`. Configure it in a bootstrap hook or via the admin UI. The built-in
middleware is registered globally at `-1000` and skipped per-request by `skipRateLimit`.
Collection-scoped endpoints re-bind their own `collectionPathRateLimit` under the *same*
`pbRateLimit` id (replacement semantics).

### Custom middleware

```go
// mw/patient_access.go
const PatientAccessMiddlewareId = "medigoPatientAccess"

func RequirePatientAccess(svc service.PatientService) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       PatientAccessMiddlewareId,
		Priority: 10, // after RequireAuth (0), well after loadAuthToken (-1020)
		Func: func(e *core.RequestEvent) error {
			patientID := e.Request.PathValue("patientId")
			if patientID == "" {
				return e.BadRequestError("Missing patient id.", nil)
			}

			ok, err := svc.CanAccess(e.Request.Context(), e.Auth.Id, patientID)
			if err != nil {
				return e.InternalServerError("Failed to resolve patient access.", err)
			}
			if !ok {
				// 404 not 403: don't confirm the patient exists
				return e.NotFoundError("", nil)
			}

			e.Set("medigo:patientId", patientID)
			return e.Next()
		},
	}
}
```

Two idioms worth adopting:

```go
// wrap any std middleware (Sentry, otelhttp, prometheus)
se.Router.BindFunc(apis.WrapStdMiddleware(otelhttp.NewMiddleware("medigo")))

// wrap any std handler (promhttp)
se.Router.GET("/metrics", apis.WrapStdHandler(promhttp.Handler())).
	Bind(apis.RequireSuperuserAuth())
```

`WrapStdMiddleware` correctly re-assigns `e.Response`/`e.Request` inside the wrapped handler
so downstream PB handlers see the std middleware's wrapped writer. That's the supported OTel
/ Sentry integration point.

**Forgetting `e.Next()` silently truncates the chain** — the request returns 200 with an
empty body and no handler ever runs. There is a bootstrap-time warning for the *app* hooks
("OnBootstrap hook didn't fail but the app is still not bootstrapped - maybe missing
e.Next()?") but nothing equivalent for routes. Make it a `.golangci.yml` custom lint or a
review checklist item.

---

## 5. Collections & schema-as-code

### Where schema lives

Collections are rows in `_collections` in `data.db`. "Schema as code" means **Go migrations**
that build `*core.Collection` values and `app.Save()` them.

Register the plugin (this is what adds the `migrate` command):

```go
migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
	TemplateLang: migratecmd.TemplateLangGo, // default is Go; JS only matters with jsvm
	Automigrate:  cfg.Dev,                   // dev only — see below
	Dir:          "internal/migrations",     // default: <dataDir>/../migrations
})
```

Migrations are registered by **package `init()`**, so the package must be blank-imported:

```go
import _ "medigo/internal/migrations"
```

```go
// migrations/1737000000_create_patients.go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		// ---- up ----
		c := core.NewBaseCollection("patients")

		c.Fields.Add(
			&core.TextField{
				Name:     "full_name",
				Required: true,
				Max:      200,
			},
			&core.DateField{
				Name:     "birth_date",
				Required: true,
			},
			&core.SelectField{
				Name:      "biological_sex",
				Values:    []string{"female", "male", "intersex", "unknown"},
				MaxSelect: 1,
				Required:  true,
			},
			&core.NumberField{
				Name:    "height_cm",
				Min:     types.Pointer(0.0),
				Max:     types.Pointer(300.0),
				OnlyInt: false,
			},
			&core.BoolField{
				Name: "is_deceased",
			},
			&core.JSONField{
				Name:    "metadata",
				MaxSize: 64_000,
			},
			&core.EditorField{
				Name:        "clinical_summary",
				MaxSize:     200_000,
				ConvertURLs: false,
			},
			&core.FileField{
				Name:      "avatar",
				MaxSelect: 1,
				MaxSize:   5 << 20,
				MimeTypes: []string{"image/jpeg", "image/png", "image/webp"},
				Thumbs:    []string{"64x64", "256x256f"},
				Protected: true,        // see §9 — ALWAYS true for MediGo
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)

		// relation to the users auth collection
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		c.Fields.Add(&core.RelationField{
			Name:          "owner",
			CollectionId:  users.Id,
			Required:      true,
			MaxSelect:     1,     // single relation
			CascadeDelete: true,  // delete the patient when the owner user is deleted
		})

		// UNIQUENESS IS AN INDEX, NOT A FIELD FLAG
		c.AddIndex("idx_patients_owner_name", true, "owner, full_name", "")
		c.AddIndex("idx_patients_owner", false, "owner", "")
		// partial index example (the 4th arg is a WHERE clause)
		c.AddIndex("idx_patients_alive", false, "owner", "is_deceased = false")

		// API rules — nil means SUPERUSER ONLY (see §7)
		c.ListRule = nil
		c.ViewRule = nil
		c.CreateRule = nil
		c.UpdateRule = nil
		c.DeleteRule = nil

		return app.Save(c)
	}, func(app core.App) error {
		// ---- down ----
		c, err := app.FindCollectionByNameOrId("patients")
		if err != nil {
			return err
		}
		return app.Delete(c)
	})
}
```

### The exact field structs (v0.40.1, verbatim member lists)

Every field shares `Name, Id, System, Hidden, Presentable, Help` (except `AutodateField`,
which has no `Help`). Type-specific members:

| Type | Struct | Type-specific members |
|---|---|---|
| text | `core.TextField` | `Min int`, `Max int`, `Pattern string`, `AutogeneratePattern string`, `Required bool`, `PrimaryKey bool` |
| number | `core.NumberField` | `Min *float64`, `Max *float64`, `OnlyInt bool`, `Required bool` |
| bool | `core.BoolField` | `Required bool` |
| date | `core.DateField` | `Min types.DateTime`, `Max types.DateTime`, `Required bool` |
| select | `core.SelectField` | `Values []string`, `MaxSelect int`, `Required bool` |
| relation | `core.RelationField` | `CollectionId string`, `CascadeDelete bool`, `MinSelect int`, `MaxSelect int`, `Required bool` |
| file | `core.FileField` | `MaxSize int64`, `MaxSelect int`, `MimeTypes []string`, `Thumbs []string`, `Protected bool`, `Required bool` |
| json | `core.JSONField` | `MaxSize int64`, `Required bool` |
| editor | `core.EditorField` | `MaxSize int64`, `ConvertURLs bool`, `Required bool` |
| autodate | `core.AutodateField` | `OnCreate bool`, `OnUpdate bool` |
| email | `core.EmailField` | `ExceptDomains`, `OnlyDomains`, `Required` |
| url | `core.URLField` | `ExceptDomains`, `OnlyDomains`, `Required` |
| password | `core.PasswordField` | `Min`, `Max`, `Pattern`, `Cost`, `Required` |
| geoPoint | `core.GeoPointField` | `Required bool` |

**⚠️ There is no `Unique` field flag.** Uniqueness is a collection index:

```go
func (m *Collection) AddIndex(name string, unique bool, columnsExpr string, optWhereExpr string)
func (m *Collection) RemoveIndex(name string)
```

`AddIndex` builds a raw `CREATE [UNIQUE] INDEX ... ON ... (...) [WHERE ...]` string and
appends it to `collection.Indexes`. This is a genuinely nice escape hatch: any SQLite index
expression works, including partial and expression indexes. **Case-insensitive unique email
would be `AddIndex("idx_x", true, "LOWER(email)", "")`.**

### Relations: single vs multiple, cascade

- **Single relation:** `MaxSelect: 1`. Stored as a scalar TEXT id. `record.GetString("owner")`.
- **Multiple relation:** `MaxSelect: 0` (unlimited) or `> 1`. Stored as a JSON array.
  `record.GetStringSlice("tags")`.
- `MinSelect` enforces a lower bound (combine with `Required`).
- **`CascadeDelete: true`** means: when the *referenced* record is deleted, delete **this**
  record. For a multi-relation it removes the id from the array; if that leaves it below
  `MinSelect`/empty-and-required, the owning record is deleted too. Get the direction right —
  it's on the field holding the reference, describing what happens to the holder.
- PocketBase blocks deleting a collection that is still referenced by another collection's
  relation field (integrity check in `onCollectionDeleteExecute`). Order your `down`
  migrations accordingly.

### The 5 API rules

`ListRule`, `ViewRule`, `CreateRule`, `UpdateRule`, `DeleteRule` — all `*string`:

- `nil` → **superuser only**
- `types.Pointer("")` → **public, everyone**
- `types.Pointer("owner = @request.auth.id")` → filter expression

`AuthRule` and `ManageRule` (auth collections only) are separate and *not* part of the five.

### `Automigrate`

`migratecmd` binds `OnCollectionCreateRequest` / `OnCollectionUpdateRequest` /
`OnCollectionDeleteRequest` and writes a new migration file whenever you change a collection
**through the admin UI**. Note it hooks the `*Request` variants — it only fires for HTTP-driven
changes, not for `app.Save(collection)` from your own Go code.

**Recommendation: `Automigrate: cfg.Dev` only.** In dev it's a great workflow (click in the
dashboard, get a Go migration file, commit it). In production it means the running binary
tries to write `.go` files into a directory that doesn't exist in the container. Never ship
it enabled.

### `migrate` subcommands

- `migrate up` — apply pending
- `migrate down [n]` — revert last n
- `migrate create <name>` — blank template
- `migrate collections` — snapshot the **current** live collection config into one migration
  (the bootstrap move for a greenfield project: model in the UI, snapshot, commit, then hand-edit)
- `migrate history-sync` — drop `_migrations` rows for deleted files

App migrations also run automatically at the start of `apis.Serve` via `RunAllMigrations()`,
so `medigo serve` self-migrates. System migrations run during `Bootstrap()`.

---

## 6. Records: CRUD, queries, transactions

### Reading

```go
// by id
rec, err := app.FindRecordById("patients", id)
rec, err := app.FindRecordById(collection, id, func(q *dbx.SelectQuery) error {
	q.AndWhere(dbx.HashExp{"owner": userID})   // extra constraint, still one query
	return nil
})

app.FindRecordsByIds("patients", []string{a, b, c})
app.FindAllRecords("patients", dbx.HashExp{"owner": userID})
app.FindFirstRecordByData("users", "email", "a@b.c")
app.CountRecords("patients", dbx.HashExp{"owner": userID})

// filter DSL + params (the same DSL as API rules)
recs, err := app.FindRecordsByFilter(
	"lab_results",
	"patient = {:pid} && collected_at >= {:since} && value > {:threshold}",
	"-collected_at",       // sort; "-" = DESC, comma-separated for multi
	50,                    // limit
	0,                     // offset
	dbx.Params{"pid": patientID, "since": since, "threshold": 0.0},
)
one, err := app.FindFirstRecordByFilter("patients", "owner = {:u}", dbx.Params{"u": uid})

// raw dbx builder for anything the DSL can't express
q := app.RecordQuery("lab_results").
	AndWhere(dbx.HashExp{"patient": patientID}).
	AndWhere(dbx.NewExp("collected_at >= {:d}", dbx.Params{"d": since})).
	OrderBy("collected_at DESC").
	Limit(100)

var records []*core.Record
if err := q.All(&records); err != nil { return err }
```

**Always use `{:param}` placeholders.** The filter DSL string is parsed by `fexpr` and
compiled to SQL; interpolating user input into the filter string is an injection vector.

### Writing

```go
collection, err := app.FindCachedCollectionByNameOrId("patients")
if err != nil { return err }

rec := core.NewRecord(collection)
rec.Set("full_name", "Jane Doe")
rec.Set("birth_date", types.NowDateTime())
rec.Set("owner", userID)
rec.Set("metadata", map[string]any{"source": "manual"})

if err := app.Save(rec); err != nil { return err }   // validates + fires hooks
```

App-level model methods (from `core.App`):

```go
Save(model Model) error
SaveWithContext(ctx context.Context, model Model) error
SaveNoValidate(model Model) error                    // skips validation, still fires hooks
SaveNoValidateWithContext(ctx, model) error
Delete(model Model) error
DeleteWithContext(ctx, model) error
Validate(model Model) error
ValidateWithContext(ctx, model) error
UnsafeWithoutHooks() App                             // same app, hooks disabled — use sparingly
```

`Save` is create-or-update based on `model.IsNew()`. `SaveWithContext` is what you want in a
request handler so a client disconnect cancels the write.

Record accessors: `Get`, `GetString`, `GetBool`, `GetInt`, **`GetInt64`** (new in v0.40),
`GetFloat`, `GetDateTime`, `GetStringSlice`, `GetGeoPoint`, `UnmarshalJSONField(key, &dst)`,
`Set`, `SetRaw`, `Load(map)`, `Fresh()`, `Clone()`, `Original()`.

> `number` fields are float64 in SQLite/JSON. `GetInt64` exists but the changelog explicitly
> warns the serialisable max safe integer is ~2^53-1. **Don't store medical record IDs or
> anything needing full int64 range in a `number` field** — use `text`.

### Expanding relations

```go
errs := app.ExpandRecord(patient, []string{"owner", "tags", "labs.test_catalog"}, nil)
if len(errs) > 0 { /* map[string]error, per-path */ }

owner := patient.ExpandedOne("owner")     // *core.Record
tags  := patient.ExpandedAll("tags")      // []*core.Record

app.ExpandRecords(patients, []string{"owner"}, nil)  // batched, N+1-safe
```

The third arg is an `ExpandFetchFunc` — pass a custom one to apply access rules during
expansion (that's what the built-in API does). Passing `nil` **bypasses access checks**, which
is correct for internal service code and dangerous if you echo the result straight to a
client. Since MediGo hand-writes every response DTO, `nil` + explicit authz in the service
layer is the right call.

### Transactions

```go
func (app *BaseApp) RunInTransaction(fn func(txApp App) error) error
func (app *BaseApp) AuxRunInTransaction(fn func(txApp App) error) error
```

Semantics, straight from `core/db_tx.go`:

- **`txApp` is a shallow clone of the app** with `concurrentDB`/`nonconcurrentDB` swapped for
  the `*dbx.Tx`. It is a `*core.BaseApp`, not your wrapper type (see §13).
- **Nesting is safe but only if you use the callback's `txApp`.** If the receiver is already
  a `*dbx.Tx`, `runInTransaction` just calls `fn(app)` and joins the outer transaction.
  Using the *outer* `app` inside the callback silently escapes the transaction — that's the
  #1 PocketBase transaction bug.
- Returning an error rolls back. Returning nil commits.
- **After-commit callbacks:** `txApp.TxInfo().OnComplete(func(txErr error) error { ... })`.
  These run after the transaction ends (success or rollback) and receive the tx error. Errors
  from them are `errors.Join`-ed into the result. This is where side effects belong — email,
  Datastar SSE broadcast, audit shipping, Sentry breadcrumbs — because doing them inside the
  tx means they fire on rollback too.
- SQLite is single-writer. Keep transactions short. Never do HTTP calls inside one.

```go
err := app.RunInTransaction(func(txApp core.App) error {
	patient, err := txApp.FindRecordById("patients", patientID)   // txApp, NOT app
	if err != nil { return err }

	patient.Set("last_visit", types.NowDateTime())
	if err := txApp.Save(patient); err != nil { return err }

	visit := core.NewRecord(visitsCollection)
	visit.Set("patient", patientID)
	visit.Set("notes", notes)
	if err := txApp.Save(visit); err != nil { return err }

	txApp.TxInfo().OnComplete(func(txErr error) error {
		if txErr == nil {
			sse.BroadcastPatientUpdated(patientID)
		}
		return nil
	})
	return nil
})
```

### Typed models: `core.RecordProxy`

v0.23 added a proxy mechanism that MediGo should use to keep `record.Get("...")` stringly-typed
access out of the service layer:

```go
type Patient struct {
	core.BaseRecordProxy
}

func (p *Patient) FullName() string          { return p.GetString("full_name") }
func (p *Patient) SetFullName(v string)      { p.Set("full_name", v) }
func (p *Patient) BirthDate() types.DateTime { return p.GetDateTime("birth_date") }
func (p *Patient) OwnerID() string           { return p.GetString("owner") }

// usable directly as a scan target
var patients []*Patient
err := app.RecordQuery("patients").
	AndWhere(dbx.HashExp{"owner": uid}).
	All(&patients)
```

`BaseRecordProxy` embeds `*core.Record`, so a proxy is still a `core.Model` and works with
`app.Save`/`app.Delete`. This is the clean seam between PocketBase's dynamic records and
MediGo's typed domain — build one proxy per collection, and let the mappers to/from DTOs live
next to them.

---

## 7. 🔒 Locking down the auto-generated `/api/collections/*` API

**Short answer: yes, cleanly — but only via API rules, and it has three sharp edges that
directly hit MediGo's other locked decisions.**

### First, know what actually lives under `/api/collections/`

It is **not** one thing. `apis.NewRouter` registers three distinct families under that prefix:

```go
// apis/collection.go — collection MANAGEMENT (schema), already superuser-only
rg.Group("/collections").Bind(RequireSuperuserAuth())
    GET/POST ""  •  GET/PATCH/DELETE "/{collection}"  •  DELETE "/{collection}/truncate"
    PUT "/import"  •  GET "/meta/scaffolds"  •  GET "/meta/oauth2-providers"

// apis/record_crud.go — the record CRUD you want gone
rg.Group("/collections/{collection}/records")
    GET ""  •  GET "/{id}"  •  POST ""  •  PATCH "/{id}"  •  DELETE "/{id}"

// apis/record_auth.go — AUTH, which MediGo explicitly wants (PB native auth)
rg.Group("/collections/{collection}")
    GET  "/auth-methods"
    POST "/auth-refresh" | "/auth-with-password" | "/auth-with-oauth2"
    POST "/request-otp" | "/auth-with-otp"
    POST "/request-password-reset" | "/confirm-password-reset"
    POST "/request-verification" | "/confirm-verification"
    POST "/request-email-change"  | "/confirm-email-change"
    POST "/impersonate/{id}"   (superuser only)
```

**So "404 the whole `/api/collections/` prefix" is the wrong instinct** — it would kill PB
native auth and the admin dashboard in one stroke.

### Mechanism A — nil API rules (the correct primary mechanism)

Set all five rules to `nil` on every collection. The check is explicit in `apis/record_crud.go`:

```go
if collection.ListRule == nil && !requestInfo.HasSuperuserAuth() {
	return e.ForbiddenError("Only superusers can perform this action.", nil)
}
```

and in `core.CanAccessRecord`:

```go
if requestInfo.HasSuperuserAuth() { return true, nil }   // superusers bypass everything
if accessRule == nil              { return false, nil }  // nil => superuser only
if *accessRule == ""              { return true, nil }   // "" => public
```

Properties:

- ✅ **Public CRUD is dead.** Non-superuser requests get `403 {"status":403,"message":"Only
  superusers can perform this action."}`.
- ✅ **The admin dashboard keeps working** — it authenticates as a `_superusers` record, and
  superusers bypass rules entirely. You keep the admin UI, which is a locked MediGo decision.
- ✅ **Your Go code is completely unaffected.** `app.FindRecordById`, `app.Save`,
  `app.RecordQuery` never consult API rules. Rules are evaluated only in the `apis` HTTP
  layer via `RequestInfo`. Internal use is untouched. This is the key property that makes the
  whole MediGo design viable.
- ✅ Enforced in one place per collection, in a committed migration, reviewable in a diff.
- ⚠️ 403 confirms the collection exists. If you want opacity, add Mechanism B.

Belt-and-braces: assert it at boot so a dashboard fat-finger can't silently open a hole.

```go
app.OnServe().BindFunc(func(se *core.ServeEvent) error {
	cols, err := se.App.FindAllCollections(core.CollectionTypeBase, core.CollectionTypeView)
	if err != nil { return err }

	for _, c := range cols {
		if c.System { continue }
		for name, rule := range map[string]*string{
			"list": c.ListRule, "view": c.ViewRule, "create": c.CreateRule,
			"update": c.UpdateRule, "delete": c.DeleteRule,
		} {
			if rule != nil {
				return fmt.Errorf(
					"collection %q has a non-nil %s rule (%q); MediGo requires nil (superuser-only)",
					c.Name, name, *rule)
			}
		}
	}
	return se.Next()
})
```

Fail the boot. A medical records app should not start with an open collection.

### Mechanism B — a 404 middleware for opacity (optional, additive)

`e.Router` is the root router; you cannot *unregister* PB's routes (there's no `Unroute`).
But you can bind a root middleware that runs **after** `loadAuthToken` and 404s the record
CRUD subtree for non-superusers:

```go
const BlockAutoAPIMiddlewareId = "medigoBlockAutoAPI"

func BlockAutoCollectionAPI() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       BlockAutoAPIMiddlewareId,
		Priority: -1019, // AFTER loadAuthToken (-1020) so e.Auth is populated
		Func: func(e *core.RequestEvent) error {
			p := e.Request.URL.Path

			// only the record CRUD subtree; auth-* and superuser mgmt are untouched
			if strings.HasPrefix(p, "/api/collections/") && strings.Contains(p, "/records") {
				if !e.HasSuperuserAuth() {
					return e.NotFoundError("", nil)
				}
			}
			return e.Next()
		},
	}
}

// in OnServe:
se.Router.Bind(mw.BlockAutoCollectionAPI())
```

Also worth blocking for non-superusers, on the same principle (all are superuser-gated
already, so this is pure defence in depth): `/api/batch`, `/api/sql`, `/api/logs`,
`/api/backups`, `/api/crons`, `/api/settings`.

⚠️ `/api/batch` deserves a specific mention: it is a **transactional multiplexer over the
record CRUD handlers** — `apis/batch.go` literally calls `recordCreate`/`recordUpdate`/
`recordDelete`. **Verified: it re-uses the same handler bodies, so it enforces the same API
rules** (the boolean those constructors take is `responseWriteAfterTx`, not a rules bypass).
So nil rules do cover it. Still **leave it off** (`Settings().Batch.Enabled`, default
`false`): it's a large, transaction-holding attack surface with a ~128MB default body limit
and no benefit to MediGo, whose writes all go through hand-written endpoints.

### 🚨 The three collisions this creates

#### COLLISION #1 — nil rules kill PocketBase native realtime for regular users

`apis/realtime.go` builds its authorization map straight from the CRUD rules:

```go
subscriptionRuleMap := map[string]*string{
	(collection.Name + "/" + record.Id + "?"): collection.ViewRule,
	(collection.Id   + "/" + record.Id + "?"): collection.ViewRule,
	(collection.Name + "/*?"):                 collection.ListRule,
	(collection.Id   + "/*?"):                 collection.ListRule,
	(collection.Name + "?"):                   collection.ListRule,  // deprecated alias
	(collection.Id   + "?"):                   collection.ListRule,
}
...
if !realtimeCanAccessRecord(accessCheckApp, record, requestInfo, rule) { continue }
```

`realtimeCanAccessRecord` → `CanAccessRecord` → `accessRule == nil` → **false** for anyone
who isn't a superuser.

**So: `ListRule = ViewRule = nil` means native realtime silently delivers nothing to logged-in
users.** Not an error — the message is just skipped in the broadcast loop. You'd debug this
for hours.

The locked decisions say "use PocketBase's native realtime where PocketBase natively supports
it; use Datastar SSE for everything else." Combined with the lockdown, **PocketBase natively
supports it nowhere for MediGo's users.**

> **Recommended resolution: drop native realtime entirely and use Datastar SSE for 100% of
> realtime.** This is a simplification, not a loss:
> - The lockdown and native realtime are fundamentally incompatible — you'd have to re-open
>   `ListRule`/`ViewRule` to make it work, which defeats the entire API design.
> - Native realtime speaks raw collection records — exactly the shape MediGo is trying not to
>   expose. It would leak internal schema to the browser, bypassing your DTOs.
> - The frontend is Datastar, which consumes SSE natively. Two realtime transports
>   (PB's `/api/realtime` SSE + Datastar SSE) in one app is pointless complexity.
>
> Drive Datastar SSE off `OnRecordAfterCreateSuccess` / `OnRecordAfterUpdateSuccess` hooks
> (§10) or off `TxInfo().OnComplete` callbacks, fan out through your own broker, and emit
> DTO-shaped events. You keep one transport, one authorization path, one schema boundary.
>
> If you *must* keep `/api/realtime` for something, the only mechanism is a non-nil
> `ViewRule` on that specific collection — and then that collection's records are readable
> over `/api/collections/.../records` too. There is no way to separate the two.

#### COLLISION #2 — nil `ViewRule` breaks protected file downloads

`apis/file.go`:

```go
if fileField.Protected {
	...
	if ok, _ := e.App.CanAccessRecord(record, &requestInfo, record.Collection().ViewRule); !ok {
		return e.NotFoundError("", errors.New("insufficient permissions to access the file resource"))
	}
}
```

Same nil-rule logic: with `ViewRule = nil`, `GET /api/files/{collection}/{recordId}/{filename}`
returns **404 for every non-superuser** on a protected field.

#### COLLISION #3 (the nasty one) — *non*-protected files are PUBLIC regardless of rules

Read that snippet again: the access check is **inside `if fileField.Protected`**. There is no
`else`. For a `FileField` with `Protected: false`:

**`GET /api/files/{collection}/{recordId}/{filename}` serves the file to an unauthenticated
stranger.** No token, no auth, no rule evaluation. The only protection is that the filename
has a random 10-char suffix, i.e. security through obscurity. Locking down the CRUD API does
**nothing** here — a leaked or logged URL is a permanent public link.

For an app storing medical scans, lab PDFs and insurance documents this is a data breach
waiting to happen.

> **Recommended resolution for #2 and #3 together:**
> 1. **`Protected: true` on every single `FileField` in MediGo. No exceptions.** Make it a
>    boot-time assertion like the rules check above. This closes #3.
> 2. **Serve files from your own `/api/v1` route** using the filesystem abstraction directly
>    (§9), with authorization from your service layer. This routes around #2.
> 3. Optionally 404 `/api/files/` for non-superusers in the Mechanism B middleware, so the
>    built-in endpoint isn't even reachable.

### Summary answer to Q7

| | |
|---|---|
| **Is it cleanly possible?** | **Yes.** `nil` on all five rules is a first-class, documented, source-verified "superusers only" mode. |
| **Primary mechanism** | Five `nil` rules per collection, set in migrations, asserted at boot. |
| **Does internal Go access still work?** | **Yes, completely.** Rules live in the `apis` HTTP layer only. |
| **Does the admin UI still work?** | **Yes.** Superusers bypass all rules. |
| **Caveats** | Kills native realtime (#1); breaks protected file downloads (#2); does **not** protect non-protected file fields, which are public to the internet (#3); `/api/batch` must stay disabled. |

---

## 8. Auth

⚠️ **v0.23 CHANGE:** admins are gone as a separate concept. They are records in the
`_superusers` auth collection. `/api/admins/*` was removed. `e.Admin` doesn't exist — it's
`e.Auth.IsSuperuser()`.

### Collections

`users` is a normal auth collection (`core.CollectionTypeAuth`) created by the initial
system migration. `_superusers`, `_externalAuths`, `_mfas`, `_otps`, `_authOrigins` are
system auth/base collections (constants: `core.CollectionNameSuperusers`,
`core.CollectionNameExternalAuths`, `core.CollectionNameMFAs`, `core.CollectionNameOTPs`,
`core.CollectionNameAuthOrigins`).

Auth-collection options (`collectionAuthOptions`):

```go
AuthRule    *string  // extra constraint applied AFTER authentication, before issuing the token.
                     // "" = any record may authenticate; nil = authentication DISABLED entirely
                     // (password, OAuth2, OTP — all of it)
ManageRule  *string  // admin-like management of auth records (change password w/o old one, etc.)
AuthAlert   AuthAlertConfig
OAuth2      OAuth2Config
PasswordAuth PasswordAuthConfig
MFA         MFAConfig
OTP         OTPConfig

AuthToken, PasswordResetToken, EmailChangeToken, VerificationToken, FileToken  TokenConfig
VerificationTemplate, ResetPasswordTemplate, ConfirmEmailChangeTemplate        EmailTemplate
```

> **`AuthRule` is not one of the five CRUD rules.** Setting `ListRule = nil` etc. for the
> lockdown does **not** disable login. Keep `AuthRule = types.Pointer("")`, or
> `types.Pointer("verified = true")` if MediGo requires email verification before use.

### Tokens

JWTs (`golang-jwt/jwt/v5`), signed per-record with a key derived from the collection secret +
the record's `tokenKey`. Five types:

```go
core.TokenTypeAuth          = "auth"
core.TokenTypeFile          = "file"
core.TokenTypeVerification  = "verification"
core.TokenTypePasswordReset = "passwordReset"
core.TokenTypeEmailChange   = "emailChange"
```

Issue (methods on `*core.Record`):

```go
func (m *Record) NewAuthToken() (string, error)
func (m *Record) NewStaticAuthToken(duration time.Duration) (string, error)  // non-renewable, "API key"
func (m *Record) NewVerificationToken() (string, error)
func (m *Record) NewPasswordResetToken() (string, error)
func (m *Record) NewEmailChangeToken(newEmail string) (string, error)
func (m *Record) NewFileToken() (string, error)
```

Verify:

```go
func (app *BaseApp) FindAuthRecordByToken(token string, validTypes ...string) (*Record, error)
```

**Invalidation:** `record.RefreshTokenKey()` then save — this rotates the per-record signing
key and invalidates *every* outstanding token for that record. That's your "log out everywhere"
and your "password changed, kill all sessions". `SetPassword` also rotates it.

Other record auth helpers: `Email()`, `SetEmail()`, `EmailVisibility()`, `SetEmailVisibility()`,
`Verified()`, `SetVerified()`, `TokenKey()`, `SetPassword()`, `ValidatePassword()`,
`IsSuperuser()`.

### `loadAuthToken` and `e.Auth`

The global `loadAuthToken` middleware (priority `-1020`) reads the token (Authorization
header) and sets `e.Auth`. It **never rejects** — it just populates or leaves nil. Rejection
is `RequireAuth`'s job. So `e.Auth` is available in every handler and every middleware with
priority > `-1020`.

### Password auth from your own route

Either proxy PB's endpoint or do it yourself. Doing it yourself keeps everything under
`/api/v1` with your own DTOs:

```go
func (h *AuthHandler) Login(e *core.RequestEvent) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Malformed request body.", err)
	}

	rec, err := e.App.FindAuthRecordByEmail("users", req.Email)
	if err != nil || !rec.ValidatePassword(req.Password) {
		// identical response for both cases — no user enumeration
		return e.UnauthorizedError("Invalid credentials.", nil)
	}
	if !rec.Verified() {
		return e.ForbiddenError("Email address not verified.", nil)
	}

	// apis.RecordAuthResponse mints the token, fires OnRecordAuthRequest,
	// records the auth origin, and writes PB's standard {token, record} envelope
	return apis.RecordAuthResponse(e, rec, core.RequestInfoContextPasswordAuth, nil)
}
```

`apis.RecordAuthResponse(e *core.RequestEvent, authRecord *core.Record, authMethod string, meta any) error`
is exported and is the supported way to complete an auth flow from a custom route. If you want
a fully custom response shape, call `rec.NewAuthToken()` yourself and `e.JSON` your own DTO —
but then you skip the auth-origin recording and the `OnRecordAuthRequest` hook, so re-implement
what you need.

⚠️ **The one thing you cannot cheaply re-implement is MFA/OTP.** Those flows are woven through
`apis/record_auth_*.go` with their own events. If MediGo wants MFA, proxy to PB's
`/api/collections/users/auth-with-password` rather than hand-rolling.

### OAuth2 providers configured in Go, not the dashboard

Providers live in `app.Settings()`... no — they live on the **auth collection**, not settings.
`collection.OAuth2` is an `OAuth2Config`:

```go
type OAuth2Config struct {
	Providers    []OAuth2ProviderConfig
	MappedFields OAuth2KnownFields  // Id, Name, Username, AvatarURL -> your field names
	Enabled      bool
}

type OAuth2ProviderConfig struct {
	PKCE         *bool
	Name         string          // must match a registered provider key
	ClientId     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	DisplayName  string
	Extra        map[string]any
}
```

Configure it in a migration (persisted) or an `OnBootstrap` hook (re-applied each boot, which
is what you want when secrets come from env vars):

```go
app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
	if err := e.Next(); err != nil { return err }   // let PB bootstrap first
	if cfg.OIDC.ClientID == "" { return nil }

	users, err := e.App.FindCollectionByNameOrId("users")
	if err != nil { return err }

	users.OAuth2.Enabled = true
	users.OAuth2.MappedFields = core.OAuth2KnownFields{
		Id:        "external_id",
		Name:      "full_name",
		AvatarURL: "avatar",
	}
	users.OAuth2.Providers = []core.OAuth2ProviderConfig{{
		Name:         "oidc",
		DisplayName:  "MediGo SSO",
		ClientId:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		AuthURL:      cfg.OIDC.AuthURL,
		TokenURL:     cfg.OIDC.TokenURL,
		UserInfoURL:  cfg.OIDC.UserInfoURL,
	}, {
		Name:         "github",
		ClientId:     cfg.GitHub.ClientID,
		ClientSecret: cfg.GitHub.ClientSecret,
	}}

	return e.App.Save(users)
})
```

Provider keys come from `tools/auth` (`google`, `github`, `gitlab`, `oidc`, `oidc2`, `oidc3`,
`microsoft`, `apple`, `discord`, `facebook`, ... ~40 of them). `oidc`/`oidc2`/`oidc3` are the
three generic OIDC slots; the built-in named providers don't need `AuthURL`/`TokenURL`/`UserInfoURL`.

⚠️ v0.40.1 specifically fixed `OAuth2Config.UnmarshalJSON` so partially-submitted provider
config merges per-provider instead of replacing the whole slice (issue #7815). If you assign
`Providers` wholesale from Go as above you get replace semantics, which is what you want for
env-driven config — just make sure you set **all** providers every boot, not a subset.

The OAuth2 web flow itself needs `/api/collections/users/auth-with-oauth2` and the global
`/api/oauth2-redirect` route. Both live under paths your Mechanism B middleware must **not**
block (it only blocks `.../records`, so this is fine as written).

### Email verification and password reset

Handled by PB: `POST /api/collections/users/request-verification`,
`/confirm-verification`, `/request-password-reset`, `/confirm-password-reset`. Templates are
per-collection (`VerificationTemplate`, `ResetPasswordTemplate`, `ConfirmEmailChangeTemplate`),
each with `Subject`/`Body` and `{APP_NAME}`, `{APP_URL}`, `{TOKEN}` placeholders.

To customise the email itself, hook the mailer instead of editing templates:

```go
app.OnMailerRecordPasswordResetSend("users").BindFunc(func(e *core.MailerRecordEvent) error {
	// e.Message is *mailer.Message: From, To, Subject, HTML, Text, Headers, Attachments
	html, err := mail.PasswordResetTempl(e.Record.Email(), e.Meta["token"]).Render(...)  // templ!
	if err != nil { return err }
	e.Message.HTML = html
	e.Message.Subject = "Reset your MediGo password"
	return e.Next()
})
```

Available: `OnMailerSend`, `OnMailerRecordAuthAlertSend`, `OnMailerRecordPasswordResetSend`,
`OnMailerRecordVerificationSend`, `OnMailerRecordEmailChangeSend`, `OnMailerRecordOTPSend`.
**This is the clean way to render all transactional email with templ** — one renderer for
web pages and email.

### Attaching the authenticated user to your service layer

Don't pass `*core.Record` into services; it drags PocketBase into your domain. Map at the
handler boundary:

```go
// internal/authctx/actor.go
type Actor struct {
	ID         string
	Email      string
	Verified   bool
	Superuser  bool
	Collection string
}

func FromRecord(r *core.Record) *Actor {
	if r == nil { return nil }
	return &Actor{
		ID:         r.Id,
		Email:      r.Email(),
		Verified:   r.Verified(),
		Superuser:  r.IsSuperuser(),
		Collection: r.Collection().Name,
	}
}
```

Then either pass `*Actor` explicitly as the first service arg (preferred — explicit, testable,
no context-key magic) or stash it on the request context in a middleware. Given the SOLID /
interfaces-at-every-seam principle, **explicit parameter wins**: `svc.CreateLab(ctx, actor, input)`
makes authorization a visible part of every service signature.

---

## 9. Files

### The abstraction

`app.NewFilesystem() (*filesystem.System, error)` returns local or S3 depending on
`app.Settings().S3.Enabled`. Local root is `<dataDir>/storage`.
`app.NewBackupsFilesystem()` is the parallel one for backups (separate S3 config).

```go
type S3Config struct {
	Enabled        bool
	Bucket, Region, Endpoint, AccessKey, Secret string
	ForcePathStyle bool
}
```

`*filesystem.System` API:

```go
func NewLocal(dirPath string) (*System, error)
func NewS3(bucket, region, endpoint, accessKey, secretKey string, s3ForcePathStyle bool) (*System, error)

func (s *System) SetContext(ctx context.Context)
func (s *System) Close() error
func (s *System) Exists(fileKey string) (bool, error)
func (s *System) Attributes(fileKey string) (*blob.Attributes, error)
func (s *System) GetReader(fileKey string) (*blob.Reader, error)
func (s *System) GetFile(fileKey string) (*blob.Reader, error)
func (s *System) GetReuploadableFile(fileKey string, preserveName bool) (*File, error)
func (s *System) Copy(srcKey, dstKey string) error
func (s *System) List(prefix string) ([]*blob.ListObject, error)
func (s *System) Upload(content []byte, fileKey string) error
func (s *System) UploadFile(file *File, fileKey string) error
func (s *System) UploadMultipart(fh *multipart.FileHeader, fileKey string) error
func (s *System) NewWriter(fileKey string, opts *blob.WriterOptions) (*blob.Writer, error)  // v0.40
func (s *System) Delete(fileKey string) error
func (s *System) DeletePrefix(prefix string) []error
func (s *System) Serve(res http.ResponseWriter, req *http.Request, fileKey, name string) error
func (s *System) CreateThumb(originalKey, thumbKey, thumbSize string) error
func (s *System) OnNewWriter() *hook.Hook[*NewWriterEvent]   // v0.40
func (s *System) OnDelete()    *hook.Hook[*DeleteEvent]      // v0.40
```

**Always `defer fsys.Close()`.** For S3 it holds a client.

### Upload from a Go handler

The v0.23+ way: **a file is just a field value.** `record.Set("field", file)` — no forms API.

```go
func (h *AttachmentHandler) Upload(e *core.RequestEvent) error {
	patientID := e.Request.PathValue("patientId")

	files, err := e.FindUploadedFiles("attachment")   // []*filesystem.File
	if err != nil || len(files) == 0 {
		return e.BadRequestError("Missing attachment.", err)
	}

	col, err := e.App.FindCachedCollectionByNameOrId("attachments")
	if err != nil { return err }

	rec := core.NewRecord(col)
	rec.Set("patient", patientID)
	rec.Set("uploaded_by", e.Auth.Id)
	rec.Set("file", files[0])          // <- the *filesystem.File IS the value

	if err := e.App.SaveWithContext(e.Request.Context(), rec); err != nil {
		return e.InternalServerError("Failed to store attachment.", err)
	}

	return e.JSON(http.StatusCreated, toAttachmentDTO(rec))
}
```

`*filesystem.File` constructors:

```go
filesystem.NewFileFromPath(path string) (*File, error)
filesystem.NewFileFromBytes(b []byte, name string) (*File, error)
filesystem.NewFileFromMultipart(mh *multipart.FileHeader) (*File, error)
filesystem.NewFileFromURL(ctx context.Context, url string) (*File, error)
```

> ⚠️ **`NewFileFromURL` is a textbook SSRF sink** — it fetches an arbitrary URL server-side.
> (The monorepo memory notes a prior SSRF fix in `technologia` logo uploads for exactly this
> pattern.) If MediGo ever ingests a file by URL, validate the resolved IP against
> private/link-local ranges *after* DNS resolution, and never follow redirects blindly.

⚠️ **v0.23 CHANGE:** uploading to a multi-file field **replaces** existing files. To append or
prepend use the modifier keys `"+field"` (prepend) / `"field+"` (append). Pre-v0.23 appended
by default.

Deleting a file: `record.Set("file", "")` or remove the filename from the slice, then save.
PB cleans up the blob after a successful record save.

### Serving files — MediGo's own route

Given collisions #2/#3 in §7, serve everything yourself:

```go
func (h *AttachmentHandler) Download(e *core.RequestEvent) error {
	actor := authctx.FromRecord(e.Auth)
	id := e.Request.PathValue("attachmentId")

	att, err := h.svc.GetForDownload(e.Request.Context(), actor, id)  // authz lives HERE
	switch {
	case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrForbidden):
		return e.NotFoundError("", nil)
	case err != nil:
		return e.InternalServerError("Failed to load attachment.", err)
	}

	fsys, err := e.App.NewFilesystem()
	if err != nil {
		return e.InternalServerError("Storage unavailable.", err)
	}
	defer fsys.Close()
	fsys.SetContext(e.Request.Context())

	// key = <collection baseFilesPath>/<recordId>/<filename>
	key := att.Record.BaseFilesPath() + "/" + att.Filename

	// Serve handles Range, ETag, Content-Type, and Content-Disposition
	return fsys.Serve(e.Response, e.Request, key, att.OriginalName)
}
```

`record.BaseFilesPath()` gives `<collectionId>/<recordId>`. Thumb keys are
`<baseFilesPath>/thumbs_<filename>/<size>_<filename>`.

> v0.40 added quoting around the default `Content-Disposition` filename, so names with
> special characters are handled correctly. Patient-supplied filenames are fine to pass to
> `Serve`'s `name` argument.

### Thumbnails

Declared per file field: `Thumbs: []string{"100x100", "300x200f", "0x400", "600x0"}`.
Format is `WxH`, plus a suffix: `f` = fit (no crop), `t`/`b`/`l`/`r` = crop anchored top/
bottom/left/right, none = centre crop. `0` on an axis preserves aspect ratio.

Thumbs are generated **lazily on first request** by the built-in `/api/files/...?thumb=100x100`
handler, with a `singleflight` group and a weighted semaphore (`PB_THUMBS_MAX_WORKERS`,
default `NumCPU+2`; `PB_THUMBS_MAX_WAIT`, default 60s).

⚠️ **If you bypass `/api/files/` (which §7 says you should), nothing generates thumbs.** Call
`fsys.CreateThumb(originalKey, thumbKey, size)` yourself, either eagerly on upload (in an
`OnRecordAfterCreateSuccess` hook) or lazily in your own download route. Eager is simpler and
avoids the thundering-herd machinery; do it in a `TxInfo().OnComplete` callback so it doesn't
extend the write transaction.

### File tokens

`POST /api/files/token` (requires auth) → `{"token": "..."}` from `e.Auth.NewFileToken()`.
The token is short-lived (`collection.FileToken.Duration`) and passed as `?token=` to the
built-in protected-file route. **MediGo doesn't need this** if you serve files from your own
authenticated routes — your session token is already on the request. Skip the whole mechanism.

---

## 10. Hooks

### Naming taxonomy (v0.40)

Three orthogonal axes:

1. **Subject**: `Model` (any model) → `Record` / `Collection` (specialised).
2. **Stage**: `<verb>` (before, inside the tx) → `<verb>Execute` (immediately around the DB
   write) → `After<Verb>Success` / `After<Verb>Error` (after the tx settles).
3. **Origin**: bare (any source, including your Go code) vs `...Request` (HTTP only).

```
OnModelValidate / OnModelCreate / OnModelCreateExecute
  / OnModelAfterCreateSuccess / OnModelAfterCreateError
  (same for Update, Delete)

OnRecordValidate / OnRecordCreate / OnRecordCreateExecute
  / OnRecordAfterCreateSuccess / OnRecordAfterCreateError
  (same for Update, Delete)  + OnRecordEnrich

OnCollectionValidate / OnCollectionCreate / ... (same shape)

OnRecordsListRequest / OnRecordViewRequest / OnRecordCreateRequest
  / OnRecordUpdateRequest / OnRecordDeleteRequest

OnRecordAuthRequest / OnRecordAuthWithPasswordRequest / OnRecordAuthWithOAuth2Request
  / OnRecordAuthRefreshRequest / OnRecordRequestPasswordResetRequest
  / OnRecordConfirmPasswordResetRequest / OnRecordRequestVerificationRequest
  / OnRecordConfirmVerificationRequest / OnRecordRequestEmailChangeRequest
  / OnRecordConfirmEmailChangeRequest / OnRecordRequestOTPRequest / OnRecordAuthWithOTPRequest

OnBootstrap / OnServe / OnTerminate / OnSettingsReload
OnBackupCreate / OnBackupRestore
OnMailerSend / OnMailerRecord*Send
OnFileDownloadRequest / OnFileTokenRequest
OnRealtimeConnectRequest / OnRealtimeSubscribeRequest / OnRealtimeMessageSend
OnSettingsListRequest / OnSettingsUpdateRequest
OnCollectionsListRequest / OnCollectionViewRequest / ... / OnCollectionsImportRequest
OnBatchRequest
```

⚠️ **v0.23 CHANGE:** the old `OnRecordBeforeCreateRequest` / `OnRecordAfterCreateRequest` /
`OnBeforeServe` names are **gone**. `Before` disappeared from the vocabulary (the bare name
*is* the before-hook); `After` survives only in `After<Verb>Success` / `After<Verb>Error`.

### Tagged hooks

Everything record/collection-scoped returns `*hook.TaggedHook[T]`, filtered by collection
name or id:

```go
app.OnRecordAfterCreateSuccess("lab_results").BindFunc(...)         // one collection
app.OnRecordAfterCreateSuccess("lab_results", "vitals").BindFunc(...) // several
app.OnRecordAfterCreateSuccess().BindFunc(...)                      // all collections
```

### Chain semantics

Identical to middleware: `Bind(&hook.Handler[T]{Id, Priority, Func})` or `BindFunc(fn)`;
lower priority first; same id replaces; **you must call `e.Next()`** or the chain (and the
underlying operation) is short-circuited. Not calling `Next()` in a *before* hook is the
supported way to veto an operation — return an error instead if you want a meaningful message.

`OnBootstrap` is the one place where `e.Next()` goes **first**, because you need PB to have
actually bootstrapped before you can touch the DB:

```go
app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
	if err := e.Next(); err != nil { return err }   // FIRST
	// now Settings(), DB(), collections are all available
	return applyMediGoSettings(e.App)
})
```

### Which hook for which job (MediGo)

| Need | Hook |
|---|---|
| Domain invariants, cross-field validation | `OnRecordValidate("collection")` |
| Denormalise / derive a field before write | `OnRecordCreate` / `OnRecordUpdate` |
| Audit log entry in the same transaction | `OnRecordCreateExecute` etc. |
| Datastar SSE broadcast, cache bust, webhook | `OnRecordAfterCreateSuccess` / `...UpdateSuccess` |
| Sentry capture on write failure | `OnRecordAfterCreateError` (`*RecordErrorEvent`) |
| Hide fields from API output | `OnRecordEnrich` |
| Settings/OAuth2/rate-limit config from env | `OnBootstrap` (after `e.Next()`) |
| Routes, custom middleware, static assets | `OnServe` |
| Flush metrics, close pools, drain SSE | `OnTerminate` |
| Custom transactional email via templ | `OnMailerRecord*Send` |

`OnRecordAfterCreateSuccess` fires **after** the transaction commits, so it is the correct
place for side effects — no risk of broadcasting a rolled-back write. Use it in preference to
`TxInfo().OnComplete` when the trigger is "a record changed"; use `OnComplete` when the
trigger is "my multi-step service operation finished".

### Composing hooks with custom routes

They're the same primitive, so composition is free. A custom route can trigger record hooks
just by calling `app.Save()`. Two things to watch:

1. **Your `/api/v1` writes fire `OnRecord*` hooks but *not* `OnRecord*Request` hooks** —
   the `*Request` family is bound inside the built-in CRUD handlers. Since MediGo locks those
   down, **never put business logic in a `*Request` hook**; it will simply never run. Use the
   bare (non-`Request`) hooks.
2. `app.UnsafeWithoutHooks()` returns an app with hooks disabled — for seeding, migrations, or
   breaking a hook recursion. Use it deliberately and rarely; it also skips your audit logging.

---

## 11. Realtime

### How it works

- `GET /api/realtime` — SSE stream. On connect PB registers a `subscriptions.Client` in
  `app.SubscriptionsBroker()` and sends `PB_CONNECT` with the generated `clientId`.
- `POST /api/realtime` — body `{clientId, subscriptions: ["patients", "patients/abc123",
  "patients/*"]}`. Replaces the client's whole subscription set.
- Server pushes SSE events named after the subscription topic, payload
  `{"action":"create"|"update"|"delete","record":{...}}`.

Guards in `realtimeSetSubscriptions`: the subscribing request's IP must match the connecting
IP; auth may be upgraded guest→auth once, but any *change* of auth identity is a 403.

`Client` interface: `Id()`, `Channel() chan Message`, `Subscriptions(prefixes...)`,
`Subscribe(subs...)`, `Unsubscribe(subs...)`, `HasSubscription(sub)`, `Get/Set/Unset(key)`,
`Send(Message)`, `Discard()`, `IsDiscarded()`.

### What it can push

**Only record change events, for collections, gated by `ListRule`/`ViewRule`.** The topic→rule
map is hard-coded (quoted in §7). Per-subscription client-side `filter` and `expand` query
options are honoured, and the filter is re-evaluated server-side per record.

### What it cannot push

- Anything that isn't a record change: progress, presence, notifications, computed aggregates.
- Anything DTO-shaped. It sends the raw record (minus hidden fields).
- Anything to a user who can't pass the collection's `ListRule`/`ViewRule`.

### Can a Go server publish custom realtime messages?

**Yes, technically.** The broker is exported and `Client.Send` takes an arbitrary message:

```go
for _, client := range app.SubscriptionsBroker().Clients() {
	if !client.HasSubscription("medigo:lab-import") { continue }
	client.Send(subscriptions.Message{
		Name: "medigo:lab-import",
		Data: []byte(`{"progress":42}`),
	})
}
```

Caveats: you own the authorization (nothing checks it for custom topics), the
`OnRealtimeMessageSend` hook still fires, and `Clients()` is a snapshot map you must not
mutate. Clients also have to `POST /api/realtime` with your custom topic name — the SDKs will
happily do that, but you're now maintaining a bespoke protocol on top of PB's.

### 🚨 Recommendation (repeat of COLLISION #1)

**Don't use PB realtime in MediGo. Use Datastar SSE for everything.**

With the nil-rule lockdown, native realtime delivers nothing to regular users (§7,
COLLISION #1), and the only fix is re-opening the collection API. Meanwhile everything native
realtime pushes is raw-record-shaped, which is precisely the abstraction MediGo is paying to
avoid. Running PB realtime *and* Datastar SSE gives two transports, two auth models, and two
event schemas for no benefit.

A single Datastar SSE endpoint under `/api/v1`, fed from `OnRecordAfterCreateSuccess` /
`OnRecordAfterUpdateSuccess` hooks through your own broker, gives you DTO-shaped events, your
own authorization, and one thing to test. Revise the locked decision to "Datastar SSE for all
realtime" — PocketBase natively supports realtime for MediGo's access model nowhere.

---

## 12. Testing

### `tests.TestApp`

```go
type TestApp struct {
	*core.BaseApp
	EventCalls map[string]int   // hook name -> fire count
	TestMailer *TestMailer      // captures sent messages instead of SMTP
}

func tests.NewTestApp(optTestDataDir ...string) (*TestApp, error)
func tests.NewTestAppWithConfig(config core.BaseAppConfig) (*TestApp, error)
func (t *TestApp) Cleanup()
func (t *TestApp) ResetEventCalls()
```

**The throwaway-app pattern is built in.** `NewTestAppWithConfig` calls
`TempDirClone(config.DataDir)` and points the app at the clone, then `Bootstrap()`s it and
applies pending migrations. `Cleanup()` fires `OnTerminate` and `os.RemoveAll`s the temp dir.
So every call gives you a genuinely isolated SQLite database seeded from a fixture directory.

Default fixture dir is PocketBase's own `tests/data`. **MediGo should ship its own**: run the
app once, apply migrations, seed reference data (the standardized lab test catalog, tag
vocabularies), then commit that `pb_data` as `internal/testdata/pb_data`.

### testify helper

```go
package testutil

func NewApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{
		DataDir:       "../../internal/testdata/pb_data",
		EncryptionEnv: "medigo_test_env",
		IsDev:         false,
	})
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	return app
}
```

```go
func TestLabService_Create(t *testing.T) {
	t.Parallel()      // safe: each subtest gets its own cloned DB

	app := testutil.NewApp(t)
	svc := service.NewLabService(store.New(app), zerolog.Nop())

	owner, err := app.FindAuthRecordByEmail("users", "test@medigo.local")
	require.NoError(t, err)

	out, err := svc.Create(t.Context(), authctx.FromRecord(owner), service.CreateLabInput{
		PatientID: "pat_000000000001",
		TestCode:  "HGB",
		Value:     14.2,
		Unit:      "g/dL",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, out.ID)
	assert.Equal(t, "normal", out.Flag)

	saved, err := app.FindRecordById("lab_results", out.ID)
	require.NoError(t, err)
	assert.InDelta(t, 14.2, saved.GetFloat("value"), 0.001)
}
```

Because `*tests.TestApp` embeds `*core.BaseApp` it satisfies `core.App` — pass it anywhere a
`core.App` is expected. And because MediGo's services take narrow interfaces, most unit tests
won't need a TestApp at all; use testify mocks and reserve TestApp for the store layer and
handler integration tests.

### `tests.ApiScenario` — HTTP-level tests

```go
type ApiScenario struct {
	Name    string
	Method  string
	URL     string
	Body    io.Reader
	Headers map[string]string
	Delay   time.Duration
	Timeout time.Duration
	DisableTestAppCleanup bool

	ExpectedStatus     int
	ExpectedContent    []string   // substrings that MUST appear
	NotExpectedContent []string   // substrings that MUST NOT appear
	ExpectedEvents     map[string]int  // "*": 0 asserts NO other events fired

	TestAppFactory func(t testing.TB) *TestApp
	BeforeTestFunc func(t testing.TB, app *TestApp, e *core.ServeEvent)
	AfterTestFunc  func(t testing.TB, app *TestApp, res *http.Response)
}

func (s *ApiScenario) Test(t *testing.T)
func (s *ApiScenario) Benchmark(b *testing.B)
```

`BeforeTestFunc` receives the `*core.ServeEvent`, so **this is where you register MediGo's
routes** for the test:

```go
func TestPatientsAPI(t *testing.T) {
	setup := func(t testing.TB, app *tests.TestApp, se *core.ServeEvent) {
		api.Register(se, api.Deps{App: app, Logger: zerolog.Nop()})
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "guest cannot list patients",
			Method:          http.MethodGet,
			URL:             "/api/v1/patients",
			TestAppFactory:  func(t testing.TB) *tests.TestApp { return testutil.NewApp(t.(*testing.T)) },
			BeforeTestFunc:  setup,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:               "auto CRUD API is locked down",
			Method:             http.MethodGet,
			URL:                "/api/collections/patients/records",
			Headers:            map[string]string{"Authorization": userToken},
			TestAppFactory:     func(t testing.TB) *tests.TestApp { return testutil.NewApp(t.(*testing.T)) },
			ExpectedStatus:     404,
			NotExpectedContent: []string{"full_name", "birth_date"},
		},
	}

	for _, s := range scenarios {
		s.Test(t)
	}
}
```

That second scenario is worth writing for **every** collection — it's the regression test for
§7. `ExpectedEvents: map[string]int{"*": 0}` is a nice way to assert a request had no side
effects at all.

Note `ExpectedContent` is substring matching, not JSON-structural. For real DTO assertions use
`AfterTestFunc` and unmarshal the body yourself.

---

## 13. 🪵 Logging — replacing PocketBase's slog handler

**Short answer: PocketBase does NOT cleanly support replacing the logger. There is no config
field, no setter, and no hook. There is a workaround that covers most — not all — of it, and
you should combine it with disabling `_logs`.**

### What PocketBase actually does

`core/base.go`:

```go
type BaseApp struct {
	...
	logger *slog.Logger     // UNEXPORTED
	...
}

func (app *BaseApp) Logger() *slog.Logger {
	if app.logger == nil {
		return slog.Default()
	}
	return app.logger
}

func (app *BaseApp) initLogger() error {
	handler := logger.NewBatchHandler(logger.BatchOptions{
		Level:     getLoggerMinLevel(app),
		BatchSize: 200,
		BeforeAddFunc: func(ctx context.Context, log *logger.Log) bool {
			if app.IsDev() {
				printLog(log)                                        // <- stdout, unstructured
				if log.Level < slog.Level(app.Settings().Logs.MinLevel) { return false }
			}
			ticker.Reset(duration)
			return app.Settings().Logs.MaxDays > 0                   // <- the kill switch
		},
		WriteFunc: func(ctx context.Context, logs []*logger.Log) error {
			if !app.IsBootstrapped() || app.Settings().Logs.MaxDays == 0 { return nil }
			app.AuxRunInTransaction(func(txApp App) error { /* INSERT into _logs */ })
			return nil
		},
	})
	...
	app.logger = slog.New(handler)     // <- the ONLY assignment, from initLogger, at bootstrap
	...
}
```

Search the whole package: there is **no** `SetLogger`, the `Logger` field is absent from
`core.BaseAppConfig` and from `pocketbase.Config`, and `app.logger` is written in exactly one
place. `Logger()` returns a `*slog.Logger` (a concrete struct pointer), and `slog.Logger` has
no handler setter. **`BatchHandler` exposes only `SetLevel`, `WriteAll`, `Enabled`,
`Handle`, `WithAttrs`, `WithGroup`** — you cannot swap its `WriteFunc` or `BeforeAddFunc`
after construction.

**Verdict: not cleanly supported. Be honest about this in the spec.**

### The workaround: wrap the embedded `core.App` interface

`pocketbase.PocketBase` embeds `core.App` as an **interface field named `App`**, and it's
exported. So you can decorate it:

```go
// internal/pblog/app.go
package pblog

type LoggedApp struct {
	core.App                 // embedded interface: everything else passes through
	logger *slog.Logger
}

func (a *LoggedApp) Logger() *slog.Logger { return a.logger }

// Wrap replaces pb's app with a decorated one whose Logger() returns a zerolog-backed slog.
func Wrap(pb *pocketbase.PocketBase, zl zerolog.Logger) {
	pb.App = &LoggedApp{
		App:    pb.App,
		logger: slog.New(NewZerologHandler(zl)),
	}
}
```

The `slog.Handler` bridge itself is straightforward:

```go
type ZerologHandler struct {
	zl    zerolog.Logger
	attrs []slog.Attr
	group string
}

func NewZerologHandler(zl zerolog.Logger) *ZerologHandler { return &ZerologHandler{zl: zl} }

func (h *ZerologHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return h.zl.GetLevel() <= toZerolog(lvl)
}

func (h *ZerologHandler) Handle(_ context.Context, r slog.Record) error {
	ev := h.zl.WithLevel(toZerolog(r.Level)).Str("source", "pocketbase")
	for _, a := range h.attrs { ev = appendAttr(ev, h.group, a) }
	r.Attrs(func(a slog.Attr) bool { ev = appendAttr(ev, h.group, a); return true })
	ev.Msg(r.Message)
	return nil
}

func (h *ZerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(slices.Clip(h.attrs), attrs...)
	return &c
}

func (h *ZerologHandler) WithGroup(name string) slog.Handler {
	c := *h
	if c.group != "" { c.group += "." + name } else { c.group = name }
	return &c
}

func toZerolog(l slog.Level) zerolog.Level {
	switch {
	case l >= slog.LevelError: return zerolog.ErrorLevel
	case l >= slog.LevelWarn:  return zerolog.WarnLevel
	case l >= slog.LevelInfo:  return zerolog.InfoLevel
	default:                   return zerolog.DebugLevel
	}
}
```

Call `pblog.Wrap(app, zl)` **immediately after `NewWithConfig`, before any hook registration**.

### What the wrapper actually covers — and what it doesn't

The decisive question is what `e.App` is bound to. Traced through the source:

- `cmd.NewServeCommand(pb, ...)` closes over `pb` (the `*PocketBase`).
- It calls `apis.Serve(app, ...)` → `apis.NewRouter(app)`.
- `apis/base.go:24` in the event factory: `event.App = app`.

So **every `*core.RequestEvent` carries `pb`**, and `pb.Logger()` → embedded
`core.App.Logger()` → your `LoggedApp.Logger()` → zerolog. Same for `*core.ServeEvent`
(`apis/serve.go:210`).

| Log source | Reaches zerolog? |
|---|---|
| `activityLogger` middleware — every HTTP request/response | ✅ yes |
| `panicRecover` middleware | ✅ yes |
| Anything using `e.App.Logger()` in `apis/*` (realtime, files, auth, batch) | ✅ yes |
| Your own handlers/hooks using `e.App.Logger()` | ✅ yes |
| `BaseApp` internals: `app.Logger()` inside `core/base.go` methods — WAL checkpoint warnings, cron failures, `Restart()` errors, collection cache reload warnings | ❌ **no** — still the `BatchHandler` |
| Anything inside `RunInTransaction` using `txApp.Logger()` — `createTxApp` does `clone := *app` on the **`*BaseApp`**, not your wrapper | ❌ **no** |
| The `__pbLogsCleanup__` cron | ❌ no |

So the wrapper gets **the request path — which is 95%+ of log volume — and misses a thin tail
of BaseApp-internal operational warnings.**

### Closing the gap: disable `_logs`

The locked decision says PB's `_logs` collection is disabled. That's a single setting, and it
conveniently also neuters the leaked handler:

```go
app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
	if err := e.Next(); err != nil { return err }

	s := e.App.Settings()
	s.Logs.MaxDays = 0     // BeforeAddFunc returns false -> nothing batched, nothing written
	s.Logs.MinLevel = int(slog.LevelWarn)
	s.Logs.LogIP = false   // PHI-adjacent; don't persist
	s.Logs.LogAuthId = false
	return e.App.Save(s)
})
```

With `MaxDays = 0`:
- `BeforeAddFunc` returns `false` → the log never enters the batch.
- `WriteFunc` early-returns → no `_logs` INSERTs, no aux-DB write amplification.
- In dev mode `printLog` still writes to stdout **before** the `MaxDays` check — that's fine
  locally, and `IsDev` is false in production.
- The `__pbLogsCleanup__` cron becomes a no-op.

Net effect in production: the residual `BatchHandler` becomes a **/dev/null sink**. The
BaseApp-internal warnings in the table above are therefore not *misrouted* — they are
**dropped**. That's the honest trade.

### If you need that last 5%

Options, worst to best:

1. **Accept the loss.** These are low-frequency operational warnings (WAL checkpoint, aux
   VACUUM, cron failure). Your own cron wrappers and health checks can cover the same ground.
   **Recommended.**
2. **Set `Logs.MaxDays = 1` and bridge `_logs` writes.** `WriteFunc` calls
   `txApp.AuxSave(model)` on a `*core.Log`, which goes through the model hooks — so
   `app.OnModelAfterCreateSuccess("_logs")` can forward into zerolog. But this contradicts
   "disable `_logs`", batches with a 3s ticker, and doubles storage. Not worth it.
3. **`slog.SetDefault(zerologSlog)` early.** Only helps before bootstrap (when `app.logger`
   is nil and `Logger()` falls back to `slog.Default()`). Do it anyway — it's one line and
   catches pre-bootstrap logging — but it is not a solution.
4. **Fork/patch `BaseApp`.** No.

### Verdict for the spec

> The zerolog swap is **~95% clean, not 100%**. Wrapping `pb.App` redirects the entire request
> path — every access log, panic, auth event, file event and realtime event — into zerolog
> through a `slog.Handler` bridge. A thin tail of `core.BaseApp`-internal operational warnings
> keeps a private handler that we neutralise by setting `Logs.MaxDays = 0`, so those lines are
> dropped rather than misrouted. PocketBase exposes no supported handler-injection point; this
> is a decorator workaround over an embedded interface field, and it will need re-verification
> on every PocketBase upgrade.

Also worth knowing: v0.40 added `Logs.MaxDataSize` (default ~16KB) which truncates oversized
log data and appends `"__pb_truncated__": true`, and caps log messages at 8k characters.
Irrelevant once `_logs` is off, but it tells you PB assumes untrusted data reaches its logger —
so should you.

---

## 14. Templates: PB's `tools/template` vs templ

PocketBase ships a small `html/template` wrapper used for its own email templates:

```go
func template.NewRegistry() *Registry
func (r *Registry) AddFuncs(funcs map[string]any) *Registry
func (r *Registry) LoadFiles(filenames ...string) *Renderer
func (r *Registry) LoadString(text string) *Renderer
func (r *Registry) LoadFS(fsys fs.FS, globPatterns ...string) *Renderer
// Renderer.Render(data any) (string, error)
```

**MediGo will not use it, and there is zero conflict.** It is an ordinary helper, not a
registered view engine — nothing in the router or `ServeEvent` references it, and no hook
requires it. Ignore the package entirely.

Serving templ from a PB route is trivial. templ components implement
`Render(ctx context.Context, w io.Writer) error`, and `e.Response` is a plain
`http.ResponseWriter`:

```go
func render(e *core.RequestEvent, status int, c templ.Component) error {
	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
	e.Response.WriteHeader(status)
	return c.Render(e.Request.Context(), e.Response)
}

func (h *PatientHandler) Page(e *core.RequestEvent) error {
	actor := authctx.FromRecord(e.Auth)
	vm, err := h.svc.PatientPage(e.Request.Context(), actor, e.Request.PathValue("patientId"))
	if err != nil {
		return e.NotFoundError("", nil)
	}
	return render(e, http.StatusOK, views.PatientPage(vm))
}
```

Two notes:

- `templ.Handler(component)` returns an `http.Handler`, so `apis.WrapStdHandler(templ.Handler(c))`
  also works. The explicit helper above is better — it keeps error handling in PocketBase's
  `error`-returning idiom and lets you set the status.
- **Datastar SSE from a PB route works the same way.** Get the raw writer with `e.Response`,
  pass it to `datastar.NewSSE(e.Response, e.Request)`, and keep the handler blocked until
  `e.Request.Context()` is done. Set `Priority` so gzip doesn't wrap it — or simply don't bind
  `apis.Gzip()` on the SSE route, since gzip buffering breaks SSE flushing. `router.Event`
  exposes `Flush()` and the wrapped `ResponseWriter` implements `Unwrap()`, so
  `http.ResponseController` works correctly for flush/deadline control.

---

## 15. Static and embedded assets

`apis.Static(fsys fs.FS, indexFallback bool) func(*core.RequestEvent) error`. It **requires
the route to have a `{path...}` wildcard** — it reads `e.Request.PathValue("path")`
(`apis.StaticWildcardParam`).

```go
//go:embed all:dist
var assetsFS embed.FS

app.OnServe().BindFunc(func(se *core.ServeEvent) error {
	// strip the "dist" prefix; panics on failure
	dist := apis.MustSubFS(assetsFS, "dist")

	se.Router.GET("/static/{path...}", apis.Static(dist, false)).
		Bind(apis.Gzip()).
		BindFunc(func(e *core.RequestEvent) error {
			// hashed filenames -> immutable caching
			if !e.App.IsDev() {
				e.Response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			return e.Next()
		})

	return se.Next()
})
```

Behaviour worth knowing:

- Directory-traversal is checked eagerly, plus `os.DirFS` refuses `..` anyway.
- `/x/index.html` → 301 to `/x/`; a directory without a trailing slash → 301 to `/x/`;
  a file path with a trailing slash → 301 to the non-slash form.
- `indexFallback: true` serves `index.html` for any miss — for SPAs. **MediGo is server-rendered
  templ, so use `false`** and let misses 404 properly.
- `Static` sets `requestEventKeySkipSuccessActivityLog`, so successful asset hits don't flood
  the activity log. Errors are still logged.
- The `examples/base` pattern binds the catch-all `GET /{path...}` at `Priority: 999` and
  guards with `if !e.Router.HasRoute(http.MethodGet, "/{path...}")` so user routes win. Copy
  that guard if you register a root catch-all.

For MediGo: embed the Tailwind output and the Datastar JS bundle. Prefer a **content-hashed
filename** (`app.a1b2c3.css`) generated by the Tailwind step and referenced from a templ
layout, so `immutable` caching is safe.

⚠️ The admin dashboard occupies `/_/{path...}` and sets a strict CSP on its own responses.
Don't mount MediGo assets under `/_/`.

---

## 16. Collision register — locked decisions vs. how PocketBase wants to be used

Consolidated, most severe first.

| # | Collision | Severity | Recommended resolution |
|---|---|---|---|
| **0** | **Go 1.26.5 cannot build PB v0.40.1.** `go 1.27` + 120 files importing `encoding/json/v2` unguarded. | 🔴 **Blocker** | Move the locked toolchain to **Go 1.27.x**. Fallback: pin PB to v0.39.x. Also budget time for Go 1.27's not-fully-backward-compatible `encoding/json` retrofit across MediGo's own DTOs. |
| **1** | **Nil API rules kill native realtime.** `apis/realtime.go` gates broadcasts on `ListRule`/`ViewRule`; `nil` → superuser-only. Silently delivers nothing, no error. | 🔴 High | **Drop native realtime. Use Datastar SSE for 100% of realtime**, fed from `OnRecordAfter*Success` hooks. Revise the locked "use PB native realtime where supported" — with the lockdown, that's nowhere. |
| **2** | **Non-protected file fields are world-readable.** `apis/file.go` only access-checks when `fileField.Protected` is true. No `else`. Locking down CRUD does not help. | 🔴 High (data breach class) | **`Protected: true` on every `FileField`, asserted at boot.** Serve all files from hand-written `/api/v1` routes via `app.NewFilesystem()`. Optionally 404 `/api/files/` for non-superusers. |
| **3** | **Nil `ViewRule` breaks protected file downloads** via the built-in `/api/files/` route. | 🟠 Medium | Consequence of the same design; resolved by the same move — serve files yourself. |
| **4** | **zerolog swap is not natively supported.** No config field, no setter, no hook; `app.logger` is private and assigned once in `initLogger`. | 🟠 Medium | Decorate the embedded `pb.App` interface field with a `Logger()` override (covers the whole request path) **and** set `Logs.MaxDays = 0`. Accept that BaseApp-internal warnings are dropped. Re-verify on every PB upgrade. |
| **5** | **`OnRecord*Request` hooks never fire** for MediGo, because the built-in CRUD handlers that trigger them are locked down. | 🟠 Medium | Put business logic in the **bare** hooks (`OnRecordCreate`, `OnRecordAfterCreateSuccess`), never the `*Request` variants. Worth a lint rule or a review checklist entry — this will bite someone. |
| **6** | **`/api/batch` is a multiplexer over the same CRUD handlers.** It *does* enforce the same API rules (verified), so nil rules cover it — but it's a transaction-holding surface with a ~128MB body limit. | 🟡 Low | Keep `Settings().Batch.Enabled = false` (the default) and assert it at boot alongside the API-rule assertion. Defence in depth, not a hole. |
| **7** | **No per-field `Unique`.** Uniqueness is a raw `CREATE UNIQUE INDEX` string on the collection. | 🟡 Low | Use `collection.AddIndex(name, true, cols, where)` in migrations. Actually an upgrade — partial and expression indexes come free. |
| **8** | **`number` fields are float64** (~2^53-1 safe integer range). | 🟡 Low | Never store identifiers or large integers in `number`. Use `text`. `GetInt64` exists but doesn't widen the underlying type. |
| **9** | **PB's `Config` has no logger/telemetry injection at all** — Sentry, Prometheus and OTel must all attach via `OnServe` middleware. | 🟡 Low | `apis.WrapStdMiddleware(otelhttp.NewMiddleware(...))` in `OnServe`; `apis.WrapStdHandler(promhttp.Handler())` behind `RequireSuperuserAuth`. Well supported, just not declarative. |
| **10** | **No trailing-slash normalisation** since v0.23. | 🟡 Low | Pick a convention (no trailing slash), enforce it in the OpenAPI generator, and cover it in the Playwright smoke gate. |
| **11** | **Automigrate writes `.go` files at runtime**, hooked to `OnCollection*Request`. | 🟡 Low | `Automigrate: cfg.Dev` only. Never in a container image. |
| **12** | **Cobra root sets `FParseErrWhitelist{UnknownFlags: true}`** — typo'd flags are silently ignored. | 🟡 Low | Validate flag values inside `RunE`; set `SilenceUsage: true` on MediGo commands. |
| **13** | testify is an indirect dep at v1.8.0. | 🟢 None | Promote to direct at the current version; MVS resolves it. No conflict. |

**Non-collisions, explicitly confirmed:**

- ✅ Cobra is genuinely PB's own dependency (v1.10.2) — `RootCmd` is a real `*cobra.Command`,
  and `medigo seed` / `medigo openapi` get a fully bootstrapped app for free. No Viper needed;
  `caarlos0/env` slots in cleanly at the composition root.
- ✅ templ has zero conflict with `tools/template`; the latter is an unregistered helper.
- ✅ Datastar SSE works from a PB route — raw `e.Response`, `Flush()`, and `Unwrap()` for
  `http.ResponseController`.
- ✅ Locking down the auto-API leaves **all internal Go record access untouched** — API rules
  are evaluated only in the HTTP layer.
- ✅ The admin dashboard survives the lockdown, because superusers bypass all rules.
- ✅ `tests.NewTestAppWithConfig` gives a genuinely isolated, `t.Parallel()`-safe throwaway
  database per test, out of the box.
- ✅ `samber/lo|do|ro|mo` are pure library choices — no interaction with PocketBase.

---

## 17. Recommended `main.go` skeleton

```go
package main

import (
	"embed"
	"log"
	"log/slog"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"medigo/internal/api"
	"medigo/internal/clicmd"
	"medigo/internal/config"
	"medigo/internal/logging"
	"medigo/internal/pblog"
	"medigo/internal/wire"

	_ "medigo/internal/migrations" // registers migrations via init()
)

//go:embed all:web/dist
var assetsFS embed.FS

func main() {
	cfg, err := config.Load()            // caarlos0/env/v11, validated
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	zl := logging.New(cfg)               // zerolog: the single app logger
	slog.SetDefault(slog.New(pblog.NewZerologHandler(zl))) // catches pre-bootstrap logs

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:       cfg.DataDir,
		DefaultDev:           cfg.Dev,
		DefaultEncryptionEnv: "MEDIGO_ENCRYPTION_KEY",
		HideStartBanner:      true,
	})

	// redirect PB's request-path logging into zerolog (see §13 for the caveats)
	pblog.Wrap(app, zl)

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangGo,
		Automigrate:  cfg.Dev,
		Dir:          "internal/migrations",
	})

	// --- settings that must be enforced from env, every boot ---
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return wire.ApplySettings(e.App, cfg)  // _logs off, OAuth2, SMTP, S3, rate limits, batch off
	})

	// --- safety assertions: refuse to boot with an open collection ---
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := wire.AssertLockedDown(se.App); err != nil {
			return err
		}
		return se.Next()
	})

	// --- routes ---
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		deps := wire.Build(se.App, cfg, zl)     // samber/do container

		se.Router.Bind(api.BlockAutoCollectionAPI())               // §7 mechanism B
		se.Router.BindFunc(apis.WrapStdMiddleware(deps.Otel))      // OTel
		se.Router.BindFunc(apis.WrapStdMiddleware(deps.Sentry))    // Sentry

		api.RegisterV1(se, deps)   // every hand-written /api/v1 endpoint
		api.RegisterWeb(se, deps)  // templ pages + Datastar SSE

		se.Router.GET("/metrics", apis.WrapStdHandler(deps.PromHandler)).
			Bind(apis.RequireSuperuserAuth())

		dist := apis.MustSubFS(assetsFS, "web/dist")
		se.Router.GET("/static/{path...}", apis.Static(dist, false)).Bind(apis.Gzip())

		return se.Next()
	})

	// --- domain hooks (bare, NOT *Request — see collision #5) ---
	wire.RegisterHooks(app, zl)

	// --- custom cobra subcommands ---
	app.RootCmd.AddCommand(clicmd.NewSeed(app))
	app.RootCmd.AddCommand(clicmd.NewOpenAPI(app))

	if err := app.Start(); err != nil {   // registers serve + superuser, then Execute()
		zl.Fatal().Err(err).Msg("medigo exited")
	}
}

var _ = http.StatusOK
```

### Open items to resolve before writing the spec

1. **Confirm the Go 1.27 move.** Everything else is downstream of this. If Go 1.26.5 is
   immovable, the PocketBase version must drop to v0.39.x and this dossier needs a re-verify
   pass against that tag.
2. **Ratify "Datastar SSE for all realtime"** and strike native realtime from the locked
   decisions.
3. **Decide the auth surface:** proxy `/api/collections/users/auth-*` (keeps MFA/OTP for free)
   vs. hand-write `/api/v1/auth/*` with `apis.RecordAuthResponse` (uniform DTOs, but MFA/OTP
   would need reimplementation). Recommendation: hand-write login/refresh/logout, proxy the
   MFA/OTP/OAuth2 flows.
4. **Decide whether the admin dashboard ships in production.** It's a locked decision to keep
   it, but note `-tags no_ui` exists if the threat model changes.

### Source map (for re-verification on upgrade)

| Topic | File |
|---|---|
| Bootstrap, `Config`, `Start`, `Execute` | `pocketbase.go` |
| `core.App` interface (~200 methods) | `core/app.go` |
| `BaseApp`, `initLogger`, `Logger()` | `core/base.go:375-385`, `:1451-1605` |
| Router, groups, middleware ordering | `tools/router/{router,group,route,event,error}.go` |
| Hook priority + replacement semantics | `tools/hook/hook.go:98-101` |
| Default middlewares + priorities | `apis/middlewares*.go` |
| Route registration for all built-ins | `apis/base.go:18-58` |
| **Record CRUD lockdown check** | `apis/record_crud.go:27-34`, `:52-55` |
| **`CanAccessRecord` nil-rule semantics** | `core/record_query.go` (`CanAccessRecord`) |
| **Realtime rule map** | `apis/realtime.go:605-651` |
| **Protected-file check (and its absent `else`)** | `apis/file.go:107-134` |
| Collection model, rules, `AddIndex` | `core/collection_model.go:352-430`, `:649-682` |
| Field structs | `core/field_*.go` |
| Transactions, `TxInfo` | `core/db_tx.go` |
| Record proxy | `core/record_proxy.go` |
| Auth options, OAuth2 config | `core/collection_model_auth_options.go` |
| Tokens | `core/record_tokens.go` |
| Filesystem | `tools/filesystem/{filesystem,file}.go` |
| Test helpers | `tests/{app,api}.go` |
| Migrations | `core/migrations_list.go`, `migrations/1640988000_init.go`, `plugins/migratecmd/` |
| Admin UI embedding / `no_ui` | `ui/embed.go`, `ui/embed_no_ui.go` |
