module kubeshield.dev/whoami

go 1.13

require (
	github.com/go-openapi/spec v0.19.2
	github.com/google/gofuzz v1.1.0
	github.com/spf13/cobra v0.0.5
	k8s.io/apiextensions-apiserver v0.16.8
	k8s.io/apimachinery v0.16.8
	k8s.io/apiserver v0.16.8
	k8s.io/client-go v0.16.8
	k8s.io/component-base v0.16.8
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
	sigs.k8s.io/yaml v1.2.0
)

replace (
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190821162956-65e3620a7ae7
	k8s.io/api => k8s.io/api v0.16.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.8
	k8s.io/apiserver => k8s.io/apiserver v0.16.8
	k8s.io/client-go => k8s.io/client-go v0.16.8
	k8s.io/code-generator => k8s.io/code-generator v0.16.8
	k8s.io/component-base => k8s.io/component-base v0.16.8
)
