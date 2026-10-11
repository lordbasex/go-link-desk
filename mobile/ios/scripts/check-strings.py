#!/usr/bin/env python3
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
# Every text the app shows has its Spanish and Portuguese translation: the
# keys of Text("…"), Button("…"), String(localized: "…") and the like in
# DeskViewer must be in es.lproj and pt.lproj (English is the key itself).
import glob
import re
import sys

here = __file__.rsplit("/", 2)[0]
keys = {}
for f in glob.glob(f"{here}/DeskViewer/**/*.swift", recursive=True):
    s = open(f, encoding="utf-8").read()
    for pat in (r'String\(localized:\s*"((?:[^"\\]|\\.)*)"',
                r'\b(?:Text|Button|Section|Label|Picker|TextField|LabeledContent|navigationTitle|ContentUnavailableView|SecureField)\(\s*"((?:[^"\\]|\\.)*)"'):
        for m in re.finditer(pat, s):
            if m.group(1):
                keys.setdefault(m.group(1), f)
    # The guide's rows: Gesture("symbol", "title", "text").
    for m in re.finditer(r'\bGesture\(\s*"[^"]*",\s*"((?:[^"\\]|\\.)*)",\s*"((?:[^"\\]|\\.)*)"', s):
        keys.setdefault(m.group(1), f)
        keys.setdefault(m.group(2), f)

bad = 0
for lang in ("es", "pt"):
    text = open(f"{here}/DeskViewer/Resources/{lang}.lproj/Localizable.strings", encoding="utf-8").read()
    have = set(re.findall(r'^"((?:[^"\\]|\\.)*)"\s*=', text, re.M))
    for k, f in sorted(keys.items()):
        if k not in have:
            print(f"{f.split('/')[-1]}: no {lang} for \"{k}\"")
            bad += 1
sys.exit(1 if bad else 0)
