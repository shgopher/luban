import os, sys, glob, json
sys.path.insert(0, "/Users/shgopher/Desktop/family/luban/.pylibs")
import pypdf

SRC = "/Users/shgopher/Desktop/top课程/04-左耳听风（完结）"
OUT = "/Users/shgopher/Desktop/family/luban/.tmp-zuoer/txt"
os.makedirs(OUT, exist_ok=True)

def norm(s):
    s = s.replace("\u00a0", " ")
    lines = [ln.rstrip() for ln in s.splitlines()]
    out, blank = [], 0
    for ln in lines:
        if not ln.strip():
            blank += 1
            if blank > 1:
                continue
            out.append("")
        else:
            blank = 0
            out.append(ln)
    return "\n".join(out)

files = sorted(f for f in os.listdir(SRC) if f.lower().endswith(".pdf"))
manifest = []
for name in files:
    p = os.path.join(SRC, name)
    try:
        r = pypdf.PdfReader(p)
        pages = []
        for page in r.pages:
            try:
                pages.append(page.extract_text() or "")
            except Exception as e:
                pages.append("")
        t = norm("\n".join(pages))
    except Exception as e:
        print("ERR", name, e)
        continue
    stem = os.path.splitext(name)[0]
    outp = os.path.join(OUT, stem + ".txt")
    with open(outp, "w") as fh:
        fh.write(t)
    manifest.append({"file": name, "stem": stem, "pages": len(r.pages), "chars": len(t)})
    print(f"{len(t):8d}  {len(r.pages):3d}p  {name}")

with open("/Users/shgopher/Desktop/family/luban/.tmp-zuoer/manifest.json", "w") as fh:
    json.dump(manifest, fh, ensure_ascii=False, indent=1)
print("TOTAL", sum(m["chars"] for m in manifest), "chars,", len(manifest), "files")
