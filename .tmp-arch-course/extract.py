import os, sys, glob
import Quartz
from Foundation import NSURL

SRC = "/Users/shgopher/Desktop/top课程/03-从0开始学架构（完结）"
OUT = "/Users/shgopher/Desktop/family/luban/.tmp-arch-course/txt"

def extract(path):
    url = NSURL.fileURLWithPath_(path)
    doc = Quartz.PDFDocument.alloc().initWithURL_(url)
    if doc is None:
        return None
    return doc.string()

files = []
for root, dirs, fs in os.walk(SRC):
    for f in fs:
        if f.lower().endswith(".pdf"):
            files.append(os.path.join(root, f))
files.sort()
for p in files:
    rel = os.path.relpath(p, SRC)
    out = os.path.join(OUT, rel.replace("/", "__")[:-4] + ".txt")
    try:
        t = extract(p)
    except Exception as e:
        t = None
        print("ERR", rel, e)
    if t is None:
        print("FAIL", rel); continue
    with open(out, "w") as fh:
        fh.write(t)
    print(f"{len(t):8d}  {rel}")
