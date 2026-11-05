"""Exact-replacement helper for the i18n conversion: every pair must match once."""


def conv(f, pairs, imports=()):
    s = open(f, encoding="utf-8").read()
    for a, b in pairs:
        n = s.count(a)
        assert n == 1, (f, a[:90], n)
        s = s.replace(a, b)
    if imports:
        lines = s.split("\n")
        idx = [i for i, l in enumerate(lines) if l.startswith("import ") or l.startswith("} from '")]
        if idx:
            pos = max(idx) + 1
        else:
            pos = 0
            while pos < len(lines) and lines[pos].startswith("//"):
                pos += 1
        lines[pos:pos] = [i for i in imports if i not in s]
        s = "\n".join(lines)
    open(f, "w", encoding="utf-8").write(s)
