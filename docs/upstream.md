# Following upstream k9s

helmsman is built on the k9s code base (Apache-2.0, see `NOTICE`). The file `UPSTREAM` in the repository root holds the
k9s release the code was last brought in line with: **v0.50.18**.

A monthly workflow (`.github/workflows/upstream.yml`) compares it with the latest k9s release and opens an issue when
upstream is ahead, listing the release notes. It never changes code by itself.

## When an issue appears

1. Read the k9s release notes: security fixes and Kubernetes API changes matter most; features only if you want them.
2. Add upstream as a remote once: `git remote add upstream https://github.com/derailed/k9s.git`
3. Look at what changed in the files helmsman also changed: `git diff v0.50.18 <new-tag> -- internal/view internal/render`
4. Port what you want (a cherry-pick rarely applies cleanly because the history is separate, so apply the patch by hand
   or with `git apply -3`), run `go test ./...` and `python3 demo/flow_check.py` on the demo cluster.
5. Write the new version into `UPSTREAM` and close the issue.
