/*
Copyright The Kubeshield Authors.

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

package cmds

import (
	"io"

	"kubeshield.dev/identity-server/pkg/cmds/server"

	"github.com/appscode/go/log"
	v "github.com/appscode/go/version"
	"github.com/spf13/cobra"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"kmodules.xyz/client-go/tools/cli"
)

func NewCmdRun(out, errOut io.Writer, stopCh <-chan struct{}) *cobra.Command {
	o := server.NewIdentityServerOptions(out, errOut)

	cmd := &cobra.Command{
		Use:               "run",
		Short:             "Launch an Identity API server",
		Long:              "Launch an Identity API server",
		DisableAutoGenTag: true,
		PreRun: func(c *cobra.Command, args []string) {
			cli.SendPeriodicAnalytics(c, v.Version.Version)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Infof("Starting identity server version %s+%s ...", v.Version.Version, v.Version.CommitHash)

			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunIdentityServer(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.RecommendedOptions.AddFlags(flags)
	utilfeature.DefaultMutableFeatureGate.AddFlag(flags)

	return cmd
}
