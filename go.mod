module kubeops.dev/ui-server

go 1.18

require (
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/fluxcd/helm-controller/api v0.26.0
	github.com/fluxcd/source-controller/api v0.31.0
	github.com/go-logr/logr v1.2.3
	github.com/go-pkgz/expirable-cache/v2 v2.0.0
	github.com/gobuffalo/flect v0.3.0
	github.com/google/gofuzz v1.2.0
	github.com/google/uuid v1.3.0
	github.com/graphql-go/graphql v0.8.0
	github.com/graphql-go/handler v0.2.3
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.14.0
	github.com/spf13/cobra v1.6.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.1
	github.com/zeebo/xxh3 v1.0.2
	go.bytebuilders.dev/license-verifier v0.13.0
	go.openviz.dev/apimachinery v0.0.4
	golang.org/x/sync v0.1.0
	gomodules.xyz/jsonpath v0.0.2
	gomodules.xyz/logs v0.0.6
	gomodules.xyz/password-generator v0.2.9
	gomodules.xyz/pointer v0.1.0
	gomodules.xyz/sets v0.2.1
	gomodules.xyz/x v0.0.14
	k8s.io/api v0.25.4
	k8s.io/apiextensions-apiserver v0.25.3
	k8s.io/apimachinery v0.25.4
	k8s.io/apiserver v0.25.3
	k8s.io/client-go v0.25.4
	k8s.io/component-base v0.25.4
	k8s.io/klog/v2 v2.80.1
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280
	k8s.io/kube-state-metrics/v2 v2.7.0
	kmodules.xyz/apiversion v0.2.0
	kmodules.xyz/authorizer v0.25.0
	kmodules.xyz/client-go v0.25.19
	kmodules.xyz/custom-resources v0.25.1
	kmodules.xyz/go-containerregistry v0.0.9
	kmodules.xyz/monitoring-agent-api v0.25.1
	kmodules.xyz/resource-metadata v0.16.0
	kmodules.xyz/resource-metrics v0.25.1
	kmodules.xyz/sets v0.24.0
	kubeops.dev/scanner v0.0.7-0.20230323044740-3a3f40a6bc44
	kubepack.dev/lib-helm v0.5.9
	kubepack.dev/preset v0.0.3
	sigs.k8s.io/cli-utils v0.33.0
	sigs.k8s.io/controller-runtime v0.13.1
	sigs.k8s.io/yaml v1.3.0
)

require (
	cloud.google.com/go/compute v1.18.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.28 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.21 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/PuerkitoBio/purell v1.2.0 // indirect
	github.com/alessio/shellescape v1.4.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.4.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1-0.20220316001817-d5090ed65664 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/fluxcd/pkg/apis/acl v0.1.0 // indirect
	github.com/fluxcd/pkg/apis/kustomize v0.6.0 // indirect
	github.com/fluxcd/pkg/apis/meta v0.17.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.1 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.4.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-containerregistry v0.14.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hexops/gotextdiff v1.0.3 // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/onsi/gomega v1.20.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.59.1 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	github.com/yudai/gojsondiff v1.0.0 // indirect
	github.com/yudai/golcs v0.0.0-20170316035057-ecda9a501e82 // indirect
	go.etcd.io/etcd/api/v3 v3.5.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.4 // indirect
	go.etcd.io/etcd/client/v3 v3.5.4 // indirect
	go.opentelemetry.io/contrib v0.20.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.28.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.32.0 // indirect
	go.opentelemetry.io/otel v1.7.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v0.30.0 // indirect
	go.opentelemetry.io/otel/sdk v1.7.0 // indirect
	go.opentelemetry.io/otel/sdk/export/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/trace v1.7.0 // indirect
	go.opentelemetry.io/proto/otlp v0.16.0 // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.21.0 // indirect
	golang.org/x/crypto v0.6.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/oauth2 v0.6.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/term v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	gomodules.xyz/atomic-writer v0.0.2 // indirect
	gomodules.xyz/clock v0.0.0-20200817085942-06523dba733f // indirect
	gomodules.xyz/encoding v0.0.7 // indirect
	gomodules.xyz/flags v0.1.3 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	gomodules.xyz/mergo v0.3.13 // indirect
	gomodules.xyz/wait v0.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230124163310-31e0e69b6fc2 // indirect
	google.golang.org/grpc v1.51.0 // indirect
	google.golang.org/protobuf v1.29.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	helm.sh/helm/v3 v3.0.0-00010101000000-000000000000 // indirect
	k8s.io/cli-runtime v0.25.2 // indirect
	k8s.io/component-helpers v0.25.2 // indirect
	k8s.io/utils v0.0.0-20221101230645-61b03e2f6476 // indirect
	kmodules.xyz/offshoot-api v0.25.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.33 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/kustomize/api v0.12.1 // indirect
	sigs.k8s.io/kustomize/kyaml v0.13.9 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace github.com/graphql-go/graphql => github.com/jamesdphillips/graphql-go v0.7.4-0.20220810211622-efd2a06de890

replace github.com/Masterminds/sprig/v3 => github.com/gomodules/sprig/v3 v3.2.3-0.20220405051441-0a8a99bac1b8

replace helm.sh/helm/v3 => github.com/kubepack/helm/v3 v3.10.1-0.20220922170417-3a8110bacfda

replace sigs.k8s.io/application => github.com/kmodules/application v0.8.4-0.20220917032906-876dc58b7366

replace (
	go.opencensus.io => go.opencensus.io v0.23.0
	go.opentelemetry.io/contrib => go.opentelemetry.io/contrib v0.20.0
	go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful => go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful v0.20.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp => go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.20.0
	go.opentelemetry.io/contrib/propagators => go.opentelemetry.io/contrib/propagators v0.20.0
	go.opentelemetry.io/otel => go.opentelemetry.io/otel v0.20.0
	go.opentelemetry.io/otel/exporters/otlp => go.opentelemetry.io/otel/exporters/otlp v0.20.0
	go.opentelemetry.io/otel/metric => go.opentelemetry.io/otel/metric v0.20.0
	go.opentelemetry.io/otel/oteltest => go.opentelemetry.io/otel/oteltest v0.20.0
	go.opentelemetry.io/otel/sdk => go.opentelemetry.io/otel/sdk v0.20.0
	go.opentelemetry.io/otel/sdk/export/metric => go.opentelemetry.io/otel/sdk/export/metric v0.20.0
	go.opentelemetry.io/otel/sdk/metric => go.opentelemetry.io/otel/sdk/metric v0.20.0
	go.opentelemetry.io/otel/trace => go.opentelemetry.io/otel/trace v0.20.0
	go.opentelemetry.io/proto/otlp => go.opentelemetry.io/proto/otlp v0.7.0
)
