//go:build byom

// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"testing"

	_ "github.com/confidential-containers/cloud-api-adaptor/src/cloud-api-adaptor/test/provisioner/byom"
)

func TestByomCreateSimplePod(t *testing.T) {
	assert := ByomAssert{}
	DoTestCreateSimplePod(t, testEnv, assert)
}

func TestByomDeleteSimplePod(t *testing.T) {
	assert := ByomAssert{}
	DoTestDeleteSimplePod(t, testEnv, assert)
}

func TestByomCreatePodWithConfigMap(t *testing.T) {
	assert := ByomAssert{}
	DoTestCreatePodWithConfigMap(t, testEnv, assert)
}

func TestByomCreatePodWithSecret(t *testing.T) {
	assert := ByomAssert{}
	DoTestCreatePodWithSecret(t, testEnv, assert)
}
