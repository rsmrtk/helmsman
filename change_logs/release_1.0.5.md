# helmsman v1.0.5

## `helmsman init all | local`

- `helmsman init all` (also plain `helmsman init`) imports everything: local clusters and every cloud you are signed in to.
  `helmsman init local` does only the local part; targets combine in any order: `helmsman init local aws`.
  `aws`, `gcp` and `azure` still work on their own. An unknown word is an error that lists the valid ones (v1.0.4 ignored it
  and ran the clouds).
- `local` finds kubeconfig files in the git repository you are in (`build/kubeconfig-eks`, `kubeconfig.yaml`, `.kube/config`,
  three levels deep; outside a repository only the current directory) and kind, k3d and minikube clusters. Contexts are
  merged into your kubeconfig without touching what is there or the current context; running it again changes nothing;
  a name taken by another cluster is added as `<repo>-<name>`; relative certificate paths become absolute.
- The Kubernetes block shows the K8S lettering with a blue gradient flowing across it.
- When everything is already imported, the summary says so instead of "No clusters found".
