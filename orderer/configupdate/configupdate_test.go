/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package configupdate

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockcrypto "github.com/hyperledger/fabric/common/mocks/crypto"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

type mockSupport struct {
	ProposeConfigUpdateVal *cb.ConfigEnvelope
}

func (ms *mockSupport) ProposeConfigUpdate(env *cb.Envelope) (*cb.ConfigEnvelope, error) {
	var err error
	if ms.ProposeConfigUpdateVal == nil {
		err = fmt.Errorf("Nil result implies error in mock")
	}
	return ms.ProposeConfigUpdateVal, err
}

type mockSupportManager struct {
	GetChainVal *mockSupport
}

func (msm *mockSupportManager) GetChain(chainID string) (Support, bool) {
	return msm.GetChainVal, msm.GetChainVal != nil
}

func (msm *mockSupportManager) NewChannelConfig(env *cb.Envelope) (configtxapi.Manager, error) {
	return &mockconfigtx.Manager{
		ProposeConfigUpdateVal: &cb.ConfigEnvelope{
			LastUpdate: env,
		},
	}, nil
}

func TestChannelID(t *testing.T) {
	makeEnvelope := func(payload *cb.Payload) *cb.Envelope {
		return &cb.Envelope{
			Payload: utils.MarshalOrPanic(payload),
		}
	}

	testChannelID := "foo"

	result, err := channelID(makeEnvelope(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
				ChannelId: testChannelID,
			}),
		},
	}))
	assert.NoError(t, err, "Channel ID was present")
	assert.Equal(t, testChannelID, result, "Channel ID was present")

	_, err = channelID(makeEnvelope(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{}),
		},
	}))
	assert.Error(t, err, "Channel ID was empty")

	_, err = channelID(makeEnvelope(&cb.Payload{
		Header: &cb.Header{},
	}))
	assert.Error(t, err, "ChannelHeader was missing")

	_, err = channelID(makeEnvelope(&cb.Payload{}))
	assert.Error(t, err, "Header was missing")

	_, err = channelID(&cb.Envelope{})
	assert.Error(t, err, "Payload was missing")
}

const systemChannelID = "system_channel"
const testUpdateChannelID = "update_channel"

func newTestInstance() (*mockSupportManager, *Processor) {
	msm := &mockSupportManager{}
	msm.GetChainVal = &mockSupport{}
	return msm, New(systemChannelID, msm, mockcrypto.FakeLocalSigner)
}

func testConfigUpdate() *cb.Envelope {
	ch := &cb.ChannelHeader{
		ChannelId: testUpdateChannelID,
	}

	return &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(ch),
			},
			Data: utils.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
				ConfigUpdate: utils.MarshalOrPanic(&cb.ConfigUpdate{
					ChannelId: ch.ChannelId,
					WriteSet:  cb.NewConfigGroup(),
				}),
			}),
		}),
	}
}

func TestExistingChannel(t *testing.T) {
	msm, p := newTestInstance()

	testUpdate := testConfigUpdate()

	dummyResult := &cb.ConfigEnvelope{LastUpdate: &cb.Envelope{Payload: []byte("DUMMY")}}

	msm.GetChainVal = &mockSupport{ProposeConfigUpdateVal: dummyResult}
	env, err := p.Process(testUpdate)
	assert.NoError(t, err, "Valid config update")
	_ = utils.UnmarshalPayloadOrPanic(env.Payload)
	assert.Equal(t, dummyResult, configtx.UnmarshalConfigEnvelopeOrPanic(utils.UnmarshalPayloadOrPanic(env.Payload).Data), "Valid config update")

	msm.GetChainVal = &mockSupport{}
	_, err = p.Process(testUpdate)
	assert.Error(t, err, "Invald ProposeUpdate result")
}

func TestNewChannel(t *testing.T) {
	msm, p := newTestInstance()
	msm.GetChainVal = nil

	testUpdate := testConfigUpdate()

	env, err := p.Process(testUpdate)
	assert.NoError(t, err, "Valid config update")

	resultChan, err := channelID(env)
	assert.NoError(t, err, "Invalid envelope produced")

	assert.Equal(t, systemChannelID, resultChan, "Wrapper TX should be bound for system channel")

	chdr, err := utils.UnmarshalChannelHeader(utils.UnmarshalPayloadOrPanic(env.Payload).Header.ChannelHeader)
	assert.NoError(t, err, "UnmarshalChannelHeader error")

	assert.Equal(t, int32(cb.HeaderType_ORDERER_TRANSACTION), chdr.Type, "Wrong wrapper tx type")
}
