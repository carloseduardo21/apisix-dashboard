---
description: Run the Definition of Done for the current change and report what is missing
---

Verify the current change against the **Definition of Done** in `.claude/CLAUDE.md`.

1. Run, in order, and capture failures (skip unavailable targets and say so):
   `make fmt` (then `git diff --stat` must show no formatting-only changes),
   `make lint`, `make test`, `make vuln`, `make build`.
2. Check tests: for each changed Go handler/package and frontend module, is there a
   test covering the new/changed behavior (including authz per role for new routes)?
3. Check docs: new `DASH_*` variables in `.env.example` and README; new features or
   decisions reflected in `.claude/CLAUDE.md`; user-visible changes described in a
   Conventional Commit message.
4. Run the `/security-check` checklist mentally on the diff and list any concern.
5. Report a ✅/❌ checklist. Fix only trivial, behavior-neutral issues (formatting,
   lint autofixes, missing doc entries); list everything else for the maintainer.
