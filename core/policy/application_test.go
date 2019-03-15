/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policy

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

//go:generate mockery -dir ../../msp/ -name IdentityDeserializer -case underscore -output mocks/
//go:generate mockery -dir ../../msp/ -name Identity -case underscore -output mocks/

func TestComponentIntegrationSignaturePolicyEnv(t *testing.T) {
	idds := &mocks.IdentityDeserializer{}
	id := &mocks.Identity{}

	spp := &cauthdsl.ProviderFromStruct{
		Deserializer: idds,
	}
	ev := &ApplicationPolicyEvaluator{
		signaturePolicyProvider: spp,
	}

	spenv := cauthdsl.SignedByMspMember("msp")
	mspenv := protoutil.MarshalOrPanic(&peer.ApplicationPolicy{
		Type: &peer.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: spenv,
		},
	})

	idds.On("DeserializeIdentity", []byte("guess who")).Return(id, nil)
	id.On("GetIdentifier").Return(&msp.IdentityIdentifier{Id: "id", Mspid: "msp"})
	id.On("SatisfiesPrincipal", mock.Anything).Return(nil)
	id.On("Verify", []byte("batti"), []byte("lei")).Return(nil)
	err := ev.Evaluate(mspenv, []*protoutil.SignedData{{
		Identity:  []byte("guess who"),
		Data:      []byte("batti"),
		Signature: []byte("lei"),
	}})
	assert.NoError(t, err)
}

func TestEvaluator(t *testing.T) {
	okEval := &mocks.Policy{}
	nokEval := &mocks.Policy{}

	okEval.On("Evaluate", mock.Anything).Return(nil)
	nokEval.On("Evaluate", mock.Anything).Return(errors.New("bad bad"))

	spp := &mocks.SignaturePolicyProvider{}
	cpp := &mocks.ChannelPolicyReferenceProvider{}
	ev := &ApplicationPolicyEvaluator{
		signaturePolicyProvider:        spp,
		channelPolicyReferenceProvider: cpp,
	}

	// SCENARIO: bad policy argument

	err := ev.Evaluate([]byte("bad bad"), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal ApplicationPolicy bytes")

	// SCENARIO: bad policy type

	err = ev.Evaluate([]byte{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported policy type")

	// SCENARIO: signature policy supplied - good and bad path

	spenv := &common.SignaturePolicyEnvelope{}
	mspenv := protoutil.MarshalOrPanic(&peer.ApplicationPolicy{
		Type: &peer.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: spenv,
		},
	})
	spp.On("NewPolicy", spenv).Return(okEval, nil).Once()
	err = ev.Evaluate(mspenv, nil)
	assert.NoError(t, err)
	spp.On("NewPolicy", spenv).Return(nokEval, nil).Once()
	err = ev.Evaluate(mspenv, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bad bad")
	spp.On("NewPolicy", spenv).Return(nil, errors.New("bad policy")).Once()
	err = ev.Evaluate(mspenv, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bad policy")

	// SCENARIO: channel ref policy supplied - good and bad path

	chrefstr := "Quo usque tandem abutere, Catilina, patientia nostra?"
	chrefstrEnv := protoutil.MarshalOrPanic(&peer.ApplicationPolicy{
		Type: &peer.ApplicationPolicy_ChannelConfigPolicyReference{
			ChannelConfigPolicyReference: chrefstr,
		},
	})
	cpp.On("NewPolicy", chrefstr).Return(okEval, nil).Once()
	err = ev.Evaluate(chrefstrEnv, nil)
	assert.NoError(t, err)
	cpp.On("NewPolicy", chrefstr).Return(nokEval, nil).Once()
	err = ev.Evaluate(chrefstrEnv, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bad bad")
	cpp.On("NewPolicy", chrefstr).Return(nil, errors.New("bad policy")).Once()
	err = ev.Evaluate(chrefstrEnv, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bad policy")
}

func TestChannelPolicyReference(t *testing.T) {
	mcpmg := &mocks.ChannelPolicyManagerGetter{}
	mcpmg.On("Manager", "channel").Return(nil, false).Once()
	ape, err := New(nil, "channel", mcpmg)
	assert.Error(t, err)
	assert.Nil(t, ape)
	assert.Contains(t, err.Error(), "failed to retrieve policy manager for channel")

	mm := &mocks.Manager{}
	mcpmg.On("Manager", "channel").Return(mm, true).Once()
	ape, err = New(nil, "channel", mcpmg)
	assert.NoError(t, err)
	assert.NotNil(t, ape)

	mcpmg.On("Manager", "channel").Return(mm, true)

	mp := &mocks.Policy{}
	mp.On("Evaluate", mock.Anything).Return(nil)
	mm.On("GetPolicy", "As the sun breaks above the ground").Return(mp, true)
	err = ape.evaluateChannelConfigPolicyReference("As the sun breaks above the ground", nil)
	assert.NoError(t, err)

	mm.On("GetPolicy", "An old man stands on the hill").Return(nil, false)
	err = ape.evaluateChannelConfigPolicyReference("An old man stands on the hill", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to retrieve policy for reference")
}
