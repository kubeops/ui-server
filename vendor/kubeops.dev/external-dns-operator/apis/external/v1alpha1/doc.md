Below are all the fields that are supported by the external dns as arguments. ADDED list contain the fields that are added in the crd, and NOT ADDED are for the remaining arguments
<br>Ref: https://github.com/kubernetes-sigs/external-dns/blob/master/pkg/apis/externaldns/types.go


ADDED
   -------------------------------------------------------------
   		APIServerURL                      string
   		KubeConfig                        string
   		RequestTimeout                    time.Duration
		ContourLoadBalancerService        string
   		Sources                           []string
   		Namespace                         string
   		AnnotationFilter                  string
   		LabelFilter                       string
   		FQDNTemplate                      string
   		CombineFQDNAndAnnotation          bool
   		IgnoreHostnameAnnotation          bool
   		IgnoreIngressTLSSpec              bool
   		IgnoreIngressRulesSpec            bool
   		GatewayNamespace                  string
   		GatewayLabelFilter                string
   		Compatibility                     string
   		PublishInternal                   bool
   		PublishHostIP                     bool
   		AlwaysPublishNotReadyAddresses    bool
   		ConnectorSourceServer             string
   		Provider                          string
   		DomainFilter                      []string
   		ExcludeDomains                    []string
   		RegexDomainFilter                 *regexp.Regexp
   		RegexDomainExclusion              *regexp.Regexp
		ZoneIDFilter                      []string
   		AWSZoneType                       string
   		AWSZoneTagFilter                  []string
   		AWSAssumeRole                     string
   		AWSBatchChangeSize                int
   		AWSBatchChangeInterval            time.Duration
   		AWSEvaluateTargetHealth           bool
   		AWSAPIRetries                     int
   		AWSPreferCNAME                    bool
   		AWSZoneCacheDuration              time.Duration
   		AWSSDServiceCleanup               bool
   		CloudflareProxied                 bool
   		CloudflareZonesPerPage            int
   		Policy                            string
   		Registry                          string
   		TXTOwnerID                        string
   		TXTPrefix                         string
   		TXTSuffix                         string
   		TXTWildcardReplacement            string
   		ManagedDNSRecordTypes             []string
   		OCPRouterName                     string
		AzureResourceGroup                string
		AzureSubscriptionID               string
		AzureUserAssignedIdentityClientID string

		GoogleProject                     string
		GoogleBatchChangeSize             int
		GoogleBatchChangeInterval         time.Duration
		GoogleZoneVisibility              string

   -------------------------------------------------------------
NOT ADDED
   -------------------------------------------------------------
   	DefaultTargets                    []string
   	GlooNamespace                     string
   	SkipperRouteGroupVersion          string

   	ZoneNameFilter                    []string
   	AlibabaCloudConfigFile            string
   	AlibabaCloudZoneType              string
   		*AzureConfigFile                   string

   	BluecatDNSConfiguration           string
   	BluecatConfigFile                 string
   	BluecatDNSView                    string
   	BluecatGatewayHost                string
   	BluecatRootZone                   string
   	BluecatDNSServerName              string
   	BluecatDNSDeployType              string
   	BluecatSkipTLSVerify              bool
   	CoreDNSPrefix                     string
   	RcodezeroTXTEncrypt               bool
   	AkamaiServiceConsumerDomain       string
   	AkamaiClientToken                 string
   	AkamaiClientSecret                string
   	AkamaiAccessToken                 string
   	AkamaiEdgercPath                  string
   	AkamaiEdgercSection               string
   	InfobloxGridHost                  string
   	InfobloxWapiPort                  int
   	InfobloxWapiUsername              string
   	InfobloxWapiPassword              string `secure:"yes"`
   	InfobloxWapiVersion               string
   	InfobloxSSLVerify                 bool
   	InfobloxView                      string
   	InfobloxMaxResults                int
   	InfobloxFQDNRegEx                 string
   	InfobloxCreatePTR                 bool
   	InfobloxCacheDuration             int
   	DynCustomerName                   string
   	DynUsername                       string
   	DynPassword                       string `secure:"yes"`
   	DynMinTTLSeconds                  int
   	OCIConfigFile                     string
   	InMemoryZones                     []string
   	OVHEndpoint                       string
   	OVHApiRateLimit                   int
   	PDNSServer                        string
   	PDNSAPIKey                        string `secure:"yes"`
   	PDNSTLSEnabled                    bool
   	TLSCA                             string
   	TLSClientCert                     string
   	TLSClientCertKey                  string
   	Interval                          time.Duration
   	MinEventSyncInterval              time.Duration
   	Once                              bool
   	DryRun                            bool
   	UpdateEvents                      bool
   	LogFormat                         string
   	MetricsAddress                    string
   	LogLevel                          string
   	TXTCacheInterval                  time.Duration
   	ExoscaleEndpoint                  string
   	ExoscaleAPIKey                    string `secure:"yes"`
   	ExoscaleAPISecret                 string `secure:"yes"`
   	CRDSourceAPIVersion               string
   	CRDSourceKind                     string
   	ServiceTypeFilter                 []string
   	CFAPIEndpoint                     string
   	CFUsername                        string
   	CFPassword                        string
   	RFC2136Host                       string
   	RFC2136Port                       int
   	RFC2136Zone                       string
   	RFC2136Insecure                   bool
   	RFC2136GSSTSIG                    bool
   	RFC2136KerberosRealm              string
   	RFC2136KerberosUsername           string
   	RFC2136KerberosPassword           string `secure:"yes"`
   	RFC2136TSIGKeyName                string
   	RFC2136TSIGSecret                 string `secure:"yes"`
   	RFC2136TSIGSecretAlg              string
   	RFC2136TAXFR                      bool
   	RFC2136MinTTL                     time.Duration
   	RFC2136BatchChangeSize            int
   	NS1Endpoint                       string
   	NS1IgnoreSSL                      bool
   	NS1MinTTLSeconds                  int
   	TransIPAccountName                string
   	TransIPPrivateKeyFile             string
   	DigitalOceanAPIPageSize           int
   	GoDaddyAPIKey                     string `secure:"yes"`
   	GoDaddySecretKey                  string `secure:"yes"`
   	GoDaddyTTL                        int64
   	GoDaddyOTE                        bool
   	IBMCloudProxied                   bool
   	IBMCloudConfigFile                string
---------------------------------------------------