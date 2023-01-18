//go:build ibmcloud
// +build ibmcloud

package registry

import (
	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/hypervisor"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/hypervisor/ibmcloud"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/podnetwork"
)

func newServer(cfg hypervisor.Config, cloudConfig interface{}, workerNode podnetwork.WorkerNode, daemonPort string) hypervisor.Server {
	if cfg.HypProvider == "ibmcloud-vpc" {
		return ibmcloud.NewVPCServer(cfg, cloudConfig.(ibmcloud.VpcConfig), workerNode, daemonPort)
	} else {
		return ibmcloud.NewPowerVCServer(cfg, cloudConfig.(ibmcloud.PowerVCConfig), workerNode, daemonPort)
	}
}
