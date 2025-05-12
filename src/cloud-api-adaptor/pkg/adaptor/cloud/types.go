// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package cloud

import (
	"context"
	"strings"
	"sync"

	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/pkg/adaptor/k8sops"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/pkg/adaptor/proxy"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/pkg/podnetwork"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/pkg/podnetwork/tunneler"
	provider "github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers"
	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/util/cloudinit"
	pb "github.com/kata-containers/kata-containers/src/runtime/protocols/hypervisor"

	"github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/pkg/securecomms/wnssh"
)

type Service interface {
	pb.HypervisorService
	GetInstanceID(ctx context.Context, podNamespace, podName string, wait bool) (string, error)
	ConfigVerifier() error
	Teardown() error
	InitialisePool() error
	AllocateIP() (string, error)
	ReleaseIP(ip string) error
	DeletePool() error
}

type cloudService struct {
	provider      provider.Provider
	proxyFactory  proxy.Factory
	workerNode    podnetwork.WorkerNode
	sandboxes     map[sandboxID]*sandbox
	cond          *sync.Cond
	mutex         sync.Mutex
	ppService     *k8sops.PeerPodService
	sshClient     *wnssh.SshClient
	serverConfig  *ServerConfig
	poolingConfig *PoolingConfig
}

type sandboxID string

type sandbox struct {
	agentProxy    proxy.AgentProxy
	podNetwork    *tunneler.Config
	cloudConfig   *cloudinit.CloudConfig
	id            sandboxID
	ip            string
	podName       string
	podNamespace  string
	instanceName  string
	instanceID    string
	netNSPath     string
	spec          provider.InstanceTypeSpec
	sshClientInst *wnssh.SshClientInstance
}

type PoolingConfig struct {
	UsePooling          bool
	PoolVMs             *provider.IPPool
	IPs                 StringSlice
	PreCreatedInstances *[]provider.Instance
	PoolSize            int
}

type StringSlice []string

func (s *StringSlice) String() string {
	return strings.Join(*s, ", ")

}

func (s *StringSlice) Set(value string) error {
	parts := strings.Split(value, ",")

	for _, part := range parts {
		*s = append(*s, strings.TrimSpace(part))
	}
	return nil
}
