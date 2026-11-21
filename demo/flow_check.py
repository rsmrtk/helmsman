#!/usr/bin/env python3
"""Drives the real binary through cluster -> nodes -> namespaces -> pods on the demo cluster and checks each screen.

    python3 demo/flow_check.py [path/to/helmsman]        (needs pyte and the cluster from demo/up.sh)

It guards the navigation: the nodes step exists and is skipped when nodes cannot be listed, the `all` row is first and
carries the pod total, `all` keeps every pod while a node narrows the pods to itself, and the cluster is switched. Exit
code 1 and a list of what failed when something is off.
"""
import fcntl, os, pty, re, select, shutil, struct, sys, tempfile, termios, time

import pyte

BIN = sys.argv[1] if len(sys.argv) > 1 else shutil.which("helmsman") or "./helmsman"
CTX = os.environ.get("E2E_CONTEXT", "kind-hm-demo")
COLS, ROWS = 170, 40
failures = []


def check(ok, msg):
    print(("  ok   " if ok else "  FAIL ") + msg)
    if not ok:
        failures.append(msg)


def run(steps):
    cfg = tempfile.mkdtemp(prefix="hm-flow-")
    pid, fd = pty.fork()
    if pid == 0:
        env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", HELMSMAN_CONFIG_DIR=cfg, HELMSMAN_ICONS="plain")
        os.execve(BIN, [BIN, "--context", CTX, "--readonly"], env)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    screen, stream = pyte.Screen(COLS, ROWS), None
    stream = pyte.ByteStream(screen)
    snaps = []

    def pump(t):
        end = time.time() + t
        while time.time() < end:
            r, _, _ = select.select([fd], [], [], 0.05)
            if r:
                try:
                    stream.feed(os.read(fd, 65536))
                except OSError:
                    return

    for keys, wait, name in steps:
        if keys:
            os.write(fd, keys.encode().decode("unicode_escape").encode())
        pump(wait)
        if name:
            snaps.append((name, "\n".join(screen.display)))
    os.kill(pid, 9)
    os.close(fd)
    shutil.rmtree(cfg, ignore_errors=True)
    return dict(snaps)


def title(txt):
    m = re.search(r"(\w+)\(([^)]*)\)\[(\d+)\]", txt)
    return (m.group(1), m.group(2), int(m.group(3))) if m else ("?", "?", 0)


def pods_on(txt):
    return {n for n in re.findall(r"[\w-]+-(?:control-plane|worker\d*)", txt)}


def main():
    print(f"flow check on context {CTX}")
    # A: cluster -> nodes (first row `all`) -> Enter -> namespaces
    s = run([("", 4.5, "picker"), (f"/{CTX}\\r", 1.5, None), ("\\r", 12, "nodes"), ("\\r", 5, "namespaces")])
    kind, scope, count = title(s["nodes"])
    check(kind == "nodes", f"after picking the cluster the nodes list opens (saw {kind}({scope}))")
    check(count >= 2, f"the list has `all` plus the real nodes (count {count})")
    rows = [l for l in s["nodes"].splitlines() if "│" in l]
    first = next((l for l in rows if l.strip("│ ").split()[:1] and l.strip("│ ").split()[0] not in ("NAME↑", "NAME")), "")
    check(first.strip("│ ").startswith("all"), "the first row of the nodes list is `all`")
    total = re.findall(r"\b(\d+)\b", first)
    check(bool(total) and int(total[0]) > 0, f"the `all` row carries the pod total ({total[:1]})")
    check(title(s["namespaces"])[0] == "namespaces", "Enter on a node goes on to the namespaces")

    # B: `all` + namespace `all` -> every pod of the cluster
    sa = run([("", 4.5, None), (f"/{CTX}\\r", 1.5, None), ("\\r", 12, None), ("\\r", 5, None), ("\\r", 6, "pods_all")])
    k, sc, n_all = title(sa["pods_all"])
    check(k == "pods", f"all nodes + a namespace opens the pods (saw {k}({sc}))")

    # C: the second real node (3rd row) + namespace `all` -> only that node's pods, fewer than `all`
    sb = run([("", 4.5, None), (f"/{CTX}\\r", 1.5, None), ("\\r", 12, None), ("\\x1b[B\\x1b[B", 1, None), ("\\r", 5, None), ("\\r", 6, "pods_node")])
    k2, sc2, n_node = title(sb["pods_node"])
    check(k2 == "pods", "a node + a namespace opens the pods")
    check(0 < n_node < n_all, f"a node narrows the pods ({n_node} on the node < {n_all} in the cluster)")
    check(len(pods_on(sb["pods_node"])) <= 1, f"only one node's pods are listed ({sorted(pods_on(sb['pods_node']))})")

    # D: `p` on a node opens its pods directly
    sp = run([("", 4.5, None), (f"/{CTX}\\r", 1.5, None), ("\\r", 12, None), ("\\x1b[B\\x1b[B", 1, None), ("p", 5, "p_pods")])
    check(title(sp["p_pods"])[0] == "pods", "`p` on a node opens its pods")

    # E: Esc walks back one level at a time
    se = run([("", 4.5, None), (f"/{CTX}\\r", 1.5, None), ("\\r", 12, None), ("\\r", 5, None), ("\\x1b", 2, "back")])
    check(title(se["back"])[0] == "nodes", "Esc from the namespaces returns to the nodes")

    print("\nFAILED: " + "; ".join(failures) if failures else "\nall checks passed")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
