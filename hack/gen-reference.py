#!/usr/bin/env python3
"""Generates docs/reference.md from the CRDs (config/crd/bases): every field with its type, default and description."""
import glob, os, yaml
os.chdir(os.path.join(os.path.dirname(__file__), ".."))
ORDER = ["DXConnection", "DXScriptApplication", "DXPage", "DXTheme", "DXDamCollection", "DXDatabase"]
crds = {}
for f in glob.glob("config/crd/bases/*.yaml"):
    c = yaml.safe_load(open(f))
    crds[c["spec"]["names"]["kind"]] = c

def typ(s):
    t = s.get("type", "object")
    if t == "array":
        return "[]" + typ(s.get("items", {}))
    if "enum" in s:
        return " \\| ".join(map(str, s["enum"]))
    if s.get("x-kubernetes-int-or-string"):
        return "int or string"
    return t

def rows(schema, prefix, out, required=()):
    for name, s in (schema.get("properties") or {}).items():
        path = f"{prefix}.{name}" if prefix else name
        desc = " ".join((s.get("description") or "").split()).replace("|", "\\|")
        default = s.get("default")
        req = " (required)" if name in required else ""
        out.append(f"| `{path}` | {typ(s)}{req} | {'' if default is None else '`' + str(default) + '`'} | {desc} |")
        if s.get("type") == "object" and "properties" in s:
            rows(s, path, out, s.get("required", ()))
        if s.get("type") == "array" and "properties" in s.get("items", {}):
            rows(s["items"], path + "[]", out, s["items"].get("required", ()))

md = ["# API reference (dx.europesip.com/v1alpha1)", "", "[README](../README.md) · generated from the CRDs by `hack/gen-reference.py`", ""]
for k in ORDER:
    c = crds[k]
    v = c["spec"]["versions"][0]
    sch = v["schema"]["openAPIV3Schema"]
    names = c["spec"]["names"]
    md += [f"## {k}", "", " ".join(sch.get("description", "").split()), "",
           f"Short names: {', '.join('`' + s + '`' for s in names.get('shortNames', []))}", "", "### spec", "",
           "| Field | Type | Default | Description |", "|---|---|---|---|"]
    out = []
    rows(sch["properties"]["spec"], "", out, sch["properties"]["spec"].get("required", ()))
    md += out + ["", "### status", "", "| Field | Type | Default | Description |", "|---|---|---|---|"]
    out = []
    rows(sch["properties"]["status"], "", out)
    md += [r for r in out if not r.startswith("| `conditions.")] + [""]
open("docs/reference.md", "w").write("\n".join(md) + "\n")
print("docs/reference.md:", sum(1 for l in md if l.startswith("| `")), "fields")
