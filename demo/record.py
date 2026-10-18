#!/usr/bin/env python3
"""Records a short tour of helmsman as an asciinema cast (v2) in docs/demo.cast.

Play it with `asciinema play docs/demo.cast`, or upload it with `asciinema upload`.
Needs the demo cluster from demo/up.sh:

    python3 demo/record.py [path/to/helmsman]
"""
import fcntl, json, os, pty, select, shutil, struct, sys, tempfile, termios, time

BIN = sys.argv[1] if len(sys.argv) > 1 else shutil.which("helmsman") or "./helmsman"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "docs", "demo.cast")
COLS, ROWS = 140, 36
# (keys to send, seconds to wait afterwards)
TOUR = [
    ("", 3.5),                    # splash, then the cluster picker
    ("/kind-hm\\r", 1.2),         # filter
    ("\\r", 1.5),                 # enter the cluster -> namespaces
    ("/demo\\r", 1.2),
    ("\\r", 3.0),                 # pods
    ("/crash-loop\\r", 1.2),
    ("y", 2.0), ("\\x1b", 0.8),   # YAML in the pod's colour
    ("x", 3.0), ("\\x1b", 0.8),   # diagnose
    ("\\x14", 1.5), ("\\x14", 1.5), ("\\x14\\x14\\x14\\x14\\x14\\x14", 1.0),  # themes
    ("\\x06", 1.5), ("\\x1b", 0.8),  # jump palette
    (":overview\\r", 8.0),
]

def main():
    cfg = tempfile.mkdtemp(prefix="hm-cast-")
    pid, fd = pty.fork()
    if pid == 0:
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", HELMSMAN_CONFIG_DIR=cfg)
        os.execve(BIN, [BIN, "--context", "kind-hm-demo", "--readonly"], env)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    t0 = time.time()
    with open(OUT, "w", encoding="utf-8") as f:
        f.write(json.dumps({"version": 2, "width": COLS, "height": ROWS, "timestamp": int(t0),
                            "env": {"TERM": "xterm-256color", "SHELL": "/bin/zsh"}, "title": "helmsman"}) + "\n")

        def pump(seconds):
            end = time.time() + seconds
            while time.time() < end:
                r, _, _ = select.select([fd], [], [], 0.05)
                if r:
                    try:
                        data = os.read(fd, 65536)
                    except OSError:
                        return
                    f.write(json.dumps([round(time.time() - t0, 3), "o", data.decode("utf-8", "replace")]) + "\n")

        for keys, wait in TOUR:
            if keys:
                os.write(fd, keys.encode().decode("unicode_escape").encode())
            pump(wait)
    os.kill(pid, 9)
    os.close(fd)
    shutil.rmtree(cfg, ignore_errors=True)
    print("wrote", os.path.normpath(OUT), f"({time.time() - t0:.0f}s)")

if __name__ == "__main__":
    main()
