# Lintel for Apache APISIX

[![CI](https://github.com/carloseduardo21/apisix-dashboard/actions/workflows/ci.yml/badge.svg)](https://github.com/carloseduardo21/apisix-dashboard/actions/workflows/ci.yml)
[![CodeQL](https://github.com/carloseduardo21/apisix-dashboard/actions/workflows/codeql.yml/badge.svg)](https://github.com/carloseduardo21/apisix-dashboard/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/carloseduardo21/apisix-dashboard/badge)](https://scorecard.dev/viewer/?uri=github.com/carloseduardo21/apisix-dashboard)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A self-hosted web dashboard for the [Apache APISIX](https://apisix.apache.org/) Admin
API (traditional mode with etcd), built for **PCI DSS 4.0** environments and Docker
Swarm. It ships as a **single Go binary** with the web UI embedded, and talks only to
the Admin API. The APISIX admin key stays on the server and never reaches the browser.

```
browser ──HTTPS──▶ lintel (apisix-dashboard) ──X-API-KEY──▶ APISIX Admin API :9180 ──▶ etcd
                     │ sessions, MFA, RBAC, audit trail, secret masking
                     └ SQLite (users, sessions, audit, ACME and migration state)
```

> Lintel is an independent project. It is **not** affiliated with, endorsed by or
> part of the Apache Software Foundation. Apache, Apache APISIX and APISIX are
> trademarks of the Apache Software Foundation.

## Features

- **Strong local authentication**: argon2id passwords, mandatory TOTP MFA, account
  lockout, server-side sessions with idle and absolute timeouts, forced change of
  temporary passwords. No default credentials.
- **Role-based access**: `viewer`, `operator` and `admin`, enforced in the backend.
- **Full APISIX management** (Admin API v3): routes, services, upstreams, consumers
  and their credentials, consumer groups, SSL certificates, global rules, plugin
  configs, plugin metadata, stream routes, protos and secret managers.
- **Forms generated from APISIX's own JSON Schemas**, so new plugins and fields work
  without a dashboard upgrade, plus a raw JSON tab on every object.
- **Secret masking** in every response and in the audit trail, with transparent
  restore when an object is saved back with the masked value.
- **Audit trail** of every login, user change and APISIX change (masked
  before/after), mirrored as JSON lines for your SIEM.
- **Automatic certificates** (optional): Let's Encrypt DNS-01 through Cloudflare,
  published as APISIX SSL objects with no reload.
- **Kong → APISIX migration** (optional): read-only import from Kong 3.x with
  preview, ordered apply and automatic rollback.
- UI in English, Brazilian Portuguese and Spanish; light and dark themes.

## Install

Each [release](https://github.com/carloseduardo21/apisix-dashboard/releases) provides:

- **Container image** (linux/amd64, linux/arm64):
  `ghcr.io/carloseduardo21/apisix-dashboard:<version>`. Pin it by digest in production.
- **Binaries** for linux/amd64 and linux/arm64, with checksums and SPDX SBOMs.

Everything is signed with [cosign](https://github.com/sigstore/cosign) (keyless) and
has SLSA build provenance. See [docs/releasing.md](docs/releasing.md#verifying-a-release)
to verify a download before deploying it.

## Quick start (from source)

Requirements: Go 1.27+, Node 24+ and a reachable APISIX Admin API.

```bash
make build                                            # web UI + binary in bin/
export DASH_ADMIN_API_URL=http://127.0.0.1:9180
export DASH_ADMIN_API_KEY=change-me                   # your APISIX admin key
export DASH_COOKIE_SECURE=false                       # local HTTP testing only
./bin/apisix-dashboard user add -u admin -r admin     # prompts for the password
./bin/apisix-dashboard                                # http://localhost:8080
```

On first login, each user enrolls MFA with any TOTP authenticator app.

> Over plain HTTP (outside `localhost`) browsers do not send the `Secure` session
> cookie. Use `DASH_COOKIE_SECURE=false` only for local tests without TLS.

## Configuration

All configuration comes from environment variables (see [`.env.example`](.env.example)).
Every secret also accepts a `_FILE` suffix (Docker/Swarm secrets).

| Variable | Default | Description |
|---|---|---|
| `DASH_ADMIN_API_URL` | — | Admin API URL, e.g. `http://apisix:9180` (**required**) |
| `DASH_ADMIN_API_KEY` / `_FILE` | — | APISIX admin key. Optional only for test instances with `admin_key_required: false`; always set it in production |
| `DASH_ADMIN_API_CA_FILE` | — | CA bundle for an HTTPS Admin API |
| `DASH_ADMIN_API_TIMEOUT` | `10s` | Admin API call timeout |
| `DASH_LISTEN_ADDR` | `:8080` | Listen address |
| `DASH_TLS_CERT_FILE` / `DASH_TLS_KEY_FILE` | — | Enable native HTTPS (TLS 1.2+) |
| `DASH_DATA_DIR` | `./data` | Directory for the SQLite database and the generated key |
| `DASH_ENCRYPTION_KEY_FILE` | `$DASH_DATA_DIR/encryption.key` | AES-256 key (32 bytes, hex or base64) that encrypts TOTP secrets and the ACME account key. Generated on first start if not set: **back it up** |
| `DASH_SESSION_IDLE_TIMEOUT` | `15m` | Idle session timeout (PCI 8.2.8) |
| `DASH_SESSION_MAX_LIFETIME` | `12h` | Absolute session lifetime |
| `DASH_MAX_LOGIN_ATTEMPTS` | `6` | Failed password or MFA attempts before lockout (PCI 8.3.4: at most 10) |
| `DASH_LOCKOUT_DURATION` | `30m` | Lockout duration (PCI 8.3.4: at least 30 minutes) |
| `DASH_LOGIN_RATE_PER_MINUTE` | `20` | Password, MFA and password-change attempts allowed per client IP per minute (IPv6 grouped by /64); excess gets `429` |
| `DASH_LOGIN_CONCURRENCY` | `2` | Password hashes computed at the same time. Each argon2id hash uses 64 MiB: allow about `64 MiB × this + 64 MiB` of memory |
| `DASH_COOKIE_SECURE` | `true` | Send the session cookie over HTTPS only |
| `DASH_TRUSTED_PROXIES` | — | Comma-separated CIDRs/addresses of reverse proxies allowed to set `X-Forwarded-For` / `X-Real-IP`. The client IP (audit, sessions, rate limiting) is the rightmost address that is not one of them |
| `DASH_TRUST_PROXY_HEADERS` | `false` | **Deprecated**, use `DASH_TRUSTED_PROXIES`. When `true` without a list, only the direct peer is trusted (rightmost `X-Forwarded-For` entry) |
| `DASH_RESTRICTED_PLUGINS` | code-running plugins¹ | Plugins only admins may add or change (comma-separated; `none` for no restriction). The route `script` field is always restricted |
| `DASH_AUDIT_LOG` | `stdout` | Audit mirror as JSON lines: `stdout`, `none` or a file path |
| `DASH_ACME_CLOUDFLARE_API_TOKEN` / `_FILE` | — | Enables automatic certificates |
| `DASH_ACME_EMAIL` | — | Let's Encrypt account contact (optional) |
| `DASH_ACME_RENEW_BEFORE` | `720h` | Renewal lead time (capped at 1/3 of the certificate validity) |
| `DASH_ACME_DIRECTORY` / `DASH_ACME_STAGING_DIRECTORY` | Let's Encrypt | Production and staging ACME directories |
| `DASH_ACME_CA_FILE` | — | Extra CA to reach the ACME directory (internal CA) |
| `DASH_ACME_DNS_RESOLVER` | zone NS | `host:port` used to check TXT propagation |
| `DASH_ACME_PROPAGATION_TIMEOUT` | `2m` | Maximum wait for propagation before validation |
| `DASH_KONG_ADMIN_URL` | — | Kong Admin API, e.g. `http://kong-admin:8001`. Enables Kong migration |
| `DASH_KONG_ADMIN_TOKEN` / `_FILE` | — | Kong Admin API token, if required |
| `DASH_KONG_ADMIN_TOKEN_HEADER` | `Kong-Admin-Token` | Header that carries the token |
| `DASH_KONG_WORKSPACE` | — | Workspace (Kong Enterprise) |
| `DASH_KONG_CA_FILE` | — | CA bundle for an HTTPS Kong Admin API |
| `DASH_KONG_TIMEOUT` | `30s` | Kong call timeout |

In a container, the Go memory limit is set to 85% of the cgroup memory limit unless
`GOMEMLIMIT` is set. Outbound requests honour `HTTPS_PROXY`.

¹ `serverless-pre-function`, `serverless-post-function`, `ext-plugin-pre-req`,
`ext-plugin-post-req`, `ext-plugin-post-resp` and `body-transformer` (its templates
can run Lua): they execute arbitrary code inside the APISIX workers. The server logs a warning at startup for
every setting that deviates from PCI DSS defaults.

## Roles

| Role | Can |
|---|---|
| `viewer` | Read the APISIX configuration |
| `operator` | `viewer` + create, update and delete APISIX objects, run Kong migrations |
| `admin` | `operator` + manage dashboard users, read the audit trail, manage automatic certificates |

The last enabled admin cannot be removed, demoted or disabled.

`operator` is a privileged role: it controls where the gateway sends traffic and which
plugins run. Plugins that execute arbitrary code (`DASH_RESTRICTED_PLUGINS`) and route
scripts can only be added or changed by an `admin`; operators can still edit the rest
of those objects and remove them. A masked secret cannot be kept when the destination
it is sent to (e.g. a logger `uri`, an OIDC `discovery` URL, a secret manager `uri`)
changes in the same plugin: it must be entered again.

Hardening on the APISIX side: remove the plugins you do not use from `plugins:` in
`config.yaml` (especially the serverless and ext-plugin ones), change the default
`data_encryption.keyring`, and keep etcd and the Admin API on networks the gateway
cannot route to.

## Security controls and PCI DSS 4.0

| Requirement | Implementation |
|---|---|
| 3 / 8.3.2: protect sensitive data | Secrets are **masked** (`******`) in every response and in the audit trail, using the `encrypt_fields` each plugin declares to APISIX (including nested fields) plus a static list (passwords, SSL and mTLS private keys, Vault tokens…). Saving the masked value restores the original from APISIX. `$secret://` and `$env://` references are shown as-is |
| 7.2: least privilege | `viewer`, `operator` and `admin` roles enforced in the backend. Only allowlisted Admin API resources are reachable |
| 8.2.8: session timeout | 15 min idle timeout and 12 h absolute lifetime. Sessions are server-side and tokens are stored hashed |
| 8.3.1 / 8.4: MFA | Mandatory TOTP (RFC 6238) for every user, with replay protection. Secrets encrypted with AES-256-GCM |
| 8.3.4: account lockout | 30 min lockout after 6 failed password or MFA attempts, plus per-IP throttling of authentication attempts (audited as `auth.throttled`) so password spraying is slowed down |
| 8.3.5: first-use passwords | Passwords set by an admin are temporary and must be changed at first login |
| 8.3.6 / 8.3.7: passwords | At least 12 characters with letters and digits, must not contain the username, cannot reuse the last 4. Hashed with argon2id |
| 10.2: audit trail | Logins, logouts, failures, lockouts, MFA, user management (UI and CLI) and every APISIX change, with **before and after**, actor, IP and outcome |
| 10.3.3: protect audit logs | Events are mirrored as JSON lines to stdout or a file for SIEM shipping |
| 6.4: public-facing web apps | Strict CSP, CSRF protection (token + `Sec-Fetch-Site`/`Origin`), `HttpOnly`/`Secure`/`SameSite=Strict` cookies, HSTS, `X-Frame-Options` |

**Operator responsibilities:** ship the audit logs to a SIEM, back up the `/data`
volume and the encryption key, provide TLS (natively or at a proxy), keep the Admin
API on an internal network, and review access periodically (8.2.6: disable users
inactive for more than 90 days).

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## Docker and Swarm

```bash
make docker            # distroless, non-root image with HEALTHCHECK
```

See [`deploy/docker-stack.yml`](deploy/docker-stack.yml), which uses Swarm secrets for
the admin key, the encryption key and TLS. Because state lives in SQLite, run
**exactly one replica**, pinned to the node that holds the volume.

## CLI

The same binary provides the management CLI:

```
apisix-dashboard user add -u NAME -r ROLE      # password from prompt or stdin
apisix-dashboard user passwd -u NAME           # set a temporary password
apisix-dashboard user reset-mfa -u NAME
apisix-dashboard user unlock -u NAME
apisix-dashboard user list
apisix-dashboard healthcheck                   # used by the container HEALTHCHECK
apisix-dashboard version
```

## Forms and plugins

Each resource has a form for day-to-day fields and a **JSON** tab that edits the same
object. Fields APISIX knows but the form does not handle explicitly appear under
"Advanced", generated from APISIX's own schemas (`/apisix/admin/schema/*`).

Plugins use a form generated from each plugin's schema (`/apisix/admin/plugins?all=true`),
so they follow the APISIX version in use, including new plugins. Sensitive fields are
password inputs. Consumers use the authentication plugins' `consumer_schema`.

**Consumer credentials** (APISIX 3.10+) live in the consumer panel (row menu →
*Credentials*). A consumer can have several credentials (key-auth, basic-auth,
jwt-auth, hmac-auth…), e.g. one per application, so one can be rotated or revoked
without affecting the others. Secrets are masked and, when left as `******` on edit,
keep their current value.

## Automatic certificates (Let's Encrypt)

Lintel issues and renews certificates through the **DNS-01 challenge on Cloudflare**
and publishes them to APISIX as SSL objects, applied without a reload. No acme.sh,
certificate volume or `server` block in `config.yaml` is needed. Wildcards are
supported.

Each issuance, visible live on the **Automatic certificates** screen:

1. Order at the CA → `_acme-challenge` TXT record created at Cloudflare.
2. Wait for propagation on the zone's authoritative name servers → the CA validates.
3. Fresh key generated (ECDSA P-256 or RSA 2048) → certificate checked (covers the
   domains, matches the key) → `PUT /ssls/{id}`.
4. TXT record removed, including when issuance fails.

Renewal happens 30 days before expiry (checked hourly). Failures retry with
exponential backoff (1 h, 2 h, 4 h… up to 24 h), respecting Let's Encrypt rate
limits. Use the **staging** directory to test before issuing in production.

Security notes:

- **Private keys** are generated per issuance and sent only to APISIX; the dashboard
  never stores them or writes them to the audit trail. In etcd they are encrypted by
  `apisix.data_encryption.keyring`: **change APISIX's default keyring**, it is public.
- The **ACME account key** is encrypted with the dashboard encryption key.
- Give the **Cloudflare token** only **Zone · DNS · Edit** on the zones you need, and
  deliver it as a secret (`DASH_ACME_CLOUDFLARE_API_TOKEN_FILE`).
- Automatically managed SSLs are **locked** against manual edit and delete in the
  generic screens, so the next renewal cannot silently undo a manual change.
- Registration, every issuance (manual or automatic renewal, with fingerprints before
  and after) and failures are audited.

Required egress (for PCI firewall rules): HTTPS to `acme-v02.api.letsencrypt.org` and
`api.cloudflare.com`; DNS (port 53) to the zone's name servers for the propagation
check (if blocked, a warning is logged and validation proceeds).

To **take over an existing certificate**, register the domains with the same SSL id
and tick "replace": the current certificate keeps being served until the new one is
issued.

## Kong → APISIX migration

The **Migration → Kong → APISIX** screen (`operator` and `admin`) moves the
configuration of a Kong 3.x (DB mode) deployment to APISIX in steps. Lintel only
**reads** from Kong.

1. **Settings** define how everything is translated: id strategy (Kong name or id)
   and prefix, routes created disabled (to enable at cutover), conflict policy for
   objects that already exist in APISIX, policy for untranslatable plugins, host
   rewrites (e.g. Kong DNS → Swarm service name), extra labels and **plugin rules**.
2. **Inventory**: services with upstreams, routes and plugins; consumers with
   credentials; global plugins. Each item shows whether it is to migrate, migrated,
   changed in Kong since migration, or blocked, plus translation warnings.
3. **Preview**: the exact JSON of each object, side by side with the current APISIX
   object when it exists. Secrets are masked.
4. **Apply** writes upstreams → services → consumers → credentials → routes → global
   rules. If any object fails, everything the batch wrote is rolled back. Apply is
   accepted only if Kong and APISIX are still as they were at preview time.

Re-running is safe: unchanged objects show as "migrated" and are not rewritten.

| Kong | APISIX |
|---|---|
| Service + Upstream/Targets | `upstream` (nodes, balancing, health checks, timeouts, retries, `pass_host`) + `service` |
| Route | `route`: prefix paths become `uris` with `*`, regex paths (`~`) and headers become `vars`, `strip_path` + service path become `proxy-rewrite.regex_uri` |
| Consumer + key-auth, jwt, hmac-auth | `consumer` + credentials; ACL groups become the `groups` label |
| Global plugins | `global_rules` |

Translated plugins: `key-auth`, `basic-auth`, `jwt`, `hmac-auth`, `acl`,
`rate-limiting` (`limit-count`, one rule per window), `cors`, `ip-restriction`,
`request-transformer` and `response-transformer` (headers only), `correlation-id`,
`prometheus`, `file-log`, `http-log`, `tcp-log`, `udp-log`, `proxy-cache`,
`request-size-limiting`, `bot-detection`, `request-termination` and
`openid-connect`. Behavioural differences become inventory warnings.

**Custom plugins:** add a rule in Settings that maps the Kong plugin to an APISIX one
with a JSON template, e.g.
`{"level": "${config.level}", "limit": "${config.limit|10}", "route": "${apisix.route_id}"}`.
A rule can also simply drop the plugin.

Known limitations:

- **basic-auth** passwords cannot be migrated: Kong stores only the hash.
- The **hmac-auth** signature header differs between the two gateways.
- TCP/UDP (stream) services, expression routes and consumer-group plugins are not
  supported.
- If a credential is recreated in Kong, the old one stays in APISIX; the preview warns
  when keys collide.

Every write is audited as `kong.migrate` with before and after; rollbacks as
`kong.rollback`.

**Reverting a migration** (History tab): select objects, or "Select the last import",
and choose *Revert selected*. The preview shows, per object, what will happen, derived
from the audit trail:

- **delete**: the migration created it;
- **restore original**: it replaced an existing object, which is put back;
- **manual**: the original had secrets (kept masked in the audit trail), or there is
  no migration write in the audit trail;
- **already gone**: only the history record is removed.

Objects changed in APISIX after their last migration write are skipped unless you
include them explicitly. Objects are reverted dependents first (routes, credentials,
consumers, services, upstreams). You confirm by typing the number of objects that will
change; the server checks it and rejects the request if anything changed since the
preview. Each change is audited as `kong.revert`, the batch as `kong.revert_batch`.

Regex paths (`~...`) are matched in APISIX with their literal prefix in `uris`
(e.g. `/ecom/v1/tokens/*`) and the full regex in `vars`, so plain prefix routes do not
take precedence over them.

## Development

```bash
cp .env.example .env     # then edit; never commit .env
make dev                 # backend on :8080 + Vite on :5173 (proxies /api)
make check               # everything CI runs: fmt-check, lint, test, vuln, secrets
make help                # all targets
```

Releases are automated with release-please and GoReleaser: see
[docs/releasing.md](docs/releasing.md).

Repository layout:

```
cmd/apisix-dashboard/   main (serve, healthcheck, version) + user CLI
internal/config         DASH_* configuration
internal/server         HTTP API, auth middleware, security headers, SPA handler
internal/auth           passwords, TOTP, sessions, lockout, roles
internal/apisix         Admin API client, resource allowlist, masking, schema catalog
internal/acme           ACME DNS-01 issuer and renewer (Cloudflare)
internal/kong           Kong reader, translator, planner, applier with rollback
internal/audit          audit trail
internal/store          SQLite
internal/secure         encryption, keys, random tokens
internal/i18n           message codes for user-facing API errors
internal/web            embedded web UI (generated from web/)
web/                    React + TypeScript + Mantine
deploy/                 Docker Swarm stack example
```

Contributions are welcome: read [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Roadmap

- OIDC (Keycloak) login, keeping local login as break-glass
- Plugin metadata UI
- Swarm integration: discover services and suggest upstreams
- Configuration history and rollback from the audit trail

## License

Licensed under the [Apache License, Version 2.0](LICENSE). See [NOTICE](NOTICE).
