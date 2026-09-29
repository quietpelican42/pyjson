#!/usr/bin/env python3
"""Generate a differential corpus for pyjson.

Usage: gen_corpus.py [--seed N] [--n COUNT] [--out DIR]

Writes DIR/dumps.jsonl and DIR/loads.jsonl. Each dumps line holds a value in
a tagged form (so Go can rebuild Python-shaped data exactly) and the bytes
Python's json.dumps produced for every option set, or null when Python raised.
"""
import argparse, base64, json, math, os, random, struct, sys

OPTSETS = [
    ("default", {}),
    ("sort_keys", {"sort_keys": True}),
    ("no_ensure_ascii", {"ensure_ascii": False}),
    ("indent2", {"indent": 2}),
    ("compact", {"separators": (",", ":")}),
    ("no_nan", {"allow_nan": False}),
    ("sorted_indent_utf8", {"sort_keys": True, "indent": 2, "ensure_ascii": False}),
    ("tab_indent_skipkeys", {"indent": "\t", "skipkeys": True}),
    ("indent0_custom_sep", {"indent": 0, "separators": (";", "=")}),
    ("sorted_compact_nonascii_skip", {"sort_keys": True, "separators": (",", ":"), "ensure_ascii": False, "skipkeys": True}),
]

def b64(s):
    return base64.b64encode(s.encode("utf-8", "surrogatepass")).decode()

class Tup(tuple):
    pass

def rand_float(r):
    k = r.random()
    if k < 0.35:
        while True:
            f = struct.unpack("<d", struct.pack("<Q", r.getrandbits(64)))[0]
            if math.isfinite(f):
                return f
    if k < 0.50:
        return r.uniform(-1e3, 1e3)
    if k < 0.60:
        return float(r.randint(-10**6, 10**6))
    if k < 0.70:
        e = r.randint(-30, 30)
        return float("%de%d" % (r.randint(1, 99999), e))
    if k < 0.80:
        return r.choice([1e16, 1e15, 9999999999999998.0, 1e-4, 1e-5, 0.0001, 0.00001, 123456789012345680.0,
                         1e22, 1e21, 5e-324, 2.2250738585072014e-308, 1.7976931348623157e308, 0.1, 0.5,
                         100.0, 1e2, 1.5e300, 4.35, 2.675, 1/3, 0.30000000000000004, 1e100, 12345678901234567.0])
    if k < 0.87:
        return r.choice([0.0, -0.0, math.inf, -math.inf, math.nan])
    if k < 0.94:
        # integral floats around the exponent switch
        return float(r.randint(1, 9)) * 10.0 ** r.randint(14, 18)
    return r.random() * 10.0 ** r.randint(-8, -1)

def rand_int(r):
    k = r.random()
    if k < 0.5:
        return r.randint(-1000, 1000)
    if k < 0.7:
        return r.choice([0, -1, 2**31, -2**31, 2**53, 2**53 + 1, 2**63 - 1, -2**63, 2**63, -2**63 - 1, 2**64, 2**64 - 1])
    if k < 0.9:
        return r.randint(-10**r.randint(19, 60), 10**r.randint(19, 60))
    return r.randint(-10**r.randint(60, 300), 10**r.randint(60, 300))

SPECIAL = ["", "a", "hello world", '"', "\\", "/", "\n\r\t\b\f", "\x00", "\x1f", "\x7f", "\x80", "  ", "<script>&amp;</script>",
           "é", "中文", "\U0001f600", "\U0001d11e", "￿", "￾", "퟿", "", "\U0010ffff",
           "é", "‮", "​", "﻿", "\'"]

def rand_str(r, allow_lone=True):
    k = r.random()
    if k < 0.15:
        return r.choice(SPECIAL)
    n = r.choice([0, 1, 2, 3, 5, 8, 13, 40])
    out = []
    for _ in range(n):
        c = r.random()
        if c < 0.35:
            out.append(chr(r.randint(0x20, 0x7e)))
        elif c < 0.50:
            out.append(chr(r.randint(0, 0x1f)))
        elif c < 0.65:
            out.append(chr(r.randint(0x80, 0x7ff)))
        elif c < 0.80:
            cp = r.randint(0x800, 0xffff)
            if 0xd800 <= cp <= 0xdfff:
                cp = 0xe000 + (cp - 0xd800) if not allow_lone or r.random() < 0.9 else cp
            out.append(chr(cp))
        elif c < 0.95:
            out.append(chr(r.randint(0x10000, 0x10ffff)))
        else:
            out.append(r.choice(["<", ">", "&", '"', "\\", " ", "\x7f"]))
    return "".join(out)

def rand_key(r, depth):
    k = r.random()
    if k < 0.86:
        return rand_str(r)
    if k < 0.91:
        return rand_int(r) if r.random() < 0.5 else r.randint(-5, 5)
    if k < 0.95:
        f = rand_float(r)
        return 0.0 if f != f else f  # NaN keys have no defined sort order; excluded
    if k < 0.98:
        return r.choice([True, False, None])
    return Tup((r.randint(0, 9),))

def rand_value(r, depth=0):
    k = r.random()
    if depth >= 4 or k < 0.55:
        k2 = r.random()
        if k2 < 0.05:
            return None
        if k2 < 0.10:
            return r.random() < 0.5
        if k2 < 0.28:
            return rand_int(r)
        if k2 < 0.60:
            return rand_float(r)
        return rand_str(r)
    if k < 0.80:
        return [rand_value(r, depth + 1) for _ in range(r.choice([0, 1, 2, 3, 5, 8]))]
    d = {}
    n = r.choice([0, 1, 1, 2, 3, 4, 6, 10])
    strict = r.random() < 0.5  # half of the dicts use string keys only
    tup_used = False
    for _ in range(n):
        key = rand_str(r) if strict else rand_key(r, depth)
        if isinstance(key, Tup):
            if tup_used:
                continue
            tup_used = True
        d[key] = rand_value(r, depth + 1)
    if r.random() < 0.15 and not any(isinstance(k, (Tup, int, float)) or k is None for k in d):
        d = dict(sorted(d.items()))
    return d

def tag(v, ids):
    if v is None or v is True or v is False:
        return v
    if isinstance(v, int):
        return {"i": str(v)}
    if isinstance(v, float):
        return {"f": "%016x" % struct.unpack("<Q", struct.pack("<d", v))[0]}
    if isinstance(v, Tup):
        return {"t": v[0]}
    if isinstance(v, str):
        if any(0xd800 <= ord(c) <= 0xdfff for c in v):
            return {"sc": [ord(c) for c in v]}
        return {"s": v}
    if isinstance(v, list):
        return [tag(x, ids) for x in v]
    if isinstance(v, dict):
        return {"d": [[tag(k, ids), tag(x, ids)] for k, x in v.items()]}
    raise TypeError(type(v))

def run(v, kw):
    try:
        s = json.dumps(v, **kw)
        return b64(s)
    except (ValueError, TypeError, RecursionError):
        return None

def mutate_text(r, s):
    alphabet = list('{}[]:,"\\ \t\n0123456789.eE+-truefalsnNItyé\ud83d')
    s = list(s)
    for _ in range(r.randint(1, 3)):
        op = r.random()
        if not s:
            break
        i = r.randrange(len(s))
        if op < 0.4:
            s[i] = r.choice(alphabet)
        elif op < 0.7:
            del s[i]
        elif op < 0.9:
            s.insert(i, r.choice(alphabet))
        else:
            s = s[:i]
    return "".join(s)

HAND_LOADS = [
    '{"a":1,"b":2,"a":3}', '{"a":1,"a":2,"a":3}', '{"b":1,"a":2,"b":3,"c":4,"a":5}',
    '1e400', '-1e400', '1e-400', '-0', '-0.0', '0', '00', '01', '1.', '.5', '-', '+1', '1e', '1e+', '1E5', '1e05',
    '123456789012345678901234567890', '-123456789012345678901234567890', '9223372036854775807', '9223372036854775808',
    '-9223372036854775808', '-9223372036854775809', '18446744073709551615', '1.0', '100', '1.5e3',
    '0.1e1', '0.000001', '1e16', '1e15', '12345678901234567890.5', '5e-324', '2e-324', '1.7976931348623157e308', '1.7976931348623159e308',
    'NaN', 'Infinity', '-Infinity', 'nan', 'inf', '-NaN', '+Infinity', 'true', 'false', 'null', 'True', 'None',
    '[NaN,Infinity,-Infinity]', '"\\ud83d\\ude00"', '"\\ud83d"', '"\\ude00"', '"\\ud83dx"', '"\\ude00\\ud83d"', '"\\ud83d\\u0041"',
    '"\\u00e9"', '"\\u00E9"', '"\\u12"', '"\\uzzzz"', '"\\x41"', '"\\/"', '"\\a"', '"a\tb"', '"a\nb"', '"a\x7fb"', '"\x00"',
    '" "', '"unterminated', '"', '', ' ', ' 1 ', '\t\n\r 1\t\n\r ', '\x0b1', '\x0c1', ' 1', '1 2', '[1,]', '[,1]', '[1 2]',
    '{"a":1,}', '{,}', '{"a"}', '{"a":}', '{1:2}', "{'a':1}", '[]', '{}', '[ ]', '{ }', '[[]]', '{"":{}}',
    '["a",1,2.0,null,true,false,{"k":[]}]', '﻿{}', '{"a":1}}', '[1]]', '[' * 100 + ']' * 100, '[' * 2000 + ']' * 2000,
    '"' + 'a' * 5000 + '"', '1' + '0' * 300, '1' + '0' * 5000, '-' + '1' * 4301, '1' * 4300, '1' * 4301, '0.' + '1' * 5000,
    '{"a":{"b":{"c":[1,2,{"d":null}]}}}', '{"\\u0061":1,"a":2}', '{"a":1,"\\u0061":2}', '{"x":1,"y":2,"x":{"z":1}}',
    '1e5', '1E+5', '1e-5', '-1e-5', '123.456e-7', '0e0', '0E+0', '-0e0', '1.0e0', '[1.0,1,1e0,10E-1]', '"\\ud800\\udc00"', '"\\uD83D\\uDE00"',
]

def loads_expect(text):
    try:
        v = json.loads(text)
        return b64(json.dumps(v))
    except (ValueError, TypeError, RecursionError):
        return None

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--n", type=int, default=20000)
    ap.add_argument("--out", default="testdata")
    a = ap.parse_args()
    os.makedirs(a.out, exist_ok=True)
    r = random.Random(a.seed)
    sys.setrecursionlimit(5000)
    with open(os.path.join(a.out, "dumps.jsonl"), "w") as f:
        f.write(json.dumps({"optsets": [[n, {k: (list(v) if isinstance(v, tuple) else v) for k, v in kw.items()}] for n, kw in OPTSETS], "python": sys.version.split()[0], "seed": a.seed}) + "\n")
        for _ in range(a.n):
            v = rand_value(r)
            exp = [run(v, kw) for _, kw in OPTSETS]
            f.write(json.dumps({"v": tag(v, None), "x": exp}) + "\n")
    texts = list(HAND_LOADS)
    r2 = random.Random(a.seed + 1000)
    for _ in range(max(1000, a.n // 2)):
        v = rand_value(r2)
        kw = r2.choice(OPTSETS)[1]
        try:
            t = json.dumps(v, **{k: x for k, x in kw.items() if k not in ("skipkeys", "allow_nan")})
        except (ValueError, TypeError):
            continue
        texts.append(t)
        if r2.random() < 0.6:
            texts.append(mutate_text(r2, t))
    with open(os.path.join(a.out, "loads.jsonl"), "w") as f:
        for t in texts:
            try:
                tb = t.encode("utf-8")
            except UnicodeEncodeError:
                continue
            f.write(json.dumps({"t": base64.b64encode(tb).decode(), "x": loads_expect(t)}) + "\n")
    print("wrote", a.n, "dumps values and", len(texts), "loads texts to", a.out, file=sys.stderr)

if __name__ == "__main__":
    main()
