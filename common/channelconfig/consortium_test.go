/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestConsortiumConfig(t *testing.T) {
	cc, err := NewConsortiumConfig(&cb.ConfigGroup{}, NewMSPConfigHandler(msp.MSPv1_0))
	assert.NoError(t, err)
	orgs := cc.Organizations()
	assert.Equal(t, 0, len(orgs))

	policy := cc.ChannelCreationPolicy()
	assert.EqualValues(t, cb.Policy_UNKNOWN, policy.Type, "Expected policy type to be UNKNOWN")
}
