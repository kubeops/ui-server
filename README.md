[![Go Report Card](https://goreportcard.com/badge/kubeops.dev/ui-server)](https://goreportcard.com/report/kubeops.dev/ui-server)
[![Build Status](https://github.com/kubeshield/identity-server/workflows/CI/badge.svg)](https://github.com/kubeshield/identity-server/actions?workflow=CI)
[![codecov](https://codecov.io/gh/kubeshield/identity-server/branch/master/graph/badge.svg)](https://codecov.io/gh/kubeshield/identity-server)
[![Docker Pulls](https://img.shields.io/docker/pulls/kubeshield/identity-server.svg)](https://hub.docker.com/r/kubeshield/identity-server/)
[![Slack](https://shields.io/badge/Join_Slack-salck?color=4A154B&logo=slack)](https://slack.appscode.com)
[![Twitter](https://img.shields.io/twitter/follow/kubeops.svg?style=social&logo=twitter&label=Follow)](https://twitter.com/intent/follow?screen_name=Kubeops)

# identity-server

Identity Server implements a Kubernetes ["whoami" service](https://github.com/kubernetes/kubernetes/issues/30784).

## Deploy into a Kubernetes Cluster

You can deploy Indentity Server using Helm chart found [here](https://github.com/kubeshield/installer/tree/master/charts/identity-server).

```console
helm repo add appscode https://charts.appscode.com/stable/
helm repo update

helm install identity-server appscode/identity-server
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

Identity Server is a Kubernetes extended apiserver (EAS). As an EAS, it has [access to the user](https://github.com/kubernetes/apiserver/blob/059effb5af64033b7d296c3347addd3226af60db/pkg/endpoints/filters/authentication.go#L49-L69) who is making an api call to the "whoami" server. You can find the core of the implementation [here](https://github.com/kubeshield/identity-server/blob/78d0e36f63792380e7b630035579ab4f3bc2cc85/pkg/registry/identity/whoami/storage.go#L57).
