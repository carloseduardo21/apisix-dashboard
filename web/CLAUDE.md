# Frontend rules (`web/`)

Read `.claude/CLAUDE.md` first; these rules add to it.

## Stack

- Vite 8, React 19, **TypeScript strict** (`tsconfig.json`: `strict`,
  `noUnusedLocals`, `noUnusedParameters`), Mantine 9 (+ modals, notifications,
  spotlight), TanStack Query 5, React Router 8, `@tabler/icons-react`, `qrcode`.
- `npm run build` type-checks and writes the bundle to `../internal/web/dist`, which
  the Go binary embeds. `npm run dev` serves on :5173 and proxies `/api` to :8080.
- Dependencies are pinned to exact versions; `package-lock.json` is committed and
  installs use `npm ci`.

## Layout

```
src/api.ts            the only HTTP client (fetch, CSRF header, ApiError, types)
src/auth.tsx          AuthGate / useAuth: session state, login flow gate
src/App.tsx           routes; role gates (AdminOnly, WritersOnly)
src/resources.tsx     APISIX resource registry (columns, forms, labels)
src/pages/            screens (Overview, ResourcePage, Certificates, Users, Audit, Kong…)
src/forms/            hand-written forms per resource
src/schema/           JSON-Schema form generator + plugin editor
src/components/       layout and shared components
src/theme/            Mantine theme + global CSS
```

## Rules

- **All backend calls go through `api()` in `src/api.ts`** (same origin, `/api/*`,
  `credentials: 'same-origin'`, `X-CSRF-Token` on non-GET). Never call the APISIX Admin
  API, Kong or any third-party origin from the browser; the CSP (`connect-src 'self'`)
  blocks it anyway.
- **Never store credentials or tokens** in `localStorage`/`sessionStorage`/cookies from
  JS. The session is an HttpOnly cookie; the CSRF token lives in memory only.
  `localStorage` is acceptable only for non-sensitive UI preferences.
- **No `dangerouslySetInnerHTML`, `innerHTML`, `eval`, `new Function`** or inline
  scripts/styles that would need CSP relaxation. Render APISIX/Kong/audit data as text
  (React escaping) — it is attacker-controllable config.
- External links: `target="_blank"` always with `rel="noreferrer"`; never build `href`
  from untrusted data unless the scheme is validated (`https:` only).
- Role checks in the UI (`canWrite`, `isAdmin`) are **cosmetic**; the backend enforces
  authorization. Never rely on hiding a button for security.
- Secrets arrive masked as `MASKED` (`******`). Keep the masked value when the user does
  not change a secret field — the backend restores the original. Never try to
  "unmask" or cache secrets client-side.
- No `any` (use `unknown` + narrowing); no non-null assertions on API data without a
  check. Functional components and hooks only.
- Server state via TanStack Query (query keys per resource, invalidate after
  mutations); show errors with `ApiError.message` (already safe, from the backend).
- **i18n**: no hard-coded user-facing text. Use `useTranslation()`/`t('key')` in
  components (`i18n.t` only in non-component helpers rendered by a translated
  parent), `<Trans>` for strings with markup, `formatDate*`/`serverText`/`noteText`
  from `src/i18n/format.ts` for dates and backend messages. Add every key to
  `en.json`, `pt-BR.json` and `es.json` (placeholders `{name}`; avoid plural
  suffixes, use neutral wording with counts).
- Accessibility: labels on inputs, `aria-label` on icon-only buttons.
- No new npm dependency without justification (size, maintenance, licence, audit
  status). Prefer Mantine/stdlib-of-the-browser solutions.

## Commands

```bash
npm ci
npm run typecheck
npm run build
npm audit --audit-level=high
npm run lint          # oxlint --type-aware --max-warnings 0
npm run format        # prettier --write
```

Lint is **Oxlint** (`.oxlintrc.json`), not ESLint: typescript-eslint does not support
TypeScript 7. Disable a rule only inline with a reason:
`// oxlint-disable-next-line <rule> -- <why>`.
