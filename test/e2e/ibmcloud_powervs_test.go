//go:build ibmcloud_powervs

// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestCreateSimplePod(t *testing.T) {
	assert := IBMCloudPowerVSAssert{}
	doTestCreateSimplePod(t, assert)
}

func TestCreatePodWithConfigMap(t *testing.T) {
	assert := IBMCloudPowerVSAssert{}
	doTestCreatePodWithConfigMap(t, assert)
}

func TestCreatePodWithSecret(t *testing.T) {
	assert := IBMCloudPowerVSAssert{}
	doTestCreatePodWithSecret(t, assert)
}
func TestCreatePeerPodContainerWithExternalIPAccess(t *testing.T) {
	assert := IBMCloudPowerVSAssert{}
	doTestCreatePeerPodContainerWithExternalIPAccess(t, assert)
}

// IBMCloudPoweerVSAssert implements the CloudAssert interface for IBM Cloud PowerVS.
type IBMCloudPowerVSAssert struct {
}

func (c IBMCloudPowerVSAssert) HasPodVM(t *testing.T, id string) {
	log.Infof("PodVM name: %s", id)

}
