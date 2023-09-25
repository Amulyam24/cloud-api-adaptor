// Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/cloud"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/util"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/util/cloudinit"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
)

const maxInstanceNameLen = 63

var logger = log.New(log.Writer(), "[adaptor/cloud/openstack] ", log.LstdFlags|log.Lmsgprefix)

type openstackProvider struct {
	openstackService
	serviceConfig *Config
}

func NewProvider(config *Config) (cloud.Provider, error) {

	logger.Printf("openstack config: %#v", config.Redact())

	openstack, err := NewOpenStackService()
	if err != nil {
		return nil, err
	}

	return &openstackProvider{
		openstackService: *openstack,
		serviceConfig:    config,
	}, nil
}

func (p *openstackProvider) CreateInstance(ctx context.Context, podName, sandboxID string, cloudConfig cloudinit.CloudConfigGenerator, spec cloud.InstanceTypeSpec) (*cloud.Instance, error) {

	instanceName := util.GenerateInstanceName(podName, sandboxID, maxInstanceNameLen)

	userData, err := cloudConfig.Generate()
	if err != nil {
		return nil, err
	}

	body := servers.CreateOpts{
		Name:             instanceName,
		ImageRef:         p.serviceConfig.ImageID,
		FlavorRef:        p.serviceConfig.FlavorID,
		AvailabilityZone: p.serviceConfig.HostGroup,
		Networks: []servers.Network{
			{
				UUID: p.serviceConfig.NetworkID,
			},
		},
		UserData: []byte(userData),
	}

	createOpts := keypairs.CreateOptsExt{
		CreateOptsBuilder: body,
		KeyName:           "amulya-pub",
	}

	logger.Printf("CreateInstance: name: %q", instanceName)

	server, err := servers.Create(p.computeClient, createOpts).Extract()
	if err != nil {
		logger.Printf("failed to create an instance : %v", err)
		return nil, err
	}
	logger.Printf("Admin password: %s", server.AdminPass)

	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()

	logger.Printf("Waiting for instance to reach state: ACTIVE")
	err = retry.Do(
		func() error {
			in, err := servers.Get(p.computeClient, server.ID).Extract()
			if err != nil {
				return fmt.Errorf("failed to get the instance: %v", err)
			}

			if in.Status == "ERROR" {
				return fmt.Errorf("instance is in error state")
			}

			if in.Status == "ACTIVE" {
				logger.Printf("instance is in desired state: %s", in.Status)
				return nil
			}

			return fmt.Errorf("Instance failed to reach ACTIVE state")
		},
		retry.Context(ctx),
		retry.Attempts(0),
		retry.MaxDelay(5*time.Second),
	)

	if err != nil {
		logger.Print(err)
		return nil, err
	}

	networkName, err := p.GetNewtork(p.serviceConfig.NetworkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the network name: %w", err)
	}

	ips, err := p.getVMIPs(server.ID, *networkName)
	if err != nil {
		return nil, fmt.Errorf("failed to get IPs for the instance : %v", err)
	}

	return &cloud.Instance{
		ID:   server.ID,
		Name: instanceName,
		IPs:  ips,
	}, nil
}

func (p *openstackProvider) DeleteInstance(ctx context.Context, instanceID string) error {

	err := servers.Delete(p.computeClient, instanceID).ExtractErr()
	if err != nil {
		logger.Printf("failed to delete an instance: %v", err)
		return err
	}

	logger.Printf("deleted an instance %s", instanceID)
	return nil
}

func (p *openstackProvider) Teardown() error {
	return nil
}

func (p *openstackProvider) ConfigVerifier() error {
	ImageId := p.serviceConfig.ImageID
	if len(ImageId) == 0 {
		return fmt.Errorf("ImageId is empty")
	}
	return nil
}

func (p *openstackProvider) getVMIPs(id string, networkName string) ([]netip.Addr, error) {
	var ips []netip.Addr
	ins, err := servers.Get(p.computeClient, id).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get the instance: %w", err)
	}

	if ins.Addresses == nil {
		return nil, fmt.Errorf("address is nil for vm %s", ins.Name)
	}

	_, ok := ins.Addresses[networkName]
	if !ok {
		return nil, fmt.Errorf("failed to get network name for %s", ins.Name)
	}

	for i, networkAddresses := range ins.Addresses[networkName].([]interface{}) {
		address := networkAddresses.(map[string]interface{})
		if address["OS-EXT-IPS:type"] == "fixed" {
			if address["version"].(float64) == 4 {
				addr := address["addr"].(string)
				ip, err := netip.ParseAddr(addr)
				if err != nil {
					return nil, fmt.Errorf("failed to parse pod node IP %q: %w", addr, err)
				}

				ips = append(ips, ip)
				logger.Printf("podNodeIP[%d]=%s", i, ip.String())
			}
		}
	}

	return ips, nil
}

func (p *openstackProvider) GetNewtork(networkId string) (*string, error) {
	opts := networks.ListOpts{ID: networkId}
	pages, err := networks.List(p.networkClient, opts).AllPages()
	if err != nil {
		return nil, err
	}
	allNetworks, err := networks.ExtractNetworks(pages)
	if err != nil {
		return nil, err
	}
	if len(allNetworks) != 1 {
		return nil, fmt.Errorf("more than one network exists with the same ID")
	}
	return &allNetworks[0].Name, nil
}
