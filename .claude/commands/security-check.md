---
description: Security review of the current changes (or a path) against the project's mandatory rules
argument-hint: "[path or git range, default: uncommitted + branch changes vs main]"
---

Perform a focused security review for this dashboard, which controls an entire
Apache APISIX gateway. Scope: $ARGUMENTS (if empty, use `git diff main...HEAD` plus
uncommitted changes).

1. Run the automated checks and capture the results (skip a tool that is not installed
   and say so):
   - `gitleaks git --log-opts="main..HEAD" --redact --no-banner .` and
     `gitleaks git --pre-commit --staged --redact --no-banner .`
   - `govulncheck ./...`
   - `golangci-lint run ./...` (gosec findings matter most)
   - `cd web && npm audit --audit-level=high`
2. Review every changed file manually against this checklist and cite `file:line`:
   - **Auth/authz**: every new route uses `s.require(access{...})` with least privilege;
     writes need `write`, user/audit/ACME management needs `admin`; no auth bypass for
     non-GET; CSRF untouched.
   - **Admin key / secrets**: admin key and other secrets never reach responses, logs,
     audit, errors or the frontend; new secret fields are masked (`sensitiveKeys` or
     plugin `encrypt_fields`) and restored by `Unmask`; config secrets support `_FILE`.
   - **Input validation**: size-limited decoding, strict path/id/enum validation,
     Admin API paths only via `apisix.ParsePath`; no raw user input in URLs or SQL.
   - **Config injection / SSRF**: upstream nodes, `secrets` URIs, plugin configs (esp.
     `serverless-*`, loggers, `forward-auth`, `ext-plugin-*`) — is the change widening
     what an operator can make the gateway do?
   - **Error leakage**: no `err.Error()` of internal errors, stack traces or upstream
     bodies to the client.
   - **Audit**: every state change recorded with masked before/after.
   - **Frontend**: no `dangerouslySetInnerHTML`/`innerHTML`/`eval`, no tokens in web
     storage, external links with `rel="noreferrer"`, untrusted data rendered as text.
   - **Dependencies**: any new Go/npm dependency is justified and pinned.
3. Report findings grouped by severity (critical / high / medium / low / info), each with
   file:line, impact, and a suggested fix. **Do not modify code** — list fixes for the
   maintainer to approve.
