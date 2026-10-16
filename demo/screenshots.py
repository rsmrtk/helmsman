#!/usr/bin/env python3
"""Renders helmsman screens to SVG for the README.

It runs the real binary in a pseudo-terminal, replays a few keystrokes, reads
the screen with pyte and writes docs/img/*.svg. Needs `pip install pyte` and the
demo cluster from demo/up.sh:

    python3 demo/screenshots.py [path/to/helmsman]
"""
import fcntl
import html
import os
import pty
import select
import shutil
import struct
import sys
import tempfile
import termios
import time

import pyte

BIN = sys.argv[1] if len(sys.argv) > 1 else shutil.which("helmsman") or "./helmsman"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "docs", "img")
COLS, ROWS = 150, 36
CELL_W, CELL_H, FONT = 8.4, 18.0, 14
BG, FG = "#0b0e14", "#cdd6f4"
DEMO = ["--context", "kind-hm-demo", "--readonly"]

NAMED = {
    "black": "#000000", "red": "#cc0000", "green": "#4e9a06", "brown": "#c4a000", "blue": "#3465a4",
    "magenta": "#75507b", "cyan": "#06989a", "white": "#d3d7cf", "brightblack": "#555753",
    "brightred": "#ef2929", "brightgreen": "#8ae234", "brightbrown": "#fce94f", "brightblue": "#729fcf",
    "brightmagenta": "#ad7fa8", "brightcyan": "#34e2e2", "brightwhite": "#eeeeec",
}


def colour(c, default):
    if c in (None, "default"):
        return default
    if len(c) == 6 and all(ch in "0123456789abcdefABCDEF" for ch in c):
        return "#" + c.lower()
    return NAMED.get(c, default)


def session(args, steps, config_dir):
    """Runs the binary, plays steps [(keys, seconds)] and returns the final screen."""
    pid, fd = pty.fork()
    if pid == 0:
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", HELMSMAN_CONFIG_DIR=config_dir)
        os.execve(BIN, [BIN] + args, env)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    screen = pyte.Screen(COLS, ROWS)
    stream = pyte.ByteStream(screen)

    def pump(t):
        end = time.time() + t
        while time.time() < end:
            r, _, _ = select.select([fd], [], [], 0.05)
            if r:
                try:
                    stream.feed(os.read(fd, 65536))
                except OSError:
                    return

    for keys, wait in steps:
        if keys:
            os.write(fd, keys.encode().decode("unicode_escape").encode())
        pump(wait)
    os.kill(pid, 9)
    os.close(fd)  # closing the pty first lets the dying child exit
    try:
        os.waitpid(pid, os.WNOHANG)
    except ChildProcessError:
        pass
    return screen


def to_svg(screen, title):
    rows = [y for y in range(ROWS) if any(screen.buffer[y][x].data.strip() for x in range(COLS))]
    last = (max(rows) if rows else 0) + 1
    pad = 14
    width = COLS * CELL_W + pad * 2
    height = last * CELL_H + pad * 2 + 28
    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width:.0f}" height="{height:.0f}" viewBox="0 0 {width:.0f} {height:.0f}" role="img" aria-label="{html.escape(title)}">',
        f'<rect width="100%" height="100%" rx="10" fill="{BG}"/>',
        '<circle cx="22" cy="18" r="6" fill="#ff5f56"/><circle cx="42" cy="18" r="6" fill="#ffbd2e"/><circle cx="62" cy="18" r="6" fill="#27c93f"/>',
        f'<text x="{width / 2:.0f}" y="22" text-anchor="middle" font-family="Menlo,Monaco,Consolas,monospace" font-size="12" fill="#6c7086">{html.escape(title)}</text>',
        f'<g font-family="Menlo,Monaco,Consolas,\'DejaVu Sans Mono\',monospace" font-size="{FONT}" xml:space="preserve">',
    ]
    for y in range(last):
        base = 28 + pad + y * CELL_H
        x = 0
        while x < COLS:
            ch = screen.buffer[y][x]
            fg, bg, bold, rev = ch.fg, ch.bg, ch.bold, ch.reverse
            x2 = x
            while x2 < COLS:
                c2 = screen.buffer[y][x2]
                if (c2.fg, c2.bg, c2.bold, c2.reverse) != (fg, bg, bold, rev):
                    break
                x2 += 1
            text = "".join(screen.buffer[y][i].data or " " for i in range(x, x2))
            f, b = colour(fg, FG), colour(bg, None)
            if rev:
                f, b = (b or BG), colour(fg, FG)
            if b and b != BG:
                out.append(f'<rect x="{pad + x * CELL_W:.1f}" y="{base - CELL_H + 4:.1f}" width="{(x2 - x) * CELL_W:.1f}" height="{CELL_H:.1f}" fill="{b}"/>')
            if text.strip():
                weight = ' font-weight="bold"' if bold else ""
                # one x per glyph keeps the terminal grid exact whatever font the viewer falls back to
                xs = " ".join(f"{pad + (x + i) * CELL_W:.1f}" for i in range(len(text)))
                out.append(f'<text x="{xs}" y="{base:.1f}" fill="{f}"{weight}>{html.escape(text)}</text>')
            x = x2
    out.append("</g></svg>")
    return "\n".join(out)


SHOTS = [
    ("cluster-picker", "helmsman: start on the cluster picker", [], [("", 4.5)]),
    ("splash", "helmsman: animated splash", [], [("", 1.0)]),
    ("pods", "pods in the status palette", ["-c", "pods", "-n", "demo"] + DEMO[:0], [("", 6)]),
    ("yaml", "YAML framed in the pod's status colour", ["-c", "pods", "-n", "demo"], [("", 6), ("/crash-loop\\r", 1.2), ("y", 2)]),
    ("diagnose", "x: why is this pod unhealthy?", ["-c", "pods", "-n", "demo"], [("", 6), ("/crash-loop\\r", 1.2), ("x", 3)]),
    ("overview", ":overview: every cluster at a glance", ["-c", "pods", "-n", "demo"], [("", 5), (":overview\\r", 9)]),
    ("jump", "Ctrl-F: jump palette", ["-c", "pods", "-n", "demo"], [("", 5), ("\\x06", 1.5)]),
    ("theme-ember", "Ctrl-T: themes (ember)", ["-c", "pods", "-n", "demo"], [("", 5), ("\\x14", 2)]),
    ("theme-ocean", "Ctrl-T: themes (ocean)", ["-c", "pods", "-n", "demo"], [("", 5), ("\\x14\\x14\\x14\\x14", 2)]),
]

if __name__ == "__main__":
    os.makedirs(OUT, exist_ok=True)
    only = set(os.environ.get("ONLY", "").split(",")) - {""}
    for name, title, args, steps in SHOTS:
        if only and name not in only:
            continue
        cfg = tempfile.mkdtemp(prefix="hm-shot-")
        try:
            scr = session(args + DEMO, steps, cfg)
        finally:
            shutil.rmtree(cfg, ignore_errors=True)
        path = os.path.join(OUT, name + ".svg")
        with open(path, "w", encoding="utf-8") as f:
            f.write(to_svg(scr, title))
        print("wrote", os.path.normpath(path))
