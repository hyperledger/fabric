/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package car

import (
	"testing"

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
	assert.EqualError(t, p.GenerateDockerBuild("", nil, nil), errMsg)
	md, err := p.GetMetadataAsTarEntries([]byte{})
	assert.Nil(t, md)
	assert.NoError(t, err)
}
