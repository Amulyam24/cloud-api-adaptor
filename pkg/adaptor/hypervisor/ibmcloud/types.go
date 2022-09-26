// (C) Copyright IBM Corp. 2022.
// SPDX-License-Identifier: Apache-2.0
//go:build ibmcloud

package ibmcloud

import "github.com/confidential-containers/cloud-api-adaptor/pkg/util"

type VpcConfig struct {
	ApiKey                   string
	IamServiceURL            string
	VpcServiceURL            string
	ResourceGroupID          string
	ProfileName              string
	ZoneName                 string
	ImageID                  string
	PrimarySubnetID          string
	PrimarySecurityGroupID   string
	SecondarySubnetID        string
	SecondarySecurityGroupID string
	KeyID                    string
	VpcID                    string
}

type PowerVSConfig struct {
	ApiKey            string
	Zone              string
	ServiceInstanceID string
	NetworkID         string
	ImageID           string
	SSHKey            string
}

func (c PowerVSConfig) Redact() PowerVSConfig {
	return *util.RedactStruct(&c, "ApiKey").(*PowerVSConfig)
}

func (c VpcConfig) Redact() VpcConfig {
	return *util.RedactStruct(&c, "ApiKey").(*VpcConfig)
}
