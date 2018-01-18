/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

const (
	sampleAPI1Name      = "Foo"
	sampleAPI1PolicyRef = "foo"

	sampleAPI2Name      = "Bar"
	sampleAPI2PolicyRef = "/Channel/foo"
)

var sampleAPIsGroup = &cb.ConfigGroup{
	Values: map[string]*cb.ConfigValue{
		sampleAPI1Name: {
			Value: utils.MarshalOrPanic(&pb.APIResource{
				PolicyRef: sampleAPI1PolicyRef,
			}),
		},
		sampleAPI2Name: {
			Value: utils.MarshalOrPanic(&pb.APIResource{
				PolicyRef: sampleAPI2PolicyRef,
			}),
		},
	},
}

func TestGreenAPIsPath(t *testing.T) {
	ag, err := newAPIsGroup(sampleAPIsGroup)
	assert.NotNil(t, ag)
	assert.NoError(t, err)

	t.Run("PresentAPIs", func(t *testing.T) {
		assert.Equal(t, "/Resources/APIs/"+sampleAPI1PolicyRef, ag.PolicyRefForAPI(sampleAPI1Name))
		assert.Equal(t, sampleAPI2PolicyRef, ag.PolicyRefForAPI(sampleAPI2Name))
	})

	t.Run("MissingAPIs", func(t *testing.T) {
		assert.Empty(t, ag.PolicyRefForAPI("missing"))
	})
}

func TestBadSubgroupsAPIsGroup(t *testing.T) {
	ccg, err := newAPIsGroup(&cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			"subGroup": {},
		},
	})

	assert.Nil(t, ccg)
	assert.Error(t, err)
	assert.Regexp(t, "apis group does not support sub-groups", err.Error())
}
