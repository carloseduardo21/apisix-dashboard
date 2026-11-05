# Open-source release preparation — status

Tracking document for preparing **Lintel for Apache APISIX** for publication on
GitHub (`github.com/carloseduardo21/apisix-dashboard`, private first, then public).
Each phase is approved by the maintainer before the next one starts.

## Phases

| # | Phase | Status |
|---|---|---|
| 0 | Discovery (architecture, features, risks) | ✅ done |
| 1 | Secrets and repository hygiene | ✅ done |
| 2 | Claude Code context and rules (`.claude/`, `CLAUDE.md` files) | ✅ done |
| 3 | Local tooling (Makefile, golangci-lint, Oxlint, Prettier, scanners) + all lint findings fixed | ✅ done |
| 4 | CI/CD (CI, CodeQL, Trivy, Dependabot, Scorecard) + UI i18n (en, pt-BR, es) | ✅ done |
| 5 | Governance and docs (README, SECURITY, CONTRIBUTING, CODE_OF_CONDUCT, CODEOWNERS, templates, licence options) | ⏳ next |
| 6 | Releases and supply chain (release-please, GoReleaser, GHCR multi-arch, SBOM, cosign) | pending |
| 7 | Application security review → `docs/security-review.md` | pending |

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

## Pending questions for the maintainer

1. **Licence** (Phase 5): options (Apache-2.0, MIT, AGPL-3.0, dual licensing…) will
   be presented with pros and cons, considering future monetization. Not chosen yet.
2. **TOTP issuer**: still "APISIX Dashboard" in authenticator apps. Switch new
   enrollments to "Lintel"?
3. Phase 7 will detail the risks found in Phase 0, e.g. operators can enable
   dangerous plugins (`serverless-*`), SSRF through upstream nodes / secret URIs,
   per-account-only login lockout (no IP rate limit), spoofable `X-Forwarded-For`
   in the audit trail. No fix is applied without approval.

## Notes for when the repository is created

- No git remote is configured yet; push `main` after creating the GitHub repo.
- CodeQL, SARIF uploads and Scorecard only run on a **public** repository (code
  scanning on private repos requires GitHub Advanced Security).
- Recommended settings: branch protection on `main` with the CI checks required,
  private vulnerability reporting enabled, Dependabot alerts enabled.
