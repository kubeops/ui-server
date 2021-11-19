[![Go Report Card](https://goreportcard.com/badge/kubeops.dev/ui-server)](https://goreportcard.com/report/kubeops.dev/ui-server)
[![Build Status](https://github.com/kubeops/ui-server/workflows/CI/badge.svg)](https://github.com/kubeops/ui-server/actions?workflow=CI)
[![Docker Pulls](https://img.shields.io/docker/pulls/appscode/kube-ui-server.svg)](https://hub.docker.com/r/appscode/kube-ui-server/)
[![Slack](https://shields.io/badge/Join_Slack-salck?color=4A154B&logo=slack)](https://slack.appscode.com)
[![Twitter](https://img.shields.io/twitter/follow/kubeops.svg?style=social&logo=twitter&label=Follow)](https://twitter.com/intent/follow?screen_name=Kubeops)

# ui-server

Kubernetes UI Server is an [extended api server](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/) for Kubernetes.
This exposes a number of apis for a Kubernetes cluster, such as:

- `WhoAmI` service returns the user info of the user making the api call.
- `PodView` resource exposes actual resource usage by a Pod. The resource usage information is read from Prometheus.

## Deploy into a Kubernetes Cluster

You can deploy Indentity Server using Helm chart found [here](https://github.com/kubeops/installer/tree/master/charts/identity-server).

```console
helm repo add appscode https://charts.appscode.com/stable/
helm repo update

helm install kube-ui-server appscode/kube-ui-server
```

## Usage

```console
$ kubectl create -f artifacts/whoami.yaml -o yaml

apiVersion: identity.k8s.appscode.com/v1alpha1
kind: WhoAmI
response:
  user:
    groups:
    - system:masters
    - system:authenticated
    username: kubernetes-admin
```

## How It Woks

Identity Server is a Kubernetes extended apiserver (EAS). As an EAS, it has [access to the user](https://github.com/kubernetes/apiserver/blob/059effb5af64033b7d296c3347addd3226af60db/pkg/endpoints/filters/authentication.go#L49-L69) who is making an api call to the "whoami" server. You can find the core of the implementation [here](https://github.com/kubeops/ui-server/blob/78d0e36f63792380e7b630035579ab4f3bc2cc85/pkg/registry/identity/whoami/storage.go#L57).
