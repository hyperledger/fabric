/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package acl_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/discovery/support/acl"
	"github.com/hyperledger/fabric/discovery/support/mocks"
	common2 "github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestGetChannelConfigFunc(t *testing.T) {
	r := &mocks.Resources{}
	f := func(cid string) channelconfig.Resources {
		return r
	}
	assert.Equal(t, r, acl.ChannelConfigGetterFunc(f).GetChannelConfig("mychannel"))
}

func TestConfigSequence(t *testing.T) {
	tests := []struct {
		name           string
		resourcesFound bool
		validatorFound bool
		sequence       uint64
		shouldPanic    bool
	}{
		{
			name:        "resources not found",
			shouldPanic: true,
		},
		{
			name:           "validator not found",
			resourcesFound: true,
			shouldPanic:    true,
		},
		{
			name:           "both resoruces and validator are found",
			resourcesFound: true,
			validatorFound: true,
			sequence:       100,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			chConfig := &mocks.ChanConfig{}
			r := &mocks.Resources{}
			v := &mocks.ConfigtxValidator{}
			if test.resourcesFound {
				chConfig.GetChannelConfigReturns(r)
			}
			if test.validatorFound {
				r.ConfigtxValidatorReturns(v)
			}
			v.SequenceReturns(test.sequence)

			sup := acl.NewDiscoverySupport(&mocks.Verifier{}, chConfig)
			if test.shouldPanic {
				assert.Panics(t, func() {
					sup.ConfigSequence("mychannel")
				})
				return
			}
			assert.Equal(t, test.sequence, sup.ConfigSequence("mychannel"))
		})
	}
}

func TestEligibleForService(t *testing.T) {
	v := &mocks.Verifier{}
	v.VerifyByChannelReturnsOnCall(0, errors.New("verification failed"))
	v.VerifyByChannelReturnsOnCall(1, nil)
	chConfig := &mocks.ChanConfig{}
	sup := acl.NewDiscoverySupport(v, chConfig)
	err := sup.EligibleForService("", common2.SignedData{})
	assert.Equal(t, "verification failed", err.Error())
	err = sup.EligibleForService("", common2.SignedData{})
	assert.NoError(t, err)
}

func TestSatisfiesPrincipal(t *testing.T) {
	var (
		chConfig                      = &mocks.ChanConfig{}
		resources                     = &mocks.Resources{}
		mgr                           = &mocks.MSPManager{}
		idThatDoesNotSatisfyPrincipal = &mocks.Identity{}
		idThatSatisfiesPrincipal      = &mocks.Identity{}
	)

	tests := []struct {
		testDescription string
		before          func()
		expectedErr     string
	}{
		{
			testDescription: "Channel does not exist",
			before: func() {
				chConfig.GetChannelConfigReturns(nil)
			},
			expectedErr: "channel mychannel doesn't exist",
		},
		{
			testDescription: "MSP manager not available",
			before: func() {
				chConfig.GetChannelConfigReturns(resources)
				resources.MSPManagerReturns(nil)
			},
			expectedErr: "could not find MSP manager for channel mychannel",
		},
		{
			testDescription: "Identity cannot be deserialized",
			before: func() {
				resources.MSPManagerReturns(mgr)
				mgr.DeserializeIdentityReturns(nil, errors.New("not a valid identity"))
			},
			expectedErr: "failed deserializing identity: not a valid identity",
		},
		{
			testDescription: "Identity does not satisfy principal",
			before: func() {
				idThatDoesNotSatisfyPrincipal.SatisfiesPrincipalReturns(errors.New("does not satisfy principal"))
				mgr.DeserializeIdentityReturns(idThatDoesNotSatisfyPrincipal, nil)
			},
			expectedErr: "does not satisfy principal",
		},
		{
			testDescription: "All is fine, identity is eligible",
			before: func() {
				idThatSatisfiesPrincipal.SatisfiesPrincipalReturns(nil)
				mgr.DeserializeIdentityReturns(idThatSatisfiesPrincipal, nil)
			},
			expectedErr: "",
		},
	}

	sup := acl.NewDiscoverySupport(&mocks.Verifier{}, chConfig)
	for _, test := range tests {
		test := test
		t.Run(test.testDescription, func(t *testing.T) {
			test.before()
			err := sup.SatisfiesPrincipal("mychannel", nil, nil)
			if test.expectedErr != "" {
				assert.Equal(t, test.expectedErr, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})

	}
}
