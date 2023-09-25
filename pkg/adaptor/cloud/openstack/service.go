// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/utils/openstack/clientconfig"
)

type openstackService struct {
	computeClient *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient
}

const (
	computeServie = "compute"
	nwService     = "network"
)

func NewOpenStackService() (*openstackService, error) {
	options := &clientconfig.ClientOpts{}
	cicClient, err := clientconfig.NewServiceClient(computeServie, options)
	if err != nil {
		return nil, err
	}
	nwClient, err := clientconfig.NewServiceClient(nwService, options)
	if err != nil {
		return nil, err
	}
	return &openstackService{
		computeClient: cicClient,
		networkClient: nwClient,
	}, nil
}
