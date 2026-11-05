# Go backend rules (`internal/` and `cmd/`)

Read `.claude/CLAUDE.md` first; these rules add to it.

## Toolchain & style
- Go 1.27, module `github.com/carloseduardo21/apisix-dashboard`, `CGO_ENABLED=0`.
- `gofmt`/`goimports` formatting; imports grouped stdlib / third-party / internal.
- golangci-lint (gosec, errcheck, govet, staticcheck, revive) must be clean. A
  `//nolint:<linter> // <reason>` is allowed only with a concrete justification on the
  same line (e.g. the loopback-only `InsecureSkipVerify` in `healthcheck`).
- Wrap errors with context: `fmt.Errorf("load certificate %d: %w", id, err)`; compare
  with `errors.Is/As`. Do not ignore errors silently; if ignoring is intentional, write
  `_ =` and a comment.
- Code comments and log messages in English. User-facing strings returned by the API
  are PT-BR today and will move to i18n keys — keep them in one place per handler.

## HTTP layer (`internal/server`)
- stdlib `http.ServeMux` with method patterns (`"POST /api/x/{id}"`). No frameworks.
- **Every route** is registered through `s.require(access{...}, handler)` except
  `/api/health` and `POST /api/auth/login`. Pick the least privilege:
  `access{}` (any role, read), `access{write: true}` (operator/admin),
  `access{admin: true}`. `pending`/`mustChange` only for auth-flow endpoints.
- Non-GET requests are CSRF-checked by `require`; do not bypass it.
- Decode bodies with `readJSON` (1 MiB `MaxBytesReader`); parse path values strictly
  (`strconv.ParseInt`, regex allowlists). Reject unknown enum values.
- Responses: `writeJSON`, `writeError(status, safeMessage)`, `internalError(w, err)`
  (logs + generic 500), `badGateway(w, err)` for Admin API failures. Never send
  `err.Error()` from internal/library errors; only `auth.ValidationError`,
  `kong.ValidationError` and other explicitly user-safe errors.
- Relay Admin API responses only through `writeAdminResponse` (masks secrets, never
  relays non-JSON bodies).

## APISIX access (`internal/apisix`)
- All Admin API calls go through `apisix.Client.Do` with a validated `apisix.Path`.
  Never build Admin API URLs from raw user input.
- Adding a resource: update the `resources` allowlist, `ParsePath` shape rules,
  masking (`sensitiveKeys` / catalog `encrypt_fields`), audit action names, and add
  tests in `mask_test.go` / a new path test.
- Writes must: load the current object, `Unmask` the body, write, and audit masked
  before/after (`recordChange`).
- Treat upstream nodes, plugin configs and `secrets` URIs as **untrusted config that
  APISIX will act on** (SSRF/RCE surface): validate, and flag dangerous plugins in
  reviews (`serverless-*`, loggers to arbitrary hosts, `forward-auth`, etc.).

## Security primitives
- Randomness: `crypto/rand` only (`secure.RandomToken`). Never `math/rand` for
  anything security related.
- Secrets at rest: `secure.AEAD` with additional data binding the owner record.
- Passwords: `auth.HashPassword`/`VerifyPassword` (argon2id); constant-time compares
  (`crypto/subtle`).
- Config secrets via `config.secret("DASH_X")` so `DASH_X_FILE` works. Document every
  new variable in `.env.example` and the README.
- Logging: `log/slog` JSON. **Never log** secrets, tokens, cookies, passwords, TOTP
  codes, request/response bodies of the Admin API, or the admin key. Log ids and
  status codes instead.
- Outbound HTTP clients: explicit timeouts, `tls.Config{MinVersion: tls.VersionTLS12}`,
  response size limits (`io.LimitReader`).

## Persistence (`internal/store`)
- Parameterized SQL only (`?` placeholders) — never string-concatenate SQL.
- Schema changes must be additive and idempotent (`CREATE TABLE IF NOT EXISTS`,
  guarded `ALTER TABLE`); existing databases must keep working after upgrade.
- Audit records are append-only; never add update/delete paths for them.

## Tests
- Table-driven tests, `t.Run` subtests, `t.Parallel()` where safe.
- Handlers: `httptest.NewRecorder` / `httptest.NewServer` with a fake Admin API;
  cover authz (each role), CSRF, validation errors, and that secrets never appear in
  responses or audit output.
- No network access in unit tests; use temp dirs for SQLite (`t.TempDir()`).

## Commands
```bash
go build ./... && go vet ./... && go test ./...
go test -race ./...
govulncheck ./...
```
