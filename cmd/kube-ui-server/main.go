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

package main

import (
	"os"
	"runtime"

	"kubeops.dev/ui-server/pkg/cmds"

	_ "go.bytebuilders.dev/license-verifier/info"
	"gomodules.xyz/logs"
	_ "k8s.io/client-go/kubernetes/fake"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
)

func main() {
	if err := realMain(); err != nil {
		klog.Fatalln("Error in kube-ui-server Main:", err)
	}
}

func realMain() error {
	rootCmd := cmds.NewRootCmd()
	logs.Init(rootCmd, true)
	defer logs.FlushLogs()

	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	return rootCmd.Execute()
}
