/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package car

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	"github.com/stretchr/testify/assert"
)

func TestPlatform(t *testing.T) {
	p := &Platform{}
	assert.Equal(t, "CAR", p.Name())
	assert.Nil(t, p.ValidatePath(""))
	assert.Nil(t, p.ValidateCodePackage([]byte{}))
	payload, err := p.GetDeploymentPayload("")
	assert.Nil(t, payload)
	assert.NoError(t, err)
	df, err := p.GenerateDockerfile()
	assert.Empty(t, df)
	assert.EqualError(t, err, errMsg)
	opts, err := p.DockerBuildOptions("ignored-path")
	assert.EqualError(t, err, "CAR packages are no longer supported.  You must upgrade your chaincode and use a supported type.")
	assert.Equal(t, opts, util.DockerBuildOptions{})
}
