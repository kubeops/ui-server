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
	"flag"
	"os"

	"kubeshield.dev/identity-server/client/clientset/versioned/scheme"

	"github.com/appscode/go/flags"
	v "github.com/appscode/go/version"
	"github.com/spf13/cobra"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"kmodules.xyz/client-go/logs"
	"kmodules.xyz/client-go/tools/cli"
)

func NewRootCmd() *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:               "grafana-operator [command]",
		Short:             `Vault Operator by AppsCode - HashiCorp Vault Operator for Kubernetes`,
		DisableAutoGenTag: true,
		PersistentPreRun: func(c *cobra.Command, args []string) {
			flags.DumpAll(c.Flags())
			cli.SendAnalytics(c, v.Version.Version)

			utilruntime.Must(scheme.AddToScheme(clientsetscheme.Scheme))
		},
	}
	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	logs.ParseFlags()
	rootCmd.PersistentFlags().BoolVar(&cli.EnableAnalytics, "enable-analytics", cli.EnableAnalytics, "Send analytical events to Google Analytics")

	rootCmd.AddCommand(v.NewCmdVersion())
	stopCh := genericapiserver.SetupSignalHandler()
	rootCmd.AddCommand(NewCmdRun(os.Stdout, os.Stderr, stopCh))

	return rootCmd
}
