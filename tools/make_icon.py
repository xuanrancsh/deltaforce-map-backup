#!/usr/bin/env python3
"""Generate assets/icon.ico + assets/icon.png for DeltaForceMapBackup.

Pure standard library (no Pillow). Design: rounded-square blue tile with a
white right-arrow (backup) and an amber left-arrow (restore).
"""
import os
import struct
import zlib

HERE = os.path.dirname(os.path.abspath(__file__))
OUT_DIR = os.path.normpath(os.path.join(HERE, "..", "assets"))

BG_TOP = (37, 99, 235)
BG_BOTTOM = (23, 57, 158)
WHITE = (255, 255, 255)
AMBER = (251, 191, 36)

SS = 4  # supersample factor

ARROW_RIGHT = {
    "shaft": (0.200, 0.660, 0.315, 0.405),
    "head": ((0.600, 0.225), (0.845, 0.360), (0.600, 0.495)),
}
ARROW_LEFT = {
    "shaft": (0.340, 0.800, 0.595, 0.685),
    "head": ((0.400, 0.505), (0.155, 0.640), (0.400, 0.775)),
}


def clamp(v, lo, hi):
    return lo if v < lo else (hi if v > hi else v)


def in_round_rect(x, y, s, r):
    cx = clamp(x, r, s - r)
    cy = clamp(y, r, s - r)
    dx = x - cx
    dy = y - cy
    return dx * dx + dy * dy <= r * r


def _sign(px, py, a, b):
    return (px - b[0]) * (a[1] - b[1]) - (a[0] - b[0]) * (py - b[1])


def in_tri(px, py, a, b, c):
    d1 = _sign(px, py, a, b)
    d2 = _sign(px, py, b, c)
    d3 = _sign(px, py, c, a)
    neg = d1 < 0 or d2 < 0 or d3 < 0
    pos = d1 > 0 or d2 > 0 or d3 > 0
    return not (neg and pos)


def in_arrow(px, py, s, geo):
    x0, x1, y0, y1 = geo["shaft"]
    if x0 * s <= px <= x1 * s and y0 * s <= py <= y1 * s:
        return True
    a, b, c = [(p[0] * s, p[1] * s) for p in geo["head"]]
    return in_tri(px, py, a, b, c)


def render(n):
    s = n * SS
    r = 0.2236 * s
    buf = bytearray(s * s * 4)
    for y in range(s):
        py = y + 0.5
        t = py / s
        br = BG_TOP[0] + (BG_BOTTOM[0] - BG_TOP[0]) * t
        bg = BG_TOP[1] + (BG_BOTTOM[1] - BG_TOP[1]) * t
        bb = BG_TOP[2] + (BG_BOTTOM[2] - BG_TOP[2]) * t
        row = y * s * 4
        for x in range(s):
            px = x + 0.5
            if not in_round_rect(px, py, s, r):
                continue
            if in_arrow(px, py, s, ARROW_RIGHT):
                cr, cg, cb = WHITE
            elif in_arrow(px, py, s, ARROW_LEFT):
                cr, cg, cb = AMBER
            else:
                cr, cg, cb = br, bg, bb
            i = row + x * 4
            buf[i] = int(cr)
            buf[i + 1] = int(cg)
            buf[i + 2] = int(cb)
            buf[i + 3] = 255

    out = bytearray(n * n * 4)
    inv = 1.0 / (SS * SS)
    for y in range(n):
        for x in range(n):
            ar = ag = ab = aa = 0
            for dy in range(SS):
                base = (y * SS + dy) * s * 4 + x * SS * 4
                for dx in range(SS):
                    p = base + dx * 4
                    ar += buf[p]
                    ag += buf[p + 1]
                    ab += buf[p + 2]
                    aa += buf[p + 3]
            i = (y * n + x) * 4
            out[i] = int(ar * inv + 0.5)
            out[i + 1] = int(ag * inv + 0.5)
            out[i + 2] = int(ab * inv + 0.5)
            out[i + 3] = int(aa * inv + 0.5)
    return bytes(out)


def write_png(path, n, rgba):
    raw = bytearray()
    for y in range(n):
        raw.append(0)
        raw += rgba[y * n * 4:(y + 1) * n * 4]

    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + \
            struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    ihdr = struct.pack(">IIBBBBB", n, n, 8, 6, 0, 0, 0)
    blob = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + \
        chunk(b"IDAT", zlib.compress(bytes(raw), 9)) + chunk(b"IEND", b"")
    with open(path, "wb") as fh:
        fh.write(blob)


def write_ico(path, images):
    entries = []
    for n, rgba in images:
        hdr = struct.pack("<IiiHHIIiiII", 40, n, n * 2, 1, 32, 0, 0, 0, 0, 0, 0)
        xor = bytearray()
        for y in range(n - 1, -1, -1):
            for x in range(n):
                i = (y * n + x) * 4
                xor += bytes((rgba[i + 2], rgba[i + 1], rgba[i], rgba[i + 3]))
        androw = ((n + 31) // 32) * 4
        entries.append((n, hdr + bytes(xor) + bytes(androw * n)))

    count = len(entries)
    out = bytearray(struct.pack("<HHH", 0, 1, count))
    offset = 6 + 16 * count
    for n, data in entries:
        w = 0 if n >= 256 else n
        out += struct.pack("<BBBBHHII", w, w, 0, 0, 1, 32, len(data), offset)
        offset += len(data)
    for _, data in entries:
        out += data
    with open(path, "wb") as fh:
        fh.write(bytes(out))


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    sizes = [16, 32, 48, 64, 128, 256]
    images = []
    for n in sizes:
        images.append((n, render(n)))
        print("rendered %dx%d" % (n, n))
    write_ico(os.path.join(OUT_DIR, "icon.ico"), images)
    write_png(os.path.join(OUT_DIR, "icon.png"), 256, images[-1][1])
    print("wrote", os.path.join(OUT_DIR, "icon.ico"))
    print("wrote", os.path.join(OUT_DIR, "icon.png"))


if __name__ == "__main__":
    main()
