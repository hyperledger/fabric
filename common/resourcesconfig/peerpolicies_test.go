/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

var (
	dummyPolicy = &cb.ConfigPolicy{
		Policy: &cb.Policy{
			Type:  int32(cb.Policy_SIGNATURE),
			Value: utils.MarshalOrPanic(cauthdsl.AcceptAllPolicy),
		},
	}

	samplePeerPoliciesGroup = &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			"foo": dummyPolicy,
			"bar": dummyPolicy,
		},
		Groups: map[string]*cb.ConfigGroup{
			"subGroup": {
				Policies: map[string]*cb.ConfigPolicy{
					"other": dummyPolicy,
				},
			},
		},
	}
)

func TestGreenPeerPoliciesPath(t *testing.T) {
	ppg, err := newPeerPoliciesGroup(samplePeerPoliciesGroup)
	assert.NoError(t, err)
	assert.NotNil(t, ppg)
}

func TestPeerPolicieesWithValues(t *testing.T) {
	_, err := newPeerPoliciesGroup(&cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			"bar": {
				Values: map[string]*cb.ConfigValue{
					"foo": {},
				},
			},
		},
	})

	assert.Error(t, err)
	assert.Regexp(t, "sub-groups not allowed to have values", err.Error())
}
