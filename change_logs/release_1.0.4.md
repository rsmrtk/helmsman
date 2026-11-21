# helmsman v1.0.4

## `helmsman init`

- One command per cloud: `helmsman init aws`, `helmsman init gcp`, `helmsman init azure` (plain `helmsman init` still
  does all three). `--region` (AWS), `--dry-run` and `--no-animation` work on all of them. The `--provider` flag from
  v1.0.3 is replaced by these commands.
- The provider's own logo is the loading animation, drawn once and recoloured in place (no repeated lines, also in a
  narrow window): a beam of light sweeps across AWS, four flat Google colours scroll through Google Cloud as hard-edged diagonal bands (no gradients), a wave of blue
  rolls through Azure. The lettering is real block-letter art (ANSI Shadow) and the heading carries the provider's Nerd Font glyph. A provider that
  answers instantly still shows its animation for a moment. Results are held and printed under the finished logo.
  `helmsman init demo` plays all three without contacting any cloud.
- Animation runs only on a terminal at least 45 columns wide ; `--no-animation`, `HELMSMAN_NO_ANIM=1`, a pipe or
  `NO_COLOR` give plain text, and `HELMSMAN_ICONS=plain` swaps the icons for text. The cursor and line wrapping are always
  restored, also after Ctrl-C.
- Output is one block per provider with a status mark per line (green ok, red problem, amber skipped). Errors say what
  happened and what to do, with the command on its own line to copy: an AWS region blocked for the account, an expired
  AWS session, a disabled Kubernetes Engine API (with the exact `gcloud services enable` command for your project), a
  missing sign-in, a missing CLI.
- The account line is short: `AccountFullAccessRole @ 886312445846 · region us-east-1`.
- In kitty (and other terminals that answer the Kitty text sizing query) the provider glyph in the heading is drawn at twice its size, spanning two rows with the name and service beside it. Other terminals keep the one-line heading, so the glyph never disappears. `HELMSMAN_ICON_SCALE=1|2|3|4` overrides.

## `helmsman doctor`

- Checks the kubeconfig, whether the cluster behind the current context answers, the terminal (true color, animation,
  the big heading glyph), a Nerd Font for the icons (kitty and WezTerm ship their own symbols) and the cloud CLIs.
  It changes nothing, prints the command that fixes each problem and exits non-zero when a check fails.

## Project

- Releases are signed (cosign, keyless): every release has `checksums.sha256` with a `.sigstore.json` bundle.
- The lint check only judges changed lines; `demo/flow_check.py` drives the cluster, nodes, namespaces, pods flow on the
  demo cluster; a monthly workflow opens an issue when upstream k9s has a new release (see `docs/upstream.md`).
- README images and the recorded tour show the new flow.

