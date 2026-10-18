#!/usr/bin/env python3
"""Turns docs/demo.cast into an animated SVG (docs/img/tour.svg) that GitHub plays inside a README.

    python3 demo/record.py && python3 demo/tour_svg.py
"""
import json, os, re, sys

import pyte

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import record  # noqa: E402
import screenshots  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
CAST = os.path.join(HERE, "..", "docs", "demo.cast")
OUT = os.path.join(HERE, "..", "docs", "img", "tour.svg")
HOLD_MIN = 1.2  # a frame stays at least this long


def replay(cast_path, times):
    """Returns the screen as it looked at each time in `times` (ascending)."""
    screen = pyte.Screen(record.COLS, record.ROWS)
    stream = pyte.Stream(screen)
    with open(cast_path, encoding="utf-8") as f:
        next(f)
        events = [json.loads(l) for l in f if l.strip()]
    snaps, i = [], 0
    for t in times:
        while i < len(events) and events[i][0] <= t:
            stream.feed(events[i][2])
            i += 1
        snap = pyte.Screen(record.COLS, record.ROWS)
        for y in range(record.ROWS):
            for x, ch in screen.buffer[y].items():
                snap.buffer[y][x] = ch
        snaps.append(snap)
    return snaps


def body(svg):
    return svg[svg.index("<g font-family"):svg.rindex("</svg>")]


def main():
    screenshots.COLS, screenshots.ROWS = record.COLS, record.ROWS
    # frame boundaries follow the tour's own waits, the first wait including start-up
    ends, t = [], 0.0
    for _, wait in record.TOUR:
        t += wait
        ends.append(t)
    keep = [i for i, (keys, wait) in enumerate(record.TOUR) if wait >= HOLD_MIN or i == len(record.TOUR) - 1]
    snaps = replay(CAST, [ends[i] for i in keep])

    total = sum(max(record.TOUR[i][1], HOLD_MIN) for i in keep)
    frames, at = [], 0.0
    for snap, i in zip(snaps, keep):
        hold = max(record.TOUR[i][1], HOLD_MIN)
        frames.append((at, at + hold, screenshots.to_svg(snap, "helmsman")))
        at += hold

    first = frames[0][2]
    head = first[: first.index("<g font-family")]
    out = [head]
    for a, b, svg in frames:
        ka, kb = a / total, b / total
        keys = f"0;{ka:.4f};{kb:.4f};1" if kb < 1 else f"0;{ka:.4f};1"
        vals = "0;1;0;0" if kb < 1 else "0;1;1"
        out.append(f'<g opacity="0"><animate attributeName="opacity" calcMode="discrete" values="{vals}" keyTimes="{keys}" dur="{total:.1f}s" repeatCount="indefinite"/>')
        out.append(body(svg).replace("</g>", "</g>", 1))
        out.append("</g>")
    out.append("</svg>")
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as f:
        f.write("\n".join(out))
    print("wrote", os.path.normpath(OUT), f"{len(frames)} frames, {total:.0f}s loop")


if __name__ == "__main__":
    main()
