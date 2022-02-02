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

package server

import (
	"github.com/spf13/pflag"
	restclient "k8s.io/client-go/rest"
)

type ExtraOptions struct {
	QPS   float64
	Burst int
}

func NewExtraOptions() *ExtraOptions {
	return &ExtraOptions{
		QPS:   1e6,
		Burst: 1e6,
	}
}

func (s *ExtraOptions) AddFlags(fs *pflag.FlagSet) {
	fs.Float64Var(&s.QPS, "qps", s.QPS, "The maximum QPS to the master from this client")
	fs.IntVar(&s.Burst, "burst", s.Burst, "The maximum burst for throttle")
}

func (s *ExtraOptions) ApplyTo(clientConfig *restclient.Config) error {
	clientConfig.QPS = float32(s.QPS)
	clientConfig.Burst = s.Burst

	return nil
}
