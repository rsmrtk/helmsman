#!/bin/bash
# Spins up a kind cluster full of pods in every interesting state, to look at
# helmsman's colours: ./demo/up.sh   (remove with: kind delete cluster --name hm-demo)
set -euo pipefail
cd "$(dirname "$0")"
kind create cluster --name hm-demo --image kindest/node:v1.31.4 --config kind.yaml
kubectl --context kind-hm-demo apply -f metrics-server-kind.yaml
kubectl --context kind-hm-demo apply -f workloads.yaml
# a pod stuck in Terminating: needs a finalizer and a delete once it runs
kubectl --context kind-hm-demo -n demo wait --for=condition=Ready pod/terminating --timeout=120s
kubectl --context kind-hm-demo -n demo delete pod terminating --wait=false
echo "ready: hm --context kind-hm-demo -c pods -n demo"
