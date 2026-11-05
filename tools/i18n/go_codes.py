"""Extract (code, English template) pairs from non-test Go sources."""
import json, os, re, sys
pat = re.compile(r'"([a-z][a-z_]*(?:\.[a-z0-9_]+)+)",\s*"((?:[^"\\]|\\.)*)"')
codes = {}
for dp, _, fs in os.walk(os.path.join(sys.argv[1], "internal")):
    for f in fs:
        if f.endswith(".go") and not f.endswith("_test.go"):
            lines = [l for l in open(os.path.join(dp, f), encoding="utf-8") if not l.lstrip().startswith("//")]
            for m in pat.finditer("".join(lines)):
                c, t = m.group(1), m.group(2).replace('\\"', '"')
                if c in codes and codes[c] != t:
                    raise SystemExit(f"conflict {c}: {codes[c]!r} vs {t!r}")
                codes[c] = t
json.dump(codes, open(sys.argv[2], "w"), indent=1, ensure_ascii=False)
print(len(codes))
