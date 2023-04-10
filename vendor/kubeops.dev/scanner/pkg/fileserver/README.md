# fileserver

## kubectl-curl

- https://github.com/segmentio/kubectl-curl

## Examples

```
# browse
kubectl curl -k https://scanner-0:8443/files/ -n kubeops

# download
kubectl curl -k https://scanner-0:8443/files/a/b/kubectl -n kubeops > kubectl

# upload
kubectl curl -k -X POST -F file=@/opt/homebrew/bin/kubectl  https://scanner-0:8443/files/a/b -n kubeops
```
