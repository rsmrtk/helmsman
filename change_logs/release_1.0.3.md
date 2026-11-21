# helmsman v1.0.3

## `helmsman init`

- `helmsman init` finds the EKS, GKE and AKS clusters in the accounts you are already signed in to (`aws`, `gcloud`,
  `az`) and writes their credentials into your kubeconfig, so they show up in helmsman and kubectl from any terminal
  without `export KUBECONFIG=...`. Your current context is left as it was.
- `--provider aws|gcp|azure`, `--region <aws-region>` (repeatable), `--dry-run`. Regions or services you cannot list
  are skipped, not treated as errors. EKS contexts are named after the cluster instead of the full ARN.
