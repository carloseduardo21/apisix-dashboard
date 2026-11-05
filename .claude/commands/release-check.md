---
description: Verify the repository is ready to cut a release (quality gates, security, docs, versioning)
argument-hint: "[version, e.g. v0.2.0]"
---

Check whether `main` is ready to be released as $ARGUMENTS. Do not tag, push or publish
anything — only report.

1. Repository state: on `main`, clean working tree (`git status`), up to date with the
   remote, no unpushed commits. List commits since the last tag
   (`git describe --tags --abbrev=0` → `git log <tag>..HEAD --oneline`) and verify each
   follows Conventional Commits. Infer the expected SemVer bump (feat → minor,
   fix/perf → patch, `!`/BREAKING CHANGE → major; pre-1.0 rules if version < 1.0.0) and
   compare with the requested version.
2. Quality gates (stop at the first hard failure and report it):
   `make fmt` (no diff) → `make lint` → `make test` → `make vuln` → `make build` →
   `make docker-build`.
3. Security: `gitleaks git --log-opts="--all" --redact --no-banner .`; if Trivy is
   installed, `trivy image --severity HIGH,CRITICAL --ignore-unfixed <built image>`
   and `trivy config Dockerfile`.
4. Docs: README configuration table and `.env.example` contain every `DASH_*` variable
   used in code (`grep -rhoE 'DASH_[A-Z0-9_]+' cmd internal | sort -u`); APISIX
   compatibility notes are current; `.claude/CLAUDE.md` feature list matches new
   features; CHANGELOG / release-please PR is consistent.
5. Supply chain (if CI is set up): workflows pin actions by SHA, release workflow
   produces SBOM and cosign signature, Dependabot/Renovate has no pending critical
   security updates.

Output a checklist with ✅/❌ per item, blocking issues first, then the suggested
version and release notes summary.
