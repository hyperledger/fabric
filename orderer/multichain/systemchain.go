/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package multichain

import (
	"github.com/hyperledger/fabric/common/configtx"
	configvaluesapi "github.com/hyperledger/fabric/common/configvalues"
	configvalueschannel "github.com/hyperledger/fabric/common/configvalues/channel"
	configtxorderer "github.com/hyperledger/fabric/common/configvalues/channel/orderer"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/filter"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

// Define some internal interfaces for easier mocking
type chainCreator interface {
	newChain(configTx *cb.Envelope)
	systemChain() *systemChain
}

type limitedSupport interface {
	ChainID() string
	PolicyManager() policies.Manager
	SharedConfig() configvaluesapi.Orderer
	ChannelConfig() configvalueschannel.ConfigReader
	Enqueue(env *cb.Envelope) bool
	NewSignatureHeader() (*cb.SignatureHeader, error)
	Sign([]byte) ([]byte, error)
}

type systemChainCommitter struct {
	cc       chainCreator
	configTx *cb.Envelope
}

func (scc *systemChainCommitter) Isolated() bool {
	return true
}

func (scc *systemChainCommitter) Commit() {
	scc.cc.newChain(scc.configTx)
}

type systemChainFilter struct {
	cc chainCreator
}

func newSystemChainFilter(cc chainCreator) filter.Rule {
	return &systemChainFilter{
		cc: cc,
	}
}

func (scf *systemChainFilter) Apply(env *cb.Envelope) (filter.Action, filter.Committer) {
	msgData := &cb.Payload{}

	err := proto.Unmarshal(env.Payload, msgData)
	if err != nil {
		return filter.Forward, nil
	}

	if msgData.Header == nil || msgData.Header.ChannelHeader == nil || msgData.Header.ChannelHeader.Type != int32(cb.HeaderType_ORDERER_TRANSACTION) {
		return filter.Forward, nil
	}

	configTx := &cb.Envelope{}
	err = proto.Unmarshal(msgData.Data, configTx)
	if err != nil {
		return filter.Reject, nil
	}

	status := scf.cc.systemChain().authorizeAndInspect(configTx)
	if status != cb.Status_SUCCESS {
		return filter.Reject, nil
	}

	return filter.Accept, &systemChainCommitter{
		cc:       scf.cc,
		configTx: configTx,
	}
}

type systemChain struct {
	support limitedSupport
}

func newSystemChain(support limitedSupport) *systemChain {
	return &systemChain{
		support: support,
	}
}

// proposeChain takes in an envelope of type CONFIG_UPDATE, generates the new CONFIG and passes it along to the orderer system channel
func (sc *systemChain) proposeChain(configUpdateTx *cb.Envelope) cb.Status {
	configUpdatePayload, err := utils.UnmarshalPayload(configUpdateTx.Payload)
	if err != nil {
		logger.Debugf("Failing to propose channel creation because of payload unmarshaling error: %s", err)
		return cb.Status_BAD_REQUEST
	}

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(configUpdatePayload.Data)
	if err != nil {
		logger.Debugf("Failing to propose channel creation because of config update envelope unmarshaling error: %s", err)
		return cb.Status_BAD_REQUEST
	}

	configUpdate, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		logger.Debugf("Failing to propose channel creation because of config update unmarshaling error: %s", err)
		return cb.Status_BAD_REQUEST
	}

	if configUpdate.Header == nil {
		logger.Debugf("Failing to propose channel creation because of config update had no header")
		return cb.Status_BAD_REQUEST
	}

	sigHeader, err := sc.support.NewSignatureHeader()
	if err != nil {
		logger.Errorf("Error generating signature header: %s", err)
		return cb.Status_INTERNAL_SERVER_ERROR
	}

	configTx := &cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: &cb.ChannelHeader{
					ChannelId: configUpdate.Header.ChannelId,
					Type:      int32(cb.HeaderType_CONFIG),
				},
				SignatureHeader: sigHeader,
			},
			Data: utils.MarshalOrPanic(&cb.ConfigEnvelope{
				Config: &cb.Config{
					Header: &cb.ChannelHeader{
						ChannelId: configUpdate.Header.ChannelId,
						Type:      int32(cb.HeaderType_CONFIG),
					},
					Channel: configUpdate.WriteSet,
				},
				LastUpdate: configUpdateTx,
			}),
		}),
	}

	configTx.Signature, err = sc.support.Sign(configTx.Payload)
	if err != nil {
		logger.Errorf("Error generating signature: %s", err)
		return cb.Status_INTERNAL_SERVER_ERROR
	}

	status := sc.authorizeAndInspect(configTx)
	if status != cb.Status_SUCCESS {
		return status
	}

	marshaledEnv, err := proto.Marshal(configTx)
	if err != nil {
		logger.Debugf("Rejecting chain proposal: Error marshaling config: %s", err)
		return cb.Status_INTERNAL_SERVER_ERROR
	}

	sysPayload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: &cb.ChannelHeader{
				ChannelId: sc.support.ChainID(),
				Type:      int32(cb.HeaderType_ORDERER_TRANSACTION),
			},
			SignatureHeader: &cb.SignatureHeader{
			// XXX Appropriately set the signing identity and nonce here
			},
		},
		Data: marshaledEnv,
	}

	marshaledPayload, err := proto.Marshal(sysPayload)
	if err != nil {
		logger.Debugf("Rejecting chain proposal: Error marshaling payload: %s", err)
		return cb.Status_INTERNAL_SERVER_ERROR
	}

	sysTran := &cb.Envelope{
		Payload: marshaledPayload,
		// XXX Add signature eventually
	}

	if !sc.support.Enqueue(sysTran) {
		return cb.Status_INTERNAL_SERVER_ERROR
	}

	return cb.Status_SUCCESS
}

func (sc *systemChain) authorize(configEnvelope *cb.ConfigEnvelope) cb.Status {
	// XXX as a temporary hack to get the protos in, we assume the write set contains the whole config

	if configEnvelope.LastUpdate == nil {
		logger.Debugf("Must include a config update")
		return cb.Status_BAD_REQUEST
	}

	configEnvEnvPayload, err := utils.UnmarshalPayload(configEnvelope.LastUpdate.Payload)
	if err != nil {
		logger.Debugf("Failing to validate chain creation because of payload unmarshaling error: %s", err)
		return cb.Status_BAD_REQUEST
	}

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(configEnvEnvPayload.Data)
	if err != nil {
		logger.Debugf("Failing to validate chain creation because of config update envelope unmarshaling error: %s", err)
		return cb.Status_BAD_REQUEST
	}

	config, err := configtx.UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		logger.Debugf("Failing to validate chain creation because of unmarshaling error: %s", err)
		return cb.Status_BAD_REQUEST
	}

	if config.WriteSet == nil {
		logger.Debugf("Failing to validate channel creation because WriteSet is nil")
		return cb.Status_BAD_REQUEST
	}

	ordererGroup, ok := config.WriteSet.Groups[configtxorderer.GroupKey]
	if !ok {
		logger.Debugf("Rejecting channel creation because it is missing orderer group")
		return cb.Status_BAD_REQUEST
	}

	creationConfigItem, ok := ordererGroup.Values[configtx.CreationPolicyKey]
	if !ok {
		logger.Debugf("Failing to validate chain creation because no creation policy included")
		return cb.Status_BAD_REQUEST
	}

	creationPolicy := &ab.CreationPolicy{}
	err = proto.Unmarshal(creationConfigItem.Value, creationPolicy)
	if err != nil {
		logger.Debugf("Failing to validate chain creation because first config item could not unmarshal to a CreationPolicy: %s", err)
		return cb.Status_BAD_REQUEST
	}

	ok = false
	for _, chainCreatorPolicy := range sc.support.SharedConfig().ChainCreationPolicyNames() {
		if chainCreatorPolicy == creationPolicy.Policy {
			ok = true
			break
		}
	}

	if !ok {
		logger.Debugf("Failed to validate chain creation because chain creation policy (%s) is not authorized for chain creation", creationPolicy.Policy)
		return cb.Status_FORBIDDEN
	}

	policy, ok := sc.support.PolicyManager().GetPolicy(creationPolicy.Policy)
	if !ok {
		logger.Debugf("Failed to get policy for chain creation despite it being listed as an authorized policy")
		return cb.Status_INTERNAL_SERVER_ERROR
	}

	signedData, err := configUpdateEnv.AsSignedData()
	if err != nil {
		logger.Debugf("Failed to validate chain creation because config envelope could not be converted to signed data: %s", err)
		return cb.Status_BAD_REQUEST
	}

	err = policy.Evaluate(signedData)
	if err != nil {
		logger.Debugf("Failed to validate chain creation, did not satisfy policy: %s", err)
		return cb.Status_FORBIDDEN
	}

	return cb.Status_SUCCESS
}

func (sc *systemChain) inspect(configResources *configResources) cb.Status {
	// XXX decide what it is that we will require to be the same in the new config, and what will be allowed to be different
	// Are all keys allowed? etc.

	return cb.Status_SUCCESS
}

func (sc *systemChain) authorizeAndInspect(configTx *cb.Envelope) cb.Status {
	payload := &cb.Payload{}
	err := proto.Unmarshal(configTx.Payload, payload)
	if err != nil {
		logger.Debugf("Rejecting chain proposal: Error unmarshaling envelope payload: %s", err)
		return cb.Status_BAD_REQUEST
	}

	if payload.Header == nil || payload.Header.ChannelHeader == nil || payload.Header.ChannelHeader.Type != int32(cb.HeaderType_CONFIG) {
		logger.Debugf("Rejecting chain proposal: Not a config transaction")
		return cb.Status_BAD_REQUEST
	}

	configEnvelope := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnvelope)
	if err != nil {
		logger.Debugf("Rejecting chain proposal: Error unmarshalling config envelope from payload: %s", err)
		return cb.Status_BAD_REQUEST
	}

	// Make sure that the config was signed by the appropriate authorized entities
	status := sc.authorize(configEnvelope)
	if status != cb.Status_SUCCESS {
		return status
	}

	configResources, err := newConfigResources(configEnvelope)
	if err != nil {
		logger.Debugf("Failed to create config manager and handlers: %s", err)
		return cb.Status_BAD_REQUEST
	}

	// Make sure that the config does not modify any of the orderer
	return sc.inspect(configResources)
}
