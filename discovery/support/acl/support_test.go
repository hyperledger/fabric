/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package acl_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/discovery/support/acl"
	"github.com/hyperledger/fabric/discovery/support/mocks"
	gmocks "github.com/hyperledger/fabric/peer/gossip/mocks"
	cb "github.com/hyperledger/fabric/protos/common"
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

func TestConfigSequenceEmptyChannelName(t *testing.T) {
	// If the channel name is empty, there is no config sequence,
	// and we return 0
	sup := acl.NewDiscoverySupport(nil, nil, nil)
	assert.Equal(t, uint64(0), sup.ConfigSequence(""))
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

			sup := acl.NewDiscoverySupport(&mocks.Verifier{}, &mocks.Evaluator{}, chConfig)
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
	e := &mocks.Evaluator{}
	v.VerifyByChannelReturnsOnCall(0, errors.New("verification failed"))
	v.VerifyByChannelReturnsOnCall(1, nil)
	e.EvaluateReturnsOnCall(0, errors.New("verification failed for local msp"))
	e.EvaluateReturnsOnCall(1, nil)
	chConfig := &mocks.ChanConfig{}
	sup := acl.NewDiscoverySupport(v, e, chConfig)
	err := sup.EligibleForService("mychannel", cb.SignedData{})
	assert.Equal(t, "verification failed", err.Error())
	err = sup.EligibleForService("mychannel", cb.SignedData{})
	assert.NoError(t, err)
	err = sup.EligibleForService("", cb.SignedData{})
	assert.Equal(t, "verification failed for local msp", err.Error())
	err = sup.EligibleForService("", cb.SignedData{})
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

	sup := acl.NewDiscoverySupport(&mocks.Verifier{}, &mocks.Evaluator{}, chConfig)
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

func TestChannelVerifier(t *testing.T) {
	polMgr := &gmocks.ChannelPolicyManagerGetterWithManager{
		Managers: map[string]policies.Manager{
			"mychannel": &gmocks.ChannelPolicyManager{
				Policy: &gmocks.Policy{
					Deserializer: &gmocks.IdentityDeserializer{
						Identity: []byte("Bob"), Msg: []byte("msg"),
					},
				},
			},
		},
	}

	verifier := &acl.ChannelVerifier{
		Policy: "some policy string",
		ChannelPolicyManagerGetter: polMgr,
	}

	t.Run("Valid channel, identity, signature", func(t *testing.T) {
		err := verifier.VerifyByChannel("mychannel", &cb.SignedData{
			Data:      []byte("msg"),
			Identity:  []byte("Bob"),
			Signature: []byte("msg"),
		})
		assert.NoError(t, err)
	})

	t.Run("Invalid channel", func(t *testing.T) {
		err := verifier.VerifyByChannel("notmychannel", &cb.SignedData{
			Data:      []byte("msg"),
			Identity:  []byte("Bob"),
			Signature: []byte("msg"),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "policy manager for channel notmychannel doesn't exist")
	})

	t.Run("Writers policy cannot be retrieved", func(t *testing.T) {
		polMgr.Managers["mychannel"].(*gmocks.ChannelPolicyManager).Policy = nil
		err := verifier.VerifyByChannel("mychannel", &cb.SignedData{
			Data:      []byte("msg"),
			Identity:  []byte("Bob"),
			Signature: []byte("msg"),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed obtaining channel application writers policy")
	})

}
