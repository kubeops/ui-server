/*
Copyright AppsCode Inc. and Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scanner

import "kubeops.dev/scanner/apis/shared"

type SingleReport struct {
	SchemaVersion int
	ArtifactName  string
	ArtifactType  string
	Metadata      ImageMetadata
	Results       []Result
}

type ImageMetadata struct {
	Os          ImageOS
	ImageID     string
	DiffIDs     []string
	RepoTags    []string
	RepoDigests []string
	ImageConfig ImageConfig
}

type ImageOS struct {
	Family string
	Name   string
}

type ImageConfig struct {
	Architecture  string
	Author        string
	Container     string
	Created       shared.Time
	DockerVersion string
	History       []ImageHistory
	Os            string
	Rootfs        ImageRootfs
	Config        ImageRuntimeConfig
}

type ImageHistory struct {
	Created    shared.Time
	CreatedBy  string
	EmptyLayer bool
	Comment    string
}

type ImageRootfs struct {
	Type    string
	DiffIds []string
}

type ImageRuntimeConfig struct {
	Cmd         []string
	Env         []string
	Image       string
	Entrypoint  []string
	Labels      map[string]string
	ArgsEscaped bool
	StopSignal  string
}

type VulnerabilityLayer struct {
	Digest string
	DiffID string
}

type VulnerabilityDataSource struct {
	ID   string
	Name string
	URL  string
}

type CVSSNvd struct {
	V2Vector string
	V3Vector string
	V2Score  float64
	V3Score  float64
}

type CVSSRedhat struct {
	V2Vector string
	V3Vector string
	V2Score  float64
	V3Score  float64
}

type CVSS struct {
	Nvd    *CVSSNvd
	Redhat *CVSSRedhat
}

type Vulnerability struct {
	VulnerabilityID  string
	PkgName          string
	PkgID            string
	InstalledVersion string
	Layer            VulnerabilityLayer
	SeveritySource   string
	PrimaryURL       string
	DataSource       VulnerabilityDataSource
	Title            string
	Description      string
	Severity         string
	CweIDs           []string
	Cvss             CVSS
	References       []string
	PublishedDate    *shared.Time
	LastModifiedDate *shared.Time
	FixedVersion     string
}

type Result struct {
	Target          string
	Class           string
	Type            string
	Vulnerabilities []Vulnerability
}

//type Summary struct {
//	SchemaVersion int             `json:"SchemaVersion"`
//	ArtifactName  string          `json:"ArtifactName"`
//	ArtifactType  string          `json:"ArtifactType"`
//	Metadata      ImageMetadata   `json:"Metadata"`
//	Results       []SummaryResult `json:"Results"`
//}
//
//type SummaryResult struct {
//	Target          string         `json:"Target"`
//	Class           string         `json:"Class"`
//	Type            string         `json:"Type"`
//	Vulnerabilities map[string]int `json:"Vulnerabilities,omitempty"`
//}
