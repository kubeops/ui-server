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

package cmds

import (
	"context"
	"io"
	"kubeops.dev/ui-server/pkg/cmds/server"

	"github.com/spf13/cobra"
	v "gomodules.xyz/x/version"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
)

func NewCmdRun(ctx context.Context, out, errOut io.Writer) *cobra.Command {
	o := server.NewUIServerOptions(out, errOut)

	cmd := &cobra.Command{
		Use:               "run",
		Short:             "Launch a UI API server",
		Long:              "Launch a UI API server",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			klog.Infof("Starting ui server version %s+%s ...", v.Version.Version, v.Version.CommitHash)

			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunUIServer(ctx); err != nil {
				return err
			}
			return nil
		},
	}
	flags := cmd.Flags()
	o.AddFlags(flags)
	utilfeature.DefaultMutableFeatureGate.AddFlag(flags)

	return cmd
}
