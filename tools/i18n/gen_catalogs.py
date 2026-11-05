"""Generate web/src/i18n/locales/{en,pt-BR,es}.json from the catalog sources."""
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from catalog_server import SERVER  # noqa: E402
import catalog_ui  # noqa: E402

root = sys.argv[1]
go_codes = json.load(open(os.path.join(HERE, "server_codes.json"), encoding="utf-8"))
# Audit action names picked up by the regex are not message codes.
for fake in ("auth.login", "auth.mfa", "auth.mfa_enroll", "auth.password_change"):
    go_codes.pop(fake, None)

missing = sorted(set(go_codes) - set(SERVER))
extra = sorted(set(SERVER) - set(go_codes))
if missing or extra:
    sys.exit(f"server catalog mismatch: missing={missing} extra={extra}")

langs = {"en": {}, "pt-BR": {}, "es": {}}
for code, (pt, es) in SERVER.items():
    for lang, text in (("en", go_codes[code]), ("pt-BR", pt), ("es", es)):
        langs[lang]["server." + code] = text
for key, (en, pt, es) in catalog_ui.UI.items():
    for lang, text in (("en", en), ("pt-BR", pt), ("es", es)):
        langs[lang][key] = text

ph = re.compile(r"\{(\w+)\}")
for key in langs["en"]:
    want = set(ph.findall(langs["en"][key]))
    for lang in ("pt-BR", "es"):
        got = set(ph.findall(langs[lang][key]))
        if got != want:
            sys.exit(f"placeholder mismatch in {lang} {key}: {got} != {want}")


def nest(flat):
    out = {}
    for k in sorted(flat):
        parts = k.split(".")
        node = out
        for p in parts[:-1]:
            node = node.setdefault(p, {})
            if not isinstance(node, dict):
                sys.exit(f"key {k} conflicts with a leaf")
        if parts[-1] in node:
            sys.exit(f"key {k} conflicts with a branch")
        node[parts[-1]] = flat[k]
    return out


outdir = os.path.join(root, "web/src/i18n/locales")
for lang, flat in langs.items():
    with open(os.path.join(outdir, f"{lang}.json"), "w", encoding="utf-8") as f:
        json.dump(nest(flat), f, ensure_ascii=False, indent=2)
        f.write("\n")
print({lang: len(flat) for lang, flat in langs.items()})
