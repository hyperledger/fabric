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
	"bytes"

	"github.com/hyperledger/fabric/common/chainconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/orderer/common/filter"
	"github.com/hyperledger/fabric/orderer/common/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

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
	SharedConfig() sharedconfig.Manager
	ChainConfig() chainconfig.Descriptor
	Enqueue(env *cb.Envelope) bool
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

	if msgData.Header == nil || msgData.Header.ChainHeader == nil || msgData.Header.ChainHeader.Type != int32(cb.HeaderType_ORDERER_TRANSACTION) {
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

func (sc *systemChain) proposeChain(configTx *cb.Envelope) cb.Status {
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
			ChainHeader: &cb.ChainHeader{
				ChainID: sc.support.ChainID(),
				Type:    int32(cb.HeaderType_ORDERER_TRANSACTION),
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

func (sc *systemChain) authorize(configEnvelope *cb.ConfigurationEnvelope) cb.Status {
	if len(configEnvelope.Items) == 0 {
		return cb.Status_BAD_REQUEST
	}

	creationConfigItem := &cb.ConfigurationItem{}
	err := proto.Unmarshal(configEnvelope.Items[0].ConfigurationItem, creationConfigItem)
	if err != nil {
		logger.Debugf("Failing to validate chain creation because of unmarshaling error: %s", err)
		return cb.Status_BAD_REQUEST
	}

	if creationConfigItem.Key != configtx.CreationPolicyKey {
		logger.Debugf("Failing to validate chain creation because first configuration item was not the CreationPolicy")
		return cb.Status_BAD_REQUEST
	}

	creationPolicy := &ab.CreationPolicy{}
	err = proto.Unmarshal(creationConfigItem.Value, creationPolicy)
	if err != nil {
		logger.Debugf("Failing to validate chain creation because first configuration item could not unmarshal to a CreationPolicy: %s", err)
		return cb.Status_BAD_REQUEST
	}

	ok := false
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

	// XXX actually do policy signature validation
	_ = policy

	configHash := configtx.HashItems(configEnvelope.Items[1:], sc.support.ChainConfig().HashingAlgorithm())

	if !bytes.Equal(configHash, creationPolicy.Digest) {
		logger.Debugf("Validly signed chain creation did not contain correct digest for remaining configuration %x vs. %x", configHash, creationPolicy.Digest)
		return cb.Status_BAD_REQUEST
	}

	return cb.Status_SUCCESS
}

func (sc *systemChain) inspect(configResources *configResources) cb.Status {
	// XXX decide what it is that we will require to be the same in the new configuration, and what will be allowed to be different
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

	if payload.Header == nil || payload.Header.ChainHeader == nil || payload.Header.ChainHeader.Type != int32(cb.HeaderType_CONFIGURATION_TRANSACTION) {
		logger.Debugf("Rejecting chain proposal: Not a configuration transaction: %s", err)
		return cb.Status_BAD_REQUEST
	}

	configEnvelope := &cb.ConfigurationEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnvelope)
	if err != nil {
		logger.Debugf("Rejecting chain proposal: Error unmarshalling config envelope from payload: %s", err)
		return cb.Status_BAD_REQUEST
	}

	if len(configEnvelope.Items) == 0 {
		logger.Debugf("Failing to validate chain creation because configuration was empty")
		return cb.Status_BAD_REQUEST
	}

	// Make sure that the configuration was signed by the appropriate authorized entities
	status := sc.authorize(configEnvelope)
	if status != cb.Status_SUCCESS {
		return status
	}

	configResources, err := newConfigResources(configEnvelope)
	if err != nil {
		logger.Debugf("Failed to create config manager and handlers: %s", err)
		return cb.Status_BAD_REQUEST
	}

	// Make sure that the configuration does not modify any of the orderer
	return sc.inspect(configResources)
}
