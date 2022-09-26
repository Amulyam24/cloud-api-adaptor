// (C) Copyright IBM Corp. 2022.
// SPDX-License-Identifier: Apache-2.0
//go:build ibmcloud

package ibmcloud

import (
	"log"

	"github.com/confidential-containers/cloud-api-adaptor/pkg/adaptor/proxy"
	"github.com/confidential-containers/cloud-api-adaptor/pkg/podnetwork/tunneler"
)

const Version = "0.0.0"

var logger = log.New(log.Writer(), "[helper/hypervisor] ", log.LstdFlags|log.Lmsgprefix)

type sandboxID string

type sandbox struct {
	id               sandboxID
	pod              string
	namespace        string
	netNSPath        string
	podDirPath       string
	vsi              string
	agentProxy       proxy.AgentProxy
	podNetworkConfig *tunneler.Config
}
