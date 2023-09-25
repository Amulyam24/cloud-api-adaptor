// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package openstack

type Config struct {
	NetworkID string
	ImageID   string
	SSHKey    string
	FlavorID  string
	HostGroup string
}

func (c Config) Redact() Config {
	return Config{}
}
