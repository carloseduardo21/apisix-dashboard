## What and why

<!-- What does this change do, and why is it needed? Link the issue: "Closes #123". -->

## How it was tested

<!-- Tests added/updated, manual checks (role used, APISIX version). -->

## Checklist

- [ ] Title follows [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`…)
- [ ] Every commit is signed off (`git commit -s`, see [CONTRIBUTING.md](../CONTRIBUTING.md#developer-certificate-of-origin))
- [ ] `make check` passes locally (format, lint, tests, vulnerabilities, secrets)
- [ ] New behaviour has tests (table-driven in Go, `httptest` for routes)
- [ ] Security reviewed: auth/role on new routes, input validation, secret masking, audit record, no internal errors leaked
- [ ] User-facing strings translated in `en`, `pt-BR` and `es`
- [ ] Docs updated (README, `.env.example` for new `DASH_*` variables)
- [ ] No new dependency, or it is justified below

<!-- Dependency justification, screenshots for UI changes, breaking changes. -->
