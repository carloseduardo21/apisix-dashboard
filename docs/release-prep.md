# Open-source release preparation — status

Tracking document for preparing **Lintel for Apache APISIX** for publication on
GitHub (`github.com/carloseduardo21/apisix-dashboard`, public since 2026-11-05).
Each phase is approved by the maintainer before the next one starts.

## Phases

| # | Phase | Status |
|---|---|---|
| 0 | Discovery (architecture, features, risks) | ✅ done |
| 1 | Secrets and repository hygiene | ✅ done |
| 2 | Claude Code context and rules (`.claude/`, `CLAUDE.md` files) | ✅ done |
| 3 | Local tooling (Makefile, golangci-lint, Oxlint, Prettier, scanners) + all lint findings fixed | ✅ done |
| 4 | CI/CD (CI, CodeQL, Trivy, Dependabot, Scorecard) + UI i18n (en, pt-BR, es) | ✅ done |
| 5 | Governance and docs (README, SECURITY, CONTRIBUTING, CODE_OF_CONDUCT, CODEOWNERS, templates, licence) | ✅ done |
| 6 | Releases and supply chain (release-please, GoReleaser, GHCR multi-arch, SBOM, cosign) | ✅ done (see `docs/releasing.md`) |
| 7 | Application security review → `docs/security-review.md` | ⏳ next |

## Decisions taken

- Product name **Lintel for Apache APISIX** (ASF trademark policy); repo, module and
  binary keep the technical name `apisix-dashboard`.
- Go 1.27; module `github.com/carloseduardo21/apisix-dashboard`.
- All docs, comments, CLI output, logs and audit details in **English**.
- UI i18n with i18next: `en` (source/fallback), `pt-BR`, `es`. The API returns
  stable error **codes + params**; catalogs in `web/src/i18n/locales` are the source
  of truth, checked by `go test ./internal/i18n`.
- Frontend lint is **Oxlint** + Prettier (typescript-eslint does not support TS 7).
- Conventional Commits; Claude commits at the end of each approved phase.
- Git history was empty when the work started: no history rewrite was needed.
- Licence **Apache-2.0**; contributions under the **DCO** (no CLA); public contact
  only through GitHub (private vulnerability reporting, no e-mail in the repo).
- TOTP issuer is **Lintel** for new MFA enrollments (existing ones keep the old name).
- Repository settings: Dependabot alerts and private vulnerability reporting enabled;
  `main` protected (required CI checks, linear history, no force-push/deletion,
  admins may bypass for direct maintainer pushes; `DCO sign-off` required too).
- Squash merge only (PR title as commit title), branches deleted after merge;
  repository description and topics set.

## Pending questions for the maintainer

1. Phase 7 will detail the risks found in Phase 0, e.g. operators can enable
   dangerous plugins (`serverless-*`), SSRF through upstream nodes / secret URIs,
   per-account-only login lockout (no IP rate limit), spoofable `X-Forwarded-For`
   in the audit trail. No fix is applied without approval.

## Follow-ups

- Enable *Allow GitHub Actions to create and approve pull requests* (repository
  settings → Actions → General) so release-please can open the release PR.
- After v0.1.0: remove `release-as` from `release-please-config.json` and make the
  GHCR package public.
- Dependabot PRs opened before the DCO workflow existed need a rebase
  (`@dependabot rebase`) to get the now-required `DCO sign-off` check.
