# Dashboard for Apache APISIX

Self-hosted web dashboard for the **Apache APISIX Admin API** (traditional mode, etcd),
designed for **PCI DSS 4.0** environments and Docker Swarm. It ships as a **single Go
binary** with the React SPA embedded. It is the control plane of an entire API gateway:
treat every change as security-sensitive.

- Repository / Go module: `github.com/carloseduardo21/apisix-dashboard`
- Public name: "<Name> for Apache APISIX" (ASF trademark policy). Never brand it
  "APISIX Dashboard" or imply it is an Apache project.
- Docs language: **English** (all docs, code comments, commit messages).
  UI: currently PT-BR, **i18n is planned** (see "Roadmap / decisions").

Stack-specific rules live in `internal/CLAUDE.md` (Go, also applies to `cmd/`) and
`web/CLAUDE.md` (React/TypeScript). Read them before touching those trees.

## Architecture

```
browser ──HTTPS (cookie + CSRF)──▶ dashboard (Go) ──X-API-KEY──▶ APISIX Admin API :9180 ──▶ etcd
                                    │  sessions, MFA, RBAC, audit, secret masking
                                    ├─ SQLite (users, sessions, audit, ACME, Kong state)
                                    ├─ Let's Encrypt + Cloudflare DNS API (optional, ACME)
                                    └─ Kong Admin API, read-only (optional, migration)
```

- The **APISIX admin key lives only in the backend** (`internal/apisix.Client`). The
  browser only talks to `/api/*` on the same origin.
- The backend is a **filtering proxy**, not a passthrough: only allowlisted resources
  and validated path segments reach the Admin API (`internal/apisix/resources.go`).
- Secrets are **masked** (`******`) in every response and audit record, and restored
  from APISIX when an object is written back with the masked value (`Mask`/`Unmask`).
- State is in SQLite (pure Go, no CGO) → **one replica only**, pinned to the volume node.

### Request pipeline (`internal/server/server.go`)
`securityHeaders` (CSP, HSTS, XFO, nosniff, no-store on /api) → `http.CrossOriginProtection`
→ `ServeMux` → `s.require(access{...})` (session cookie → CSRF token on non-GET → MFA
done → forced password change → role) → handler.

## Repository layout

```
cmd/apisix-dashboard/   main (serve, healthcheck, version) + user CLI
internal/config         DASH_* env config; secrets accept *_FILE; PCI warnings
internal/server         HTTP API, auth middleware, security headers, SPA handler
internal/auth           argon2id passwords, TOTP, sessions, lockout, roles
internal/apisix         Admin API client, resource allowlist, masking, schema catalog
internal/acme           ACME DNS-01 issuer/renewer (Cloudflare) → APISIX SSL objects
internal/kong           Kong 3.x reader, translator, planner, applier with rollback
internal/audit          Audit trail (DB + JSON lines mirror for SIEM)
internal/store          SQLite schema and queries
internal/secure         AES-256-GCM, key loading, random tokens, token hashing
internal/web            embed.FS of the built SPA (generated, git-ignored)
web/                    React 19 + TypeScript (strict) + Mantine 9 + TanStack Query
deploy/                 Docker Swarm stack example
.claude/                Claude Code rules, settings, hooks, slash commands
```

## Features (complete inventory)

### Authentication & accounts
- Local login: username + password (argon2id), then **mandatory TOTP MFA** (RFC 6238,
  replay-protected, secret encrypted with AES-256-GCM). First login enrolls MFA via QR code.
- **No default credentials**: the first admin is created with the CLI.
- Passwords set by an admin are temporary → **forced change on first access**.
- Password policy: ≥12 chars, letters + digits, must not contain the username, cannot
  reuse the last 4 (PCI 8.3.6/8.3.7).
- **Account lockout** after N failed password/MFA attempts (default 6, 30 min).
- **Server-side sessions**: token stored hashed (SHA-256), idle timeout 15 min, max
  lifetime 12 h, pending-MFA timeout 5 min, background janitor purges expired ones.
  Cookie `HttpOnly`, `Secure`, `SameSite=Strict`; session id rotated at login.
- Self-service password change (revokes the user's other sessions).
- **Roles** (enforced in the backend): `viewer` (read APISIX config), `operator`
  (+ create/update/delete APISIX objects, Kong migration), `admin` (+ dashboard users,
  audit trail, ACME management).

### APISIX management (Admin API v3)
- CRUD for: routes, services, upstreams, consumers, consumer groups, SSL certificates,
  global rules, plugin configs, plugin metadata, stream routes, protos, secrets
  (Vault/AWS/GCP secret managers).
- **Consumer credentials** (APISIX 3.10+): multiple credentials per consumer
  (key-auth, basic-auth, jwt-auth, hmac-auth…).
- Per-resource forms for day-to-day fields + **raw JSON tab** on the same object +
  "Advanced" section generated from APISIX's own JSON Schemas (`/schema/*`).
- **Plugin editor** generated from each plugin's schema (`plugins?all=true`), so new
  APISIX plugins work automatically; sensitive fields render as password inputs;
  consumers use the auth plugins' `consumer_schema`. Links to the plugin docs.
- Service details panel with linked routes; SSL list shows SNIs, validity, origin
  (manual vs automatic) and SNI-overlap warnings.
- Stream routes detect and explain "stream mode disabled".
- List filters/pagination forwarded to the Admin API (`page`, `page_size`, `name`,
  `label`, `uri`); `force` delete supported.
- Secret masking uses APISIX's own `encrypt_fields` per plugin (cached catalog,
  refreshed every 10 min) plus a static list (passwords, private keys, tokens…).
  `$secret://` and `$env://` references are shown as-is.

### Overview
- Admin API reachability + latency (also shown in the header), object counts per
  resource, recent activity (admins), certificate expiry overview.
- Global search / command palette (Mantine Spotlight), light/dark theme.

### Automatic certificates (ACME, optional — enabled by `DASH_ACME_CLOUDFLARE_API_TOKEN`)
- Let's Encrypt (or any ACME CA) **DNS-01 via Cloudflare**, wildcards supported,
  ECDSA P-256 or RSA 2048, production/staging directories.
- Publishes certificates as APISIX `ssls` objects (no reload); hourly renewal check,
  renew 30 days before expiry (capped at 1/3 of validity), exponential backoff on failure.
- Live issuance log in the UI; TXT records always cleaned up; propagation check
  against the zone's authoritative NS.
- Fresh private key per issuance, never stored by the dashboard; ACME account key
  encrypted at rest.
- Auto-managed SSLs are **locked** against manual edit/delete in the generic screens.
- Can take over an existing SSL id ("replace") with zero downtime.

### Kong → APISIX migration (optional — enabled by `DASH_KONG_ADMIN_URL`; operator/admin)
- Reads Kong 3.x (DB mode) **read-only**, optional workspace and admin token.
- **Settings**: ID strategy/prefix, routes created disabled for cutover, conflict
  policy, unsupported/disabled plugin policy, websocket, host rewrites, extra labels,
  **custom plugin rules** (JSON templates with `${config.x|default}` / `${apisix.*}`).
- **Inventory** with per-item status (to migrate / migrated / changed in Kong /
  blocked) and translation warnings.
- **Preview**: exact JSON per object, side-by-side with the current APISIX object, masked.
- **Apply**: ordered writes (upstreams → services → consumers → credentials → routes →
  global rules), **automatic rollback** of the batch on failure, stale-plan protection
  (hash of Kong + APISIX state), idempotent re-runs, migration history.
- ~20 Kong plugins translated (auth, ACL, rate limiting, CORS, IP restriction,
  transformers, logging, cache, OIDC…).

### Audit trail (admin)
- Logins, logouts, failures, lockouts, MFA, user management (UI and CLI), every APISIX
  change with masked **before/after**, ACME issuance, Kong migrations/rollbacks; actor,
  IP, outcome. Filters (all / changes / access / failures), infinite scroll, diff view.
- Mirrored as JSON lines to stdout/file for SIEM shipping (`DASH_AUDIT_LOG`).

### Dashboard users (admin)
- Create (temporary password), change role, enable/disable, reset password, reset MFA,
  unlock, delete. Guard: the last enabled admin cannot be removed/demoted/disabled.

### CLI (same binary)
`serve` (default) · `user add|passwd|reset-mfa|unlock|list` · `healthcheck` (distroless
HEALTHCHECK) · `version`.

### Operations
- Config only via `DASH_*` env vars (see `.env.example`); every secret supports `_FILE`
  (Docker/Swarm secrets). Startup warnings when settings deviate from PCI DSS.
- Optional native TLS (≥1.2), custom CAs for Admin API / Kong / ACME, outbound proxy via
  `HTTPS_PROXY`. Graceful shutdown. Distroless non-root image with HEALTHCHECK.

## How to run, test, build

Toolchain: Go 1.27 (`~/.local/go/bin`), Node 24, Docker.

```bash
cp .env.example .env               # then edit; never commit .env
make build                         # web (npm ci + vite build) + Go binary → bin/
make run ARGS="user add -u admin -r admin"
make run                           # http://localhost:8080
cd web && npm run dev              # Vite on :5173, proxies /api → :8080
make test                          # go test ./...
make docker                        # distroless image
```

More targets (`make help`): `fmt`, `fmt-check`, `lint`, `lint-fast`, `test-race`,
`cover`, `vuln`, `secrets`, `check` (= everything CI runs), `dev` (go run + Vite),
`docker-build`. Prefer the Makefile over ad-hoc commands.

Linters: **golangci-lint** (`.golangci.yml`: gosec, errcheck, govet, staticcheck, revive,
…) and **Oxlint** (`web/.oxlintrc.json`, type-aware via typescript-go) + **Prettier**.
ESLint/typescript-eslint is not used: it does not support TypeScript 7. Suppressions
need a justification on the same line (`// #nosec Gxxx -- reason`,
`// oxlint-disable-next-line rule -- reason`).

## Mandatory rules

1. **Never hardcode secrets**, internal URLs, hostnames, IPs or admin keys. Everything
   comes from `DASH_*` env/`_FILE`. Examples use `example.com` / `change-me`.
2. **The APISIX admin key stays in the backend.** No endpoint, log line, error, audit
   record or frontend bundle may contain it. The browser never calls the Admin API.
3. **Every backend route is authenticated** through `s.require(access{...})` with the
   least privilege needed. Only `/api/health` and `/api/auth/login` are public. Writes
   need `write` (operator/admin); user/audit/ACME management needs `admin`.
4. **Validate all input** in every handler: decode with size limits (`readJSON`,
   `MaxBytesReader`), allowlists over denylists, validate path segments, ids, enums,
   URLs. New Admin API resources must be added to the allowlist **and** reviewed for
   masking.
5. **Errors never leak internals**: no stack traces, raw upstream bodies, file paths or
   `err.Error()` of internal failures to the client. Use `internalError` / `badGateway`
   (log server-side, generic message to the client); only typed validation errors are
   user-facing.
6. **Every state change is audited** (`audit.Logger.Record`) with masked before/after.
7. **No new dependency without justification** (Go or npm): why it is needed, why the
   stdlib/existing deps are not enough, maintenance/licence/security status. Prefer the
   stdlib. Pin exact versions.
8. **Do not change functional behavior without asking the maintainer** (security
   fixes included: propose first).
9. **Conventional Commits**: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `build:`,
   `ci:`, `chore:`, `perf:`, `style:`, `revert:`; scope optional (`feat(acme): …`);
   `!` / `BREAKING CHANGE:` for breaking changes. English, imperative mood.
10. Never read or print `.env`, `data/` (SQLite, `encryption.key`) or key/cert files.

## Definition of Done

A change is done only when all of the following pass locally (and in CI):

- [ ] `make fmt` leaves no diff; `make lint` (golangci-lint incl. gosec, Oxlint
      type-aware, Prettier check, `tsc --noEmit`) has **zero** findings.
- [ ] `make test` passes; new behavior has tests (table-driven in Go; handler tests via
      `httptest` for new/changed routes).
- [ ] `make vuln` (govulncheck + `npm audit --audit-level=high`) shows no new findings;
      `make secrets` (gitleaks) is clean.
- [ ] `make build` and `make docker` succeed.
- [ ] Security checklist reviewed for the change (auth, validation, masking, audit,
      error leakage) — use `/security-check`.
- [ ] Docs updated: README / `.env.example` (new `DASH_*` vars) / this file (new
      features or decisions) / CHANGELOG via Conventional Commits.

## Key decisions (keep unless the maintainer decides otherwise)

- Single binary, embedded SPA, stdlib `net/http` (Go 1.22+ routing), no web framework.
- SQLite (modernc, `CGO_ENABLED=0`), single replica; static distroless image.
- Server-side sessions + CSRF token (not JWT in the browser).
- Resource allowlist + masking/unmasking instead of a transparent Admin API proxy.
- Local auth now; **Keycloak/OIDC login is the next auth step** (local login kept as
  break-glass).
- PCI DSS requirements are mapped in the README; do not weaken defaults
  (session 15 m idle, lockout ≥30 m, ≤10 attempts, Secure cookie).

## Roadmap / pending decisions
- i18n of the UI (main languages) — strings must become translation keys.
- OIDC (Keycloak) login.
- Plugin metadata UI, Swarm service discovery for upstreams, config history/rollback.
