//go:build ibmcloud_powervs

// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package provisioner

import (
	"errors"
	"os"
	"strings"
)

type IBMCloudPowerVSProperties struct {
	CloudProvider     string
	APIKey            string
	ServiceInstanceID string
	Zone              string
	NetworkID         string
	ImageID           string
	SshKeyID          string
	ClusterName       string
	ResourceGroupID   string
}

var IBMCloudPowerVSProps = &IBMCloudPowerVSProperties{}

func initProperties(properties map[string]string) error {
	IBMCloudPowerVSProps = &IBMCloudPowerVSProperties{
		CloudProvider:     properties["CLOUD_PROVIDER"],
		APIKey:            properties["APIKEY"],
		ServiceInstanceID: properties["SERVICE_INSTANCE_ID"],
		Zone:              properties["ZONE"],
		NetworkID:         properties["NETWORK_ID"],
		ImageID:           properties["IMAGE_ID"],
		SshKeyID:          properties["SSH_KEY_ID"],
		ClusterName:       properties["CLUSTER_NAME"],
		ResourceGroupID:   properties["RESOURCE_GROUP_ID"],
	}

	if len(IBMCloudPowerVSProps.CloudProvider) <= 0 {
		IBMCloudPowerVSProps.CloudProvider = "ibmcloud-powervs"
	}

	if len(IBMCloudPowerVSProps.APIKey) <= 0 {
		return errors.New("APIKEY was not set.")
	}

	needProvisionStr := os.Getenv("TEST_PROVISION")
	if strings.EqualFold(needProvisionStr, "yes") || strings.EqualFold(needProvisionStr, "true") {
		// currently tying with a provisioned cluster
	} else {
		if len(IBMCloudPowerVSProps.ClusterName) <= 0 {
			return errors.New("CLUSTER_NAME was not set.")
		}

		if len(IBMCloudPowerVSProps.ResourceGroupID) <= 0 {
			return errors.New("RESOURCE_GROUP_ID was not set.")
		}

		if len(IBMCloudPowerVSProps.Zone) <= 0 {
			return errors.New("ZONE was not set.")
		}

		if len(IBMCloudPowerVSProps.ServiceInstanceID) <= 0 {
			return errors.New("SERVICE_INSTANCE_ID was not set.")
		}

		if len(IBMCloudPowerVSProps.SshKeyID) <= 0 {
			return errors.New("SSH_KEY_ID was not set.")
		}

		if len(IBMCloudPowerVSProps.NetworkID) <= 0 {
			return errors.New("NETWORK_ID was not set.")
		}

		podvmImage := os.Getenv("TEST_PODVM_IMAGE")
		if strings.EqualFold(podvmImage, "yes") || strings.EqualFold(podvmImage, "true") {
			if len(IBMCloudPowerVSProps.ImageID) <= 0 {
				return errors.New("IMAGE_ID was not set.")
			}
		}
	}
	return nil
}
