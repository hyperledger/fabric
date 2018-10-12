/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

// The tests in this file are all relatively pointless, as all of this function is exercised
// in the normal startup path and things will break horribly if they are broken.
// There's additionally really nothing to test without simply re-implementing the function
// in the test, which also provides no value.  But, not including these produces an artificially
// low code coverage count, so here they are.

func basicTest(t *testing.T, sv *StandardConfigValue) {
	assert.NotNil(t, sv)
	assert.NotEmpty(t, sv.Key())
	assert.NotNil(t, sv.Value())
}

func TestUtilsBasic(t *testing.T) {
	basicTest(t, ConsortiumValue("foo"))
	basicTest(t, HashingAlgorithmValue())
	basicTest(t, BlockDataHashingStructureValue())
	basicTest(t, OrdererAddressesValue([]string{"foo:1", "bar:2"}))
	basicTest(t, ConsensusTypeValue("foo", []byte("bar")))
	basicTest(t, BatchSizeValue(1, 2, 3))
	basicTest(t, BatchTimeoutValue("1s"))
	basicTest(t, ChannelRestrictionsValue(7))
	basicTest(t, KafkaBrokersValue([]string{"foo:1", "bar:2"}))
	basicTest(t, MSPValue(&mspprotos.MSPConfig{}))
	basicTest(t, CapabilitiesValue(map[string]bool{"foo": true, "bar": false}))
	basicTest(t, AnchorPeersValue([]*pb.AnchorPeer{{}, {}}))
	basicTest(t, ChannelCreationPolicyValue(&cb.Policy{}))
	basicTest(t, ACLValues(map[string]string{"foo": "fooval", "bar": "barval"}))
}
