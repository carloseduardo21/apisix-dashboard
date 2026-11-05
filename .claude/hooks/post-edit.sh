#!/usr/bin/env bash
# PostToolUse hook (Edit|Write): fast, per-file format + lint.
#
# - Go: gofmt -w, go vet on the package, golangci-lint --fast-only (if configured)
# - web/: prettier --write, eslint on TS/TSX (if installed)
#
# Exit 0 = ok. Exit 2 = problems found; stderr is fed back to Claude so it
# fixes them. Missing tools are skipped silently so the hook never blocks
# a fresh checkout.
set -uo pipefail

root="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null)}"
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$HOME/.local/bin:$PATH"

file=$(python3 -I -c 'import json,sys
try: print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))
except Exception: pass')
[ -n "$file" ] && [ -f "$file" ] || exit 0
case "$file" in "$root"/*) ;; *) exit 0 ;; esac
rel=${file#"$root"/}

fail() { printf '%s\n' "$@" >&2; exit 2; }

case "$rel" in
  *.go)
    command -v gofmt >/dev/null || exit 0
    gofmt -w "$file"
    pkg="./$(dirname "$rel")"
    out=$(cd "$root" && go vet "$pkg" 2>&1) || fail "go vet $pkg failed:" "$out"
    if command -v golangci-lint >/dev/null && [ -f "$root/.golangci.yml" ]; then
      out=$(cd "$root" && golangci-lint run --fast-only "$pkg" 2>&1) || fail "golangci-lint $pkg:" "$out"
    fi
    ;;
  web/package-lock.json) ;;
  web/*.ts | web/*.tsx | web/*.js | web/*.cjs | web/*.css | web/*.json | web/*.html | web/*.md)
    bin="$root/web/node_modules/.bin"
    if [ -x "$bin/prettier" ]; then
      (cd "$root/web" && "$bin/prettier" --write --log-level warn "$file") || fail "prettier failed on $rel"
    fi
    case "$rel" in
      *.ts | *.tsx)
        if [ -x "$bin/eslint" ] && ls "$root"/web/eslint.config.* >/dev/null 2>&1; then
          out=$(cd "$root/web" && "$bin/eslint" --max-warnings 0 "$file" 2>&1) || fail "eslint $rel:" "$out"
        fi
        ;;
    esac
    ;;
esac
exit 0
