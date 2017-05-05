/*
Copyright 2017 The Kubernetes Authors.

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

package framework

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
)

type kubeletOpt string

const (
	NodeStateTimeout            = 1 * time.Minute
	KubeletStart     kubeletOpt = "start"
	KubeletStop      kubeletOpt = "stop"
	KubeletRestart   kubeletOpt = "restart"
)

// kubeletCommand performs `start`, `restart`, or `stop` on the kubelet running on the node of the target pod and waits
// for the desired statues..
// - First issues the command via `systemctl`
// - If `systemctl` returns stderr "command not found, issues the command via `service`
// - If `service` also returns stderr "command not found", the test is aborted.
// Allowed kubeletOps are `kStart`, `kStop`, and `kRestart`
func KubeletCommand(kOp kubeletOpt, c clientset.Interface, pod *v1.Pod) error {
	nodeIP, err := GetHostExternalAddress(c, pod)
	if err != nil {
		return fmt.Errorf("Error getting HostExternalAddress:  %s", err)
	}
	nodeIP = nodeIP + ":22"
	systemctlCmd := fmt.Sprintf("sudo systemctl %s kubelet", string(kOp))
	Logf("Attempting `%s`", systemctlCmd)
	sshResult, err := SSH(systemctlCmd, nodeIP, TestContext.Provider)
	if err != nil {
		return fmt.Errorf("SSH to Node %q failed with error: %s", pod.Spec.NodeName, err)
	}
	LogSSHResult(sshResult)
	if strings.Contains(sshResult.Stderr, "command not found") {
		serviceCmd := fmt.Sprintf("sudo service kubelet %s", string(kOp))
		Logf("Attempting `%s`", serviceCmd)
		sshResult, err = SSH(serviceCmd, nodeIP, TestContext.Provider)
		if err != nil {
			return fmt.Errorf("SSH to Node %q failed with error: %s", pod.Spec.NodeName, err)
		}
		Logf("-[DEBUG]-  SETTING SSHRESULT.CODE=1")
		LogSSHResult(sshResult)
	} else if sshResult.Code != 0 {
		return fmt.Errorf("Failed to [%s] kubelet:\n%#v", string(kOp), sshResult)
	}
	// On restart, waiting for node NotReady prevents a race condition where the node takes a few moments to leave the
	// Ready state which in turn short circuits WaitForNodeToBeReady()
	if kOp == KubeletStop || kOp == KubeletRestart {
		if ok := WaitForNodeToBeNotReady(c, pod.Spec.NodeName, NodeStateTimeout); !ok {
			return fmt.Errorf("Node %s failed to enter NotReady state", pod.Spec.NodeName)
		}
	}
	if kOp == KubeletStart || kOp == KubeletRestart {
		if ok := WaitForNodeToBeReady(c, pod.Spec.NodeName, NodeStateTimeout); !ok {
			return fmt.Errorf("Node %s failed to enter Ready state", pod.Spec.NodeName)
		}
	}
	return nil
}
