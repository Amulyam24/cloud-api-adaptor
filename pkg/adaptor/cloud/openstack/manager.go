// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"flag"

	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/cloud"
)

var openstackConfig Config

type Manager struct{}

func (_ *Manager) ParseCmd(flags *flag.FlagSet) {
	flags.StringVar(&openstackConfig.NetworkID, "network-id", "", "ID of the network instance")
	flags.StringVar(&openstackConfig.ImageID, "image-id", "", "ID of the boot image")
	flags.StringVar(&openstackConfig.SSHKey, "ssh-key", "", "Name of the SSH Key")
	flags.StringVar(&openstackConfig.FlavorID, "flavor-id", "", "ID of the VM flavor to be used")
	flags.StringVar(&openstackConfig.HostGroup, "host-group", "", "Name of the host group to be used")
}

func (_ *Manager) LoadEnv() {
}

func (_ *Manager) NewProvider() (cloud.Provider, error) {
	return NewProvider(&openstackConfig)
}
