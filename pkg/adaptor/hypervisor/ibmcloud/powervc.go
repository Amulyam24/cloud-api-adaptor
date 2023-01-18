// (C) Copyright IBM Corp. 2022.
// SPDX-License-Identifier: Apache-2.0
//go:build ibmcloud

package ibmcloud

import (
	"fmt"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/utils/openstack/clientconfig"
)

type Service struct {
	computeClient *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient
}

const (
	computeServie = "compute"
	nwService     = "network"
)

type PowerVC interface {
	CreateVM(opts servers.CreateOpts) (*servers.Server, error)
	DeleteVM(instanceId string) error
	GetVM(instanceId string) (*servers.Server, error)
	GetNewtork(networkId string) (*string, error)
}

func (s *Service) CreateVM(opts servers.CreateOpts) (*servers.Server, error) {
	server, err := servers.Create(s.computeClient, opts).Extract()
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (s *Service) DeleteVM(instanceId string) error {
	err := servers.Delete(s.computeClient, instanceId).ExtractErr()
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) GetVM(instanceId string) (*servers.Server, error) {
	server, err := servers.Get(s.computeClient, instanceId).Extract()
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (s *Service) GetNewtork(networkId string) (*string, error) {
	opts := networks.ListOpts{ID: networkId}
	pages, err := networks.List(s.networkClient, opts).AllPages()
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

func NewPowerVCService() (PowerVC, error) {
	options := &clientconfig.ClientOpts{}
	cicClient, err := clientconfig.NewServiceClient(computeServie, options)
	if err != nil {
		return nil, err
	}
	nwClient, err := clientconfig.NewServiceClient(nwService, options)
	if err != nil {
		return nil, err
	}
	return &Service{
		computeClient: cicClient,
		networkClient: nwClient,
	}, nil
}
