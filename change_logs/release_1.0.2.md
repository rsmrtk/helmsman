# helmsman v1.0.2

First tagged release of helmsman, a terminal UI for Kubernetes. Licensed under Apache-2.0 (see `LICENSE`).

## Navigation

- Starts on the **cluster picker** instead of the pods of the current context.
- Drill-down is now **cluster → nodes → namespaces → pods**; `Esc` walks back one level at a time.
- The nodes list has a synthetic **`all`** row (it carries the total pod count) that selects every node at once.
  The node picked there narrows the pods shown after you pick a namespace; `all` keeps every pod.
  `Enter` continues to namespaces, `p` opens the pods of the highlighted node. `all` is refused for cordon,
  uncordon and drain. If the account may not list nodes, the nodes step is skipped.
- Table title counts pods per status; CPU/MEM show "requested" when metrics-server is not installed.

## Look

- Status palette for pods: healthy green, finished blue, waiting amber, broken red or pink, failing rows bold.
- YAML, describe and logs views take the colour of the pod's status.
- The synthetic `all` rows (nodes and namespaces) use the theme colour.
- New default skin in the `#92b3f4` palette; `Ctrl-T` cycles themes; ANSI logo that hides on narrow terminals.
- `CPU TREND` sparkline column on pods.

## New commands and keys

- `x` **Diagnose**: findings with advice, container state, latest events and the tail of previous logs on one screen;
  refreshes itself, `r` restarts the pod (with confirmation).
- `:overview`, `:warnings`, `:gpu`, `:drift <argo-app>`, `:diff <ctx-a> <ctx-b> [ns]` (also configmaps, secrets by hash,
  services, ingresses).
- `:cloud`: list AWS, GCP and Azure clusters through their CLIs, log in, import a cluster into kubeconfig.
  No credentials are stored.
- `:argo` and `:isvc` aliases; CRD tables are coloured by their status columns.
- `Ctrl-F` jump palette, `Ctrl-B` bookmarks, `Ctrl-Z` problems only.

## Project

- Go module `github.com/rsmrtk/helmsman`; the binary is `helmsman`.
- `demo/` brings up a local kind cluster with pods in every status; `e2e/` holds an end-to-end test (`-tags e2e`).
- Releases are built by GoReleaser on `v*` tags and published to GitHub Releases.

## Known limits

- `:cloud` was exercised against GCP only; AWS and Azure paths are untested.
- Early development: expect rough edges.
