# Contributing to Lintel

Thanks for your interest! Lintel is the control plane of an API gateway used in
PCI DSS environments, so every change is reviewed as a security-sensitive change.
Please read this guide before opening a pull request.

- Be respectful: this project follows the [Code of Conduct](CODE_OF_CONDUCT.md).
- **Security vulnerabilities are never reported in public**: see [SECURITY.md](SECURITY.md).

## Before you start

- **Bugs:** open an issue with the bug template (redact keys, passwords, hostnames).
- **Features and behaviour changes:** open an issue first and wait for the
  maintainer's agreement. Changes to functional behaviour, security defaults or the
  API contract are not merged without prior discussion.
- **Small fixes** (typos, docs, obvious bugs) can go straight to a pull request.

## Development setup

Requirements: Go 1.27+, Node 24+, Docker, and the tools checked by `make tools`
(golangci-lint, govulncheck, gitleaks).

```bash
cp .env.example .env     # then edit; never commit .env
make dev                 # backend on :8080 + Vite on :5173 (proxies /api)
make check               # everything CI runs
make help                # all targets
```

You need an APISIX instance with the Admin API enabled; a local Docker container is
enough for development.

Stack-specific rules live in [`internal/CLAUDE.md`](internal/CLAUDE.md) (Go) and
[`web/CLAUDE.md`](web/CLAUDE.md) (React/TypeScript). They are written for AI
assistants but are the project's coding rules for humans too.

## Rules every change must follow

1. **No hardcoded secrets**, internal URLs, hostnames or IPs. Configuration comes from
   `DASH_*` variables (every secret supports `_FILE`). Examples use `example.com` and
   `change-me`.
2. **The APISIX admin key stays in the backend.** No response, log line, error,
   audit record or frontend bundle may contain it.
3. **Every backend route is authenticated** with the least privilege it needs
   (`s.require(access{...})`). Only `/api/health` and `/api/auth/login` are public.
4. **Validate all input**: size-limited JSON decoding, allowlists, strict parsing of
   ids and path segments. New Admin API resources must be added to the allowlist and
   reviewed for secret masking.
5. **Errors never leak internals** (stack traces, upstream bodies, file paths).
6. **Every state change is audited** with masked before/after.
7. **No new dependency without justification** in the pull request: why it is
   needed, why the standard library or existing dependencies are not enough, and its
   licence, maintenance and security status. Pin exact versions.
8. **Internationalization:** every user-facing UI string goes through `t('key')` and
   must exist in `en`, `pt-BR` and `es` (`web/src/i18n/locales`). The backend returns
   message codes (`internal/i18n`), never translated text. If you do not speak one of
   the languages, add the English text and say so in the pull request.
9. Docs, code comments, commit messages, logs and CLI output are in **English**.

## Tests

- Go: table-driven tests with `t.Run`; handler tests with `httptest`, covering each
  role, CSRF, validation errors, and that secrets never appear in responses or audit
  output. No network access in unit tests.
- Frontend: type-checked (`tsc --noEmit`), linted with Oxlint, formatted with Prettier.
- `make check` must pass with **zero** findings. Lint suppressions need a reason on
  the same line.

## Commits and pull requests

- Use [Conventional Commits](https://www.conventionalcommits.org/):
  `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `build:`, `ci:`, `chore:`, `perf:`,
  `style:`, `revert:`; optional scope (`fix(acme): …`); `!` or `BREAKING CHANGE:` for
  breaking changes. Imperative mood: "add", not "added". The changelog and release
  versions are generated from these messages.
- Keep pull requests focused: one logical change each.
- Fill in the pull request template, including the security checklist.
- CI (lint, tests, vulnerability and secret scans, image build with Trivy, CodeQL)
  must be green.

## Developer Certificate of Origin

Lintel uses the [Developer Certificate of Origin 1.1](https://developercertificate.org/)
(DCO) instead of a contributor licence agreement. By signing off a commit you certify
that you wrote the change, or otherwise have the right to submit it under the
project's licence ([Apache-2.0](LICENSE)).

Sign off every commit with `-s`:

```bash
git commit -s -m "fix(auth): reject empty TOTP codes"
```

This adds a trailer that must match the commit author:

```
Signed-off-by: Jane Doe <jane@example.com>
```

Forgot? Fix the last commit with `git commit --amend -s --no-edit`, or every commit
of your branch with `git rebase --signoff main`, then force-push your branch. A CI
check verifies the sign-off on every pull request.

## Licence

By contributing, you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE).
