import os, re

SRC = "/Users/shgopher/Desktop/family/luban/.tmp-arch-course/txt"
OUT = "/Users/shgopher/Desktop/family/luban/.tmp-arch-course/clean"
os.makedirs(OUT, exist_ok=True)

NOISE = re.compile(r"(下载APP|讲述：|时长 |加微信|首发微信|拼课微信|防止断更|^进入课程$|^展开|^© 版权|^上一篇 |^下一篇 |^精选留言|^写留言)")

for name in sorted(os.listdir(SRC)):
    if not name.endswith(".txt"): continue
    raw = open(os.path.join(SRC, name)).read()
    lines = [l.rstrip() for l in raw.split("\n")]
    # body: up to copyright
    body = []
    tail = []
    in_tail = False
    for l in lines:
        if "© 版权" in l:
            in_tail = True
            continue
        if not in_tail:
            if NOISE.search(l): continue
            body.append(l)
        else:
            tail.append(l)
    # merge body: join hard-wrapped lines. Paragraph ends when line ends with punctuation and next line starts new idea; heuristics: join all, then split on 。！？ patterns is risky. Keep line breaks but collapse.
    txt = "\n".join(body)
    txt = re.sub(r"\n{3,}", "\n\n", txt).strip()
    # author replies
    replies = []
    for i, l in enumerate(tail):
        if "作者回复" in l:
            prev = ""
            j = i - 1
            while j >= 0 and not tail[j].strip():
                j -= 1
            if j >= 0: prev = tail[j].strip()
            replies.append((prev, l.strip()))
    out = txt
    if replies:
        out += "\n\n---- 作者回复摘录 ----\n"
        for q, a in replies:
            out += f"Q: {q}\n{q and ''}A: {a}\n"
    open(os.path.join(OUT, name), "w").write(out)
    print(f"{len(out):7d} {name}")
