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
$ kubectl create -f https://github.com/kubeshield/identity-server/raw/v0.1.1/artifacts/whoami.yaml --v=8

I0414 10:07:56.932224    7000 request.go:1017] Request Body: {"apiVersion":"identity.k8s.appscode.com/v1alpha1","kind":"WhoAmI"}
I0414 10:07:56.932282    7000 round_trippers.go:423] curl -k -v -XPOST  -H "Content-Type: application/json" -H "User-Agent: kubectl/v1.17.0 (linux/amd64) kubernetes/70132b0" -H "Accept: application/json" 'https://127.0.0.1:32769/apis/identity.k8s.appscode.com/v1alpha1/whoamis'
I0414 10:07:56.934299    7000 round_trippers.go:443] POST https://127.0.0.1:32769/apis/identity.k8s.appscode.com/v1alpha1/whoamis 201 Created in 1 milliseconds
I0414 10:07:56.934320    7000 round_trippers.go:449] Response Headers:
I0414 10:07:56.934329    7000 round_trippers.go:452]     Cache-Control: no-cache, private
I0414 10:07:56.934337    7000 round_trippers.go:452]     Content-Type: application/json
I0414 10:07:56.934342    7000 round_trippers.go:452]     Date: Tue, 14 Apr 2020 17:07:56 GMT
I0414 10:07:56.934348    7000 round_trippers.go:452]     Content-Length: 168
I0414 10:07:56.934375    7000 request.go:1017] Response Body: {"kind":"WhoAmI","apiVersion":"identity.k8s.appscode.com/v1alpha1","response":{"user":{"username":"kubernetes-admin","groups":["system:masters","system:authenticated"]}}}
whoami.identity.k8s.appscode.com/<unknown> created
```

## How It Woks

Identity Server is a Kubernetes extended apiserver (EAS). As an EAS, it has [access to the user](https://github.com/kubernetes/apiserver/blob/059effb5af64033b7d296c3347addd3226af60db/pkg/endpoints/filters/authentication.go#L49-L69) who is making an api call to the "whoami" server. You can find the core of the implementation [here](https://github.com/kubeshield/identity-server/blob/78d0e36f63792380e7b630035579ab4f3bc2cc85/pkg/registry/identity/whoami/storage.go#L57).
