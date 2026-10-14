# Roadmap: authentication

## Goal

Anyone who reaches the REST API or the web UI must present a token. The token names the caller, and for REST writes that name is what becomes the assignee or comment author. Nothing about the CLI or MCP changes.

Tracker project: `PLAT`. Tracker issue: PLAT-6.

## Assumptions

The goal left these open. Each choice below is the simplest reading that meets it.

1. **Tokens are static and come from the environment.** `CODEPLAT_TOKENS` holds a JSON object that maps a principal name to its token, for example `{"worker-1":"<random string>"}`. Tokens never appear in a file in the repository. There is no user table, no expiry, no hashing and no roles.
2. **The principal's name is the assignee and comment author.** Every token has the same authority.
3. **REST: every route except `GET /healthz` needs a token.** The token is an `Authorization: Bearer` header.
4. **Web UI: every route needs a token, including the `GET /events` route that the live-updates work adds.** The token is the password of HTTP Basic credentials, and the username is ignored. Browsers ask for Basic credentials and resend them on every request, including `EventSource` requests, which cannot set headers.
5. **CLI and MCP are not authenticated.** They open the SQLite file directly, so the file's permissions are their trust boundary. They keep free-text assignee and author, as they do now. This is a deliberate exception to "every entrypoint".
6. **Environments.** `dev` stays open, so the README workflow works without setup. `test` is open by default, and the auth tests opt in. `prod` is closed: `auth.open` is absent there, which means false. If `prod` starts without `CODEPLAT_TOKENS`, every REST and web request is refused and the app logs a warning at startup.
7. **Name rules when a principal is present.** A write may name only the principal itself or leave the field empty. An empty field is filled with the principal's name. Any other value returns `ErrInvalid`, which is HTTP 400. The rules cover `CreateIssue` (assignee), `UpdateIssue` (assignee), `ClaimIssue` (assignee) and `AddComment` (author). The effect is that a REST caller can only assign work to itself. Assigning work to someone else is done from the CLI, which is unauthenticated.
8. **Refusals.** A missing or wrong token gets HTTP 401 with `WWW-Authenticate: Basic realm="codeplat"`, `Content-Type: application/json` and the body `{"error":"unauthorized"}`. Comparison is constant-time. Tokens are never logged.
9. **TLS is out of scope.** The servers keep listening on plain HTTP. A deployment that needs TLS must terminate it in front of them.
10. **Work in flight.** This roadmap assumes the event log (PLAT-7) has landed, so `Store`, both stores and `Service` already have `EventsSince` and `LastEventID`. Epic 1 edits `service.go` next to those methods. It also assumes the live-updates roadmap (`docs/roadmaps/live-updates.md`) is landed before issue 4.2 and issue 4.3 run. Issue 4.2 edits `cmd/web/main.go`, which live-updates 3.1 also edits. Issue 4.3 edits `ARCHITECTURE.md`, which live-updates 3.2 also edits. If the live-updates changes are not in yet, the worker rebases onto them. Issue 4.2 calls `web.NewHandler` with whatever arguments it takes at that point.

## Design

```
client ---- Bearer or Basic ----> httpauth.Require ----> core.Authenticator (internal/auth)
                                        |  principal in request context
                                        v
                       REST or web handler ----> core.Service
                                                   enforces assignee and author rules
CLI and MCP ---- no token ------------------------> core.Service (no principal, free text)
```

- **Port.** `core.Authenticator` is defined in `core` because `core` owns the rules that depend on the principal. `internal/auth` implements it.
- **Context.** `httpauth.Require` puts the principal in the request context with `core.WithPrincipal`. The Service reads it with `core.PrincipalFrom`. The transports never check names themselves, so REST and web cannot disagree.
- **Wiring.** `app.New` chooses the implementation from `cfg.Auth`. Each HTTP entrypoint then passes `a.Auth` to the transport.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Contracts | `core` has the principal type, the authenticator port and the name rules. `config` reads `auth.open` and `CODEPLAT_TOKENS`. | 2, parallel |
| 2. Parts | `internal/auth` and `internal/transport/httpauth` each pass their own tests. | 2, parallel |
| 3. Composition | `app.New` builds the authenticator from config, and `apptest` offers an open app and a closed app. | 1 |
| 4. Transports | REST and the web UI refuse requests without a token, and the docs say how to use tokens. | 3, parallel |

No issue has a dependency inside its epic.

Why four epics: the authenticator in `internal/auth` returns `core.Principal` and `core.ErrUnauthenticated`, so it cannot compile before epic 1. `app.New` imports `internal/auth`, so it cannot compile before epic 2. The transport tests need `apptest.NewWithAuth`, which needs epic 3.

Why epic 1 is parallel: `internal/core` and `config` do not import each other. Why epic 2 is parallel: `internal/auth` and `internal/transport/httpauth` do not import each other, and neither edits `core`, so the build checks of one never compile a file the other is changing. Why epic 4 is parallel: `rest`, `web` and the docs share no file.

The name rules sit in `core.Service` rather than in the transports, which is what `ARCHITECTURE.md` rule 4 requires.

---

## Epic 1: Contracts

Done when: `core` defines the principal, the authenticator port and the name rules, and `config` reads the auth settings. Nothing outside those two packages changes.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
  - Use the Go standard library only. Do not edit `go.mod`.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build` or `make test`. Other workers are using the binaries in `bin/`, and a repository-wide test may fail because of someone else's half-finished file.
```

### 1.1 Add principals and the authenticator port to core, and enforce the name rules in the Service

```
Epic: 1 Contracts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
`core` knows which principal a request runs as. When a principal is present, the Service makes every assignee and comment author either the principal's own name or empty, and fills an empty one with the name.

## Read first
- `internal/core/types.go`: `ErrInvalid`, and the `NewIssue`, `IssuePatch` and `NewComment` types.
- `internal/core/service.go`: `invalid`, `CreateIssue`, `UpdateIssue`, `ClaimIssue` and `AddComment`.
- `internal/core/service_test.go`: the package is `core_test`, and `TestServiceRules` shows how the service is built through `apptest.New(t)`.

## Change
- `internal/core/auth.go` (new file, package `core`): the types and functions in Contract.
- `internal/core/service.go`: the four rules in Contract, applied in `CreateIssue`, `UpdateIssue`, `ClaimIssue` and `AddComment`.
- `internal/core/service_test.go`: add `TestPrincipalRules` as a new function in the same file.

## Contract
In `internal/core/auth.go`, add exactly this (package `core`, imports `context` and `errors`):

// ErrUnauthenticated is returned by an Authenticator for a missing or unknown token.
var ErrUnauthenticated = errors.New("unauthenticated")

// Principal is the caller a transport authenticated. Name is the assignee or
// comment author that the caller's own writes use. A Principal in a context
// always has a non-empty Name.
type Principal struct {
	Name string
}

type principalKey struct{}

// WithPrincipal returns a copy of ctx that carries p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal carried by ctx, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Authenticator maps the credential a request presented to a principal. It is
// a port: transports call it, and internal/auth implements it.
type Authenticator interface {
	// Authenticate returns the principal for token. token is empty when the
	// request presented no credential. It returns an error wrapping
	// ErrUnauthenticated when the caller is not admitted.
	Authenticate(token string) (Principal, error)
}

In `internal/core/service.go`, when `PrincipalFrom(ctx)` returns ok, apply these rules. Trim whitespace from the supplied value first, as the existing code does. Every rule that fails returns `invalid(...)`, which wraps `ErrInvalid`. Without a principal in the context, behaviour is unchanged.

1. `CreateIssue`: if `in.Assignee` (after trimming) is not empty and is not `p.Name`, fail with `invalid(fmt.Sprintf("assignee must be %s or empty", p.Name))`. Otherwise set the issue's `Assignee` to `p.Name` when the trimmed value was empty. Do this before the existing checks.
2. `UpdateIssue`: if `patch.Assignee` is not nil, the trimmed value must be empty or `p.Name`, under the same rule and message. An empty value stays empty (unassign). Check this before calling the store.
3. `ClaimIssue`: the trimmed `assignee` argument must be empty or `p.Name`, under the same rule. Then set `assignee = p.Name`, so the existing "assignee is required" check passes. Do this before the existing check.
4. `AddComment`: the trimmed `in.Author` must be empty or `p.Name`, under the same rule and message. Then set `c.Author = p.Name`.

## Done when
- `go vet ./internal/core/` succeeds and `gofmt -l internal/core` prints nothing.
- `go test -race ./internal/core/` passes.
- `TestPrincipalRules` in `service_test.go`, which takes the service from `apptest.New(t).Service`, covers:
  - with `core.WithPrincipal(ctx, core.Principal{Name: "alice"})`, an `AddComment` with empty author stores author `alice`; with author `bob` it fails with `errors.Is(err, core.ErrInvalid)`;
  - the same context, a `CreateIssue` with assignee `bob` fails with `ErrInvalid`, and with assignee `""` stores assignee `alice`;
  - a `ClaimIssue` with assignee `""` leaves the issue `in_progress` and assigned to `alice`; with assignee `bob` it fails with `ErrInvalid` and the issue stays `todo`;
  - an `UpdateIssue` with patch assignee `bob` fails with `ErrInvalid`; with patch assignee `""` it succeeds and the issue is unassigned;
  - with a context that has no principal, `AddComment` with author `bob` and `UpdateIssue` with assignee `bob` both succeed.

## Out of scope
The transports, `internal/auth`, `config`, `README.md`, `ARCHITECTURE.md`. Do not change `ErrUnauthenticated` into an HTTP status; that mapping belongs to the transports.
```

### 1.2 Add the auth setting to config, and read tokens from CODEPLAT_TOKENS

```
Epic: 1 Contracts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
Every environment states whether requests must carry a token. An environment is closed unless its config says `open`. Tokens come only from the `CODEPLAT_TOKENS` environment variable, never from a config file.

## Read first
- `config/config.go`: the `Config` struct, `FromEnvironment`, `fromEnvironment`, `parse` and `validate`. Note that `parse` rejects unknown JSON fields.
- `config/config_test.go`: `TestFromEnvironmentResolvesStorePath` shows how a test uses `t.Setenv` and `CODEPLAT_DATA_DIR`.
- `config/dev.json` and `config/test.json`: the existing layout.

## Change
- `config/config.go`: add the field and the type in Contract, and extend `FromEnvironment`.
- `config/dev.json`: add the `auth` object in Contract as a line after `"clock"`. Do not change any other line.
- `config/test.json`: the same change as `dev.json`.
- `config/config_test.go`: add `TestFromEnvironmentReadsTokens` as a new function.
- `config/prod.json` is not changed. Its missing `auth` object means closed.

## Contract
In `config/config.go`, add this field to `Config` after `Clock`:

	Auth  Auth  `json:"auth"`

and this type:

// Auth says whether requests must present a token. Tokens are not read from
// config files.
type Auth struct {
	// Open admits every request without a token. False, the zero value, requires
	// a token, so an environment is closed unless its config says otherwise.
	Open bool `json:"open"`
	// Tokens maps each principal name to its token. FromEnvironment fills it
	// from CODEPLAT_TOKENS, a JSON object of name to token.
	Tokens map[string]string `json:"-"`
}

In `FromEnvironment`, after the existing logic has produced `cfg` and before it returns: if the environment variable `CODEPLAT_TOKENS` is not empty, decode it with `json.Unmarshal` into `cfg.Auth.Tokens`. On error return `Config{}, fmt.Errorf("config: CODEPLAT_TOKENS: %w", err)`. Add `encoding/json` to the imports if it is not already there. `Load` and `parse` do not read the environment.

In `config/dev.json` and `config/test.json`, the new object is `"auth": { "open": true },`.

## Done when
- `go vet ./config/` succeeds and `gofmt -l config` prints nothing.
- `go test -race ./config/` passes.
- `TestFromEnvironmentReadsTokens` covers: with `CODEPLAT_ENV=prod`, `CODEPLAT_DATA_DIR` set to `t.TempDir()`, `CODEPLAT_CONFIG` set to `""` and `CODEPLAT_TOKENS` set to `{"alice":"a-token"}`, `FromEnvironment` returns `Auth.Open == false` and `Auth.Tokens["alice"] == "a-token"`; with `CODEPLAT_ENV=dev` it returns `Auth.Open == true`; with `CODEPLAT_TOKENS` set to `not json` it returns an error.
- `TestEveryEmbeddedConfigLoads` still passes.

## Out of scope
`internal/app`, `internal/apptest`, any transport, `README.md`. Do not add validation of token contents here; `internal/auth` does that.
```

---

## Epic 2: Parts

Done when: `internal/auth` implements the authenticator port, and `internal/transport/httpauth` wraps a handler with it. Both compile against `core` as it stands after epic 1, and neither is wired anywhere yet.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate.
  - Use the Go standard library only. Do not edit `go.mod`.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build` or `make test`.
- Do not start any HTTP server in a test. Use `httptest.NewRecorder` and `httptest.NewRequest`.
```

### 2.1 Implement the open and static authenticators in internal/auth

```
Epic: 2 Parts
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
Two implementations of `core.Authenticator`: `Open`, which admits every request and names no one, and `Static`, which admits the tokens in a fixed table.

## Read first
- `internal/core/auth.go`: `Authenticator`, `Principal` and `ErrUnauthenticated`, which epic 1 added.
- `internal/store/memory/memory.go`: an example of a small package with a constructor and a doc comment on each exported name.

## Change
- `internal/auth/auth.go` (new, package `auth`): the types and functions in Contract.
- `internal/auth/auth_test.go` (new, package `auth`): the tests in Done when.

## Contract
package auth

import "codeplat/internal/core"

// Open admits every request and identifies no one, so a write keeps the
// free-text assignee and author it would have without authentication. The dev
// and test environments use it.
type Open struct{}

// Authenticate ignores the token and returns the zero Principal and no error.
func (Open) Authenticate(string) (core.Principal, error)

// Static admits a request whose token equals one of a fixed table of tokens.
// Each entry names one principal.
type Static struct {
	entries []entry // unexported; sorted by name
}

type entry struct {
	name  string
	token string
}

// NewStatic returns an authenticator for tokens, which maps each principal
// name to its token. It returns an error wrapping nothing in particular, with
// a message that names the problem, if a name is empty or contains
// whitespace, a token is empty, or two names have the same token. An empty
// map is valid and admits nothing.
func NewStatic(tokens map[string]string) (*Static, error)

// Authenticate returns the principal whose token equals token. An empty token,
// or one that matches no entry, returns the zero Principal and an error
// wrapping core.ErrUnauthenticated.
func (s *Static) Authenticate(token string) (core.Principal, error)

Implementation rules:
- In `Authenticate`, compare with `crypto/subtle.ConstantTimeCompare` against every entry, and do not return early when an entry matches. Keep a found flag and a found principal, and return after the loop. An empty token returns before the loop, because an empty token must never match.
- In `NewStatic`, copy the map into a slice and sort it by name with `slices.SortFunc` so the order is deterministic.

## Done when
- `go vet ./internal/auth/` succeeds and `gofmt -l internal/auth` prints nothing.
- `go test -race ./internal/auth/` passes.
- The tests cover:
  - `Open{}.Authenticate("")` and `Open{}.Authenticate("anything")` both return no error and a Principal with an empty Name;
  - with `NewStatic(map[string]string{"alice": "a1", "bob": "b1"})`, `"a1"` returns Name `alice` and `"b1"` returns Name `bob`;
  - `""`, `"a"` (a prefix of `"a1"`), `"a10"` and `"x"` each return an error for which `errors.Is(err, core.ErrUnauthenticated)` is true;
  - `NewStatic` returns an error for an empty name, for a name `"a b"`, for an empty token, and for two names with the same token;
  - `NewStatic(map[string]string{})` returns no error, and `Authenticate("x")` fails.

## Out of scope
`internal/transport`, `internal/app`, `config`, `core`. Do not read the environment in this package; the tokens are passed in.
```

### 2.2 Add the authentication middleware in internal/transport/httpauth

```
Epic: 2 Parts
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
A function that wraps an `http.Handler`. It asks a `core.Authenticator` about each request's credential, then either calls the wrapped handler with the principal in the request context, or refuses the request with 401.

## Read first
- `internal/core/auth.go`: `Authenticator`, `Principal`, `WithPrincipal`.
- `internal/transport/rest/rest.go`: `respond` and `write`, to see the JSON error shape `{"error": "..."}` that this package must match.

## Change
- `internal/transport/httpauth/httpauth.go` (new, package `httpauth`): the function in Contract.
- `internal/transport/httpauth/httpauth_test.go` (new, package `httpauth_test`): the tests in Done when. Define a fake `core.Authenticator` in the test file. It accepts only the token `good`, and it returns `Principal{Name: "alice"}`.

## Contract
// Package httpauth admits the HTTP requests whose credential an
// core.Authenticator accepts. The REST and web transports share it.
package httpauth

// Require returns a handler that authenticates each request, then calls next.
//
// The credential is the remainder of an "Authorization: Bearer " header. If
// there is no such header, it is the password of HTTP Basic credentials, and
// the username is ignored. Basic credentials let browsers prompt for the token
// and resend it on every request, including EventSource requests, which
// cannot set headers.
//
// The Authenticator is called with the credential, or with "" when none was
// sent. Any error from it refuses the request. A refused request gets status
// 401, the header `WWW-Authenticate: Basic realm="codeplat"`, the header
// `Content-Type: application/json`, the body `{"error":"unauthorized"}` and a
// newline, and next is not called.
//
// An admitted request whose principal has a non-empty Name goes to next with
// that principal in its context, set by core.WithPrincipal. A principal with
// an empty Name, which is what Open returns, leaves the context unchanged.
func Require(auth core.Authenticator, next http.Handler) http.Handler

Use `strings.CutPrefix` for the Bearer header, and trim spaces from the remainder. Use `r.BasicAuth()` for the Basic form. Write the JSON body with `json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})`.

## Done when
- `go vet ./internal/transport/httpauth/` succeeds and `gofmt -l internal/transport/httpauth` prints nothing.
- `go test -race ./internal/transport/httpauth/` passes.
- The tests cover:
  - no credential: 401, the `WWW-Authenticate` header with the exact value above, and the JSON body; `next` is not called;
  - `Authorization: Bearer good`: next is called, and `core.PrincipalFrom` on its request context returns Name `alice`;
  - `Authorization: Bearer bad`: 401, and next is not called;
  - HTTP Basic with username `anyone` and password `good`: next is called with the principal;
  - an authenticator that returns the zero Principal and no error for any token: next is called and `core.PrincipalFrom` reports false.

## Out of scope
Wiring `Require` into any transport or entrypoint, the `core` package, `config`, `internal/auth`.
```

---

## Epic 3: Composition

Done when: `app.New` builds the authenticator from config, and `apptest` offers an open app and a closed app. Existing tests still pass unchanged.

### 3.1 Choose the authenticator in app.New, and give apptest an open app and a closed app

```
Epic: 3 Composition
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- Business rules live in `internal/core`. `app.New` is the only place that chooses an implementation from config.
- Other workers are editing transport packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build` or `make test`.
- Use the Go standard library only. Do not edit `go.mod`.

## Goal
`app.New` sets `App.Auth` to the authenticator the config chooses. `apptest` can build the test app open, as now, or closed, with the test tokens.

## Read first
- `internal/app/app.go`: `App`, `New`, and the order in which `New` opens the store. It opens the store before anything else, so an error after that point must close the store.
- `internal/apptest/apptest.go`: `New`.
- `config/config.go`: the `Auth` type and `Config.Auth` from epic 1.
- `internal/auth/auth.go`: `Open` and `NewStatic` from epic 2.

## Change
- `internal/app/app.go`: add the field and the code in Contract.
- `internal/apptest/apptest.go`: add the variable and the function in Contract, and make `New` use the shared helper.
- `internal/app/app_test.go` (new, package `app_test`): the tests in Done when. Use `config.Load("test")` and `app.New`, not a store or a service built by hand.

## Contract
In `internal/app/app.go`, add the field `Auth core.Authenticator` to `App`, after `Service`. In `New`, after the store is created and before `core.NewService` is called:

	if cfg.Auth.Open {
		a.Auth = auth.Open{}
	} else {
		s, err := auth.NewStatic(cfg.Auth.Tokens)
		if err != nil {
			a.Close()
			return nil, fmt.Errorf("app: auth: %w", err)
		}
		if len(cfg.Auth.Tokens) == 0 {
			a.Log.Warn("auth is closed but CODEPLAT_TOKENS is empty: every REST and web request will be refused")
		}
		a.Auth = s
	}

Import `codeplat/internal/auth`. Keep the store's `closers` handling as it is.

In `internal/apptest/apptest.go`:

	// Tokens are the test tokens. A closed test app admits exactly these.
	var Tokens = map[string]string{"alice": "alice-token", "bob": "bob-token"}

	// New returns a fresh app in the test environment, open as config/test.json
	// says, so requests need no token. It is closed when the test ends.
	func New(t testing.TB) *app.App

	// NewWithAuth returns a fresh app like New, but closed: requests need a
	// token from Tokens. It is closed when the test ends.
	func NewWithAuth(t testing.TB) *app.App

Both functions give the app a copy of `Tokens` (use `maps.Clone`). `NewWithAuth` sets `cfg.Auth.Open = false` after loading the config. Both call `t.Helper()` and `t.Fatal` on error, as `New` does now.

## Done when
- `go vet ./internal/app/ ./internal/apptest/` succeeds and `gofmt -l internal/app internal/apptest` prints nothing.
- `go test -race ./internal/app/ ./internal/apptest/ ./internal/transport/rest/ ./internal/transport/cli/ ./internal/transport/mcp/ ./internal/store/...` passes. The existing tests must pass without changes, because `apptest.New` stays open.
- The tests in `app_test.go` cover:
  - `apptest.NewWithAuth(t).Auth.Authenticate("alice-token")` returns Name `alice`, and `Authenticate("")` returns an error;
  - `apptest.New(t).Auth.Authenticate("")` returns no error;
  - `app.New` with a config from `config.Load("test")` where `Open` is false and `Tokens` is `{"a": ""}` returns an error and no app.

## Out of scope
Any transport, `cmd/`, `config`, `internal/auth`, `internal/core`. Do not add a test helper that builds a store or a service by hand.
```

---

## Epic 4: Transports

Done when: REST refuses requests without a token except `GET /healthz`, the web UI refuses requests without a token, and the docs describe the setup. The three issues share no file.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate beyond decoding their own input.
  - Tests get the app from `apptest.New(t)` or `apptest.NewWithAuth(t)`. Do not construct a store or a service by hand.
  - Use the Go standard library only. Do not edit `go.mod`.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build` or `make test`. Do not start a server on a port in a test.
- The live-updates work may be in progress. Do not edit `internal/transport/web/web.go`, `page.html`, `feed.go` or `sse/`.
```

### 4.1 Require a token on the REST API, except GET /healthz

```
Epic: 4 Transports
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
Every REST route other than `GET /healthz` refuses a request that does not carry a token the app accepts. A request with a valid token reaches the handler with its principal in the context, so the Service applies the name rules.

## Read first
- `internal/transport/rest/rest.go`: `NewHandler`, `respond`.
- `internal/transport/rest/rest_test.go`: the `do` helper, which sets no headers, and `TestREST`.
- `cmd/rest/main.go`: the call to `rest.NewHandler`.
- `internal/transport/httpauth/httpauth.go`: `Require`.
- `internal/apptest/apptest.go`: `NewWithAuth` and `Tokens`.

## Change
- `internal/transport/rest/rest.go`: change `NewHandler` as in Contract.
- `internal/transport/rest/rest_test.go`: change the existing `rest.NewHandler(a.Service, a.Log)` call to `rest.NewHandler(a.Service, a.Auth, a.Log)`, and add `TestRESTRequiresToken` as a new function.
- `cmd/rest/main.go`: change the call to `rest.NewHandler(a.Service, a.Auth, a.Log)`.

## Contract
In `rest.go`:

func NewHandler(svc *core.Service, auth core.Authenticator, log *slog.Logger) http.Handler

Keep every route registration on the inner `mux` exactly as it is now, with the existing `GET /healthz` line removed from it. Then:

	top := http.NewServeMux()
	top.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		h.write(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	top.Handle("/", httpauth.Require(auth, mux))
	return top

Import `codeplat/internal/transport/httpauth` and `codeplat/internal/core` (already imported).

## Done when
- `go vet ./internal/transport/rest/` succeeds and `gofmt -l internal/transport/rest cmd/rest` prints nothing.
- `go test -race ./internal/transport/rest/` passes.
- `go build -o /dev/null ./cmd/rest` succeeds.
- `TestRESTRequiresToken`, built from `apptest.NewWithAuth(t)`, covers:
  - `GET /healthz` with no header returns 200;
  - `GET /projects` with no header returns 401, with the `WWW-Authenticate` header and the JSON body from `httpauth`;
  - `GET /projects` with `Authorization: Bearer ` followed by `apptest.Tokens["alice"]` returns 200;
  - a wrong token returns 401;
  - with alice's token: `POST /projects` and `POST /projects/PLAT/issues` with assignee `""` succeed; `POST /issues/PLAT-1/comments` with body `{"author":"bob","body":"x"}` returns 400; the same with author `""` returns 201 and the comment's author is `alice`;
  - with bob's token, `POST /issues/PLAT-1/claim` with body `{"assignee":""}` returns 200 and the issue's assignee is `bob`.
  Add a helper in the test file that takes a token and sets the header. The existing `do` helper does not set headers, so do not change its signature.

## Out of scope
The routes' behaviour apart from auth, the web UI, the CLI, MCP, `config`, `README.md`, `ARCHITECTURE.md`.
```

### 4.2 Require a token on the web UI

```
Epic: 4 Transports
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
Every route of the web UI, including routes added later such as `GET /events`, refuses a request without a token the app accepts. The wrapping happens where the entrypoint builds the handler, so `web.go` does not change.

## Read first
- `cmd/web/main.go`: the call to `web.NewHandler`.
- `internal/transport/web/web_test.go`: its package clause and how it gets the handler from `apptest.New(t)`.
- `internal/transport/httpauth/httpauth.go`: `Require`.
- `internal/apptest/apptest.go`: `NewWithAuth` and `Tokens`.

## Change
- `cmd/web/main.go`: wrap the handler as in Contract, and add the import `codeplat/internal/transport/httpauth`.
- `internal/transport/web/auth_test.go` (new): the tests in Done when. Use the same package clause as `web_test.go`.

## Contract
In `cmd/web/main.go`, replace the argument `web.NewHandler(...)` in the `httprun.Serve` call with:

	httpauth.Require(a.Auth, web.NewHandler(a.Service, a.Log))

Keep the arguments that `web.NewHandler` takes at this point. If the live-updates work has changed its signature by the time this issue runs, keep the arguments it has then. Change only the wrapping. Do not edit `internal/transport/web/web.go`.

## Done when
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web cmd/web` prints nothing.
- `go test -race ./internal/transport/web/` passes.
- `go build -o /dev/null ./cmd/web` succeeds.
- `auth_test.go` builds the handler as `httpauth.Require(a.Auth, web.NewHandler(a.Service, a.Log))` with `a := apptest.NewWithAuth(t)`, and covers:
  - `GET /` with no credential returns 401 with `WWW-Authenticate: Basic realm="codeplat"`;
  - `GET /` with HTTP Basic credentials, username `anyone` and password `apptest.Tokens["bob"]`, returns 200 and the body contains `No projects yet`;
  - `GET /` with `Authorization: Bearer ` followed by `apptest.Tokens["alice"]` returns 200;
  - a wrong password returns 401.
  Use `httptest.NewRequest` and set the header with `req.SetBasicAuth`.
- You cannot check in a browser that the page prompts for credentials and that `EventSource` reconnects with them. Say so in your report, so a person knows that step is still open.

## Out of scope
`internal/transport/web/web.go`, `page.html`, `feed.go`, the `sse` package, anything that changes what the pages show, `README.md`, `ARCHITECTURE.md`.
```

### 4.3 Document authentication in README.md and ARCHITECTURE.md

```
Epic: 4 Transports
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)
- This issue edits documents only. Do not change any Go file.
- The live-updates work edits `ARCHITECTURE.md` as well. If its changes are not in the file yet when you start, write your edits around them and leave its text alone. If they are in, keep them.

## Goal
A reader of `README.md` can run the REST API and web UI with tokens, and `ARCHITECTURE.md` describes the auth parts without a rule that is now false.

## Read first
- `README.md`: the "Run" block and the "Environments" section.
- `ARCHITECTURE.md`: the code block under "Shape", the "Decisions" list, and the "Not built yet" bullet, which lists `auth` first.

## Change
- `README.md`: in the "Environments" section, add a paragraph in the Contract wording.
- `ARCHITECTURE.md`: the three edits in Contract.

## Contract
`README.md`, add this paragraph at the end of the "Environments" section:

Requests need a token outside `dev` and `test`. Set `CODEPLAT_TOKENS` to a JSON object that maps a name to a token, for example `CODEPLAT_TOKENS='{"worker-1":"<long random string>"}'`. The name becomes the assignee or comment author of that token's writes. `bin/codeplat-rest` takes the token as `Authorization: Bearer <token>`. `bin/codeplat-web` asks for it as the password of a login; the username is ignored. The CLI and MCP do not use tokens. They open the database file directly. Without `CODEPLAT_TOKENS`, `prod` refuses every REST and web request.

`ARCHITECTURE.md`:
1. In the code block under "Shape", add these two lines directly above the line that starts with `config/`, using the same column alignment as the lines around them:
internal/auth                  core.Authenticator implementations: open, and static tokens
internal/transport/httpauth    token check shared by the REST and web transports
2. Add this bullet to "Decisions", directly after the "Claiming" bullet:
- **Auth:** `core.Authenticator` maps a token to a `core.Principal`. `httpauth.Require` runs it on every REST route except `GET /healthz`, and on every web route, and puts the principal in the request context. With a principal, `core.Service` accepts an assignee or comment author only if it is the principal's name or empty, and fills an empty one with the name. Tokens come from `CODEPLAT_TOKENS`, never from a config file. The CLI and MCP have no principal.
3. In the "Not built yet" bullet, remove `auth, ` from the list, so the list starts with `pagination`.

## Done when
- `grep -c "CODEPLAT_TOKENS" README.md` prints at least 1.
- `grep -c "httpauth" ARCHITECTURE.md` prints 2.
- `grep -c "auth, pagination" ARCHITECTURE.md` prints 0.
- `grep -c "^- \*\*Auth:\*\*" ARCHITECTURE.md` prints 1.

## Out of scope
Every Go file, `CLAUDE.md`, `prompts/`, `docs/roadmaps/`.
```

---

## Check of the plan

- **Collisions within an epic.** Epic 1: `core/auth.go` (new), `core/service.go`, `core/service_test.go` belong to 1.1 only; `config/*` belongs to 1.2 only. Epic 2: each issue creates its own new package. Epic 4: `rest/*` and `cmd/rest` belong to 4.1; `cmd/web/main.go` and `web/auth_test.go` belong to 4.2; docs belong to 4.3.
- **Compile sets.** 1.1 compiles `core`, which nothing else in epic 1 edits. 1.2 compiles `config` only. 2.1 compiles `core` and `internal/auth`; 2.2 compiles `core` and `httpauth`; neither edits `core`. 4.1 compiles `rest`, `httpauth`, `core` and `apptest` (through its test), none of which epic 4 edits otherwise. 4.2 compiles `web`, `httpauth` and `apptest`, and `web.go` is not edited.
- **Ports.** No issue uses a port or a fixed path. All HTTP checks use `httptest` recorders.
- **Could start early.** Epic 1 needs only the event-log state, which is already in the tree. Epic 2 needs only epic 1's `core/auth.go`. Epic 4's transport issues need only epic 3's `apptest.NewWithAuth`.
- **Adjacent epics.** Epic 2 and epic 3 cannot merge: `app.New` must import the authenticator package, so it cannot be in the same issue without making that issue four files. Epic 3 and epic 4 cannot merge: the transport tests need `apptest.NewWithAuth`.
- **Dependency check.** The roadmap declares no dependency, so there is nothing to re-check.
- **Shared files with the live-updates roadmap.** `cmd/web/main.go` (issue 4.2), `ARCHITECTURE.md` (issue 4.3) and `internal/transport/web/web_test.go` (not edited here, but its `NewHandler` call is what 4.2 mirrors). These are the only points of contact; they are called out in the assumptions.

## Shape

- 4 epics, 8 issues in total.
- Epic 1 Contracts: 2 issues, parallel.
- Epic 2 Parts: 2 issues, parallel.
- Epic 3 Composition: 1 issue.
- Epic 4 Transports: 3 issues, parallel.
- Dependencies: none declared. Epic order alone sequences the work.

Files each epic changes:

- **Epic 1:** `internal/core/auth.go` (new), `internal/core/service.go`, `internal/core/service_test.go`, `config/config.go`, `config/config_test.go`, `config/dev.json`, `config/test.json`.
- **Epic 2:** `internal/auth/auth.go` (new), `internal/auth/auth_test.go` (new), `internal/transport/httpauth/httpauth.go` (new), `internal/transport/httpauth/httpauth_test.go` (new).
- **Epic 3:** `internal/app/app.go`, `internal/app/app_test.go` (new), `internal/apptest/apptest.go`.
- **Epic 4:** `internal/transport/rest/rest.go`, `internal/transport/rest/rest_test.go`, `cmd/rest/main.go`, `cmd/web/main.go`, `internal/transport/web/auth_test.go` (new), `README.md`, `ARCHITECTURE.md`.

The roadmap is at `/home/koni/coding-platform-for-agents/docs/roadmaps/authentication.md`.

## Assumptions

These are the same as the Assumptions section at the top of the roadmap:

1. Tokens are static and come from `CODEPLAT_TOKENS`, a JSON object of name to token. They never come from a config file. There are no roles, expiry, hashing or user table.
2. The principal's name becomes the assignee or comment author. A write may name only the principal itself or leave the field empty. Any other value is a 400.
3. REST: every route except `GET /healthz` needs a token, sent as `Authorization: Bearer`.
4. Web: every route needs a token, sent as the password of HTTP Basic credentials. That covers the `GET /events` route the live-updates work adds. A person must confirm in a browser that EventSource reconnects with the credentials.
5. CLI and MCP stay unauthenticated, because they open the SQLite file directly. The file's permissions are their trust boundary.
6. `dev` and `test` are open by default; `prod` is closed. `prod` without `CODEPLAT_TOKENS` refuses every REST and web request and logs a warning.
7. TLS is out of scope.
8. This roadmap assumes PLAT-7 (the event log) has landed. It also assumes live-updates is landed before issues 4.2 and 4.3, since both touch files that live-updates also edits (`cmd/web/main.go` and `ARCHITECTURE.md`).

I did not create any issues in the tracker and did not edit any code. Next I add the single comment to PLAT-6.


<invoke name="Bash">
<parameter name="command">cd /home/koni/coding-platform-for-agents && bin/codeplat issue comment PLAT-6 -author plan-p6 -body "Roadmap written to docs/roadmaps/authentication.md"