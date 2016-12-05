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

package static

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/cauthdsl"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/util"
	cb "github.com/hyperledger/fabric/protos/common"
)

func TestGenesisBlockCreation(t *testing.T) {
	_, err := New().GenesisBlock()
	if err != nil {
		t.Fatalf("Cannot create genesis block: %s", err)
	}
}

func TestGenesisBlockHeader(t *testing.T) {
	expectedHeaderNumber := uint64(0)

	genesisBlock, _ := New().GenesisBlock() // The error has been checked in a previous test

	if genesisBlock.Header.Number != expectedHeaderNumber {
		t.Fatalf("Expected header number %d, got %d", expectedHeaderNumber, genesisBlock.Header.Number)
	}

	if !bytes.Equal(genesisBlock.Header.PreviousHash, nil) {
		t.Fatalf("Expected header previousHash to be nil, got %x", genesisBlock.Header.PreviousHash)
	}
}

func TestGenesisBlockData(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OrPanicked unexpectedly: %s", r)
		}
	}()

	expectedBlockDataLength := 1
	expectedPayloadChainHeaderType := int32(cb.HeaderType_CONFIGURATION_TRANSACTION)
	expectedChainHeaderVersion := msgVersion
	expectedChainHeaderEpoch := uint64(0)
	expectedConfigEnvelopeItemsLength := 3
	expectedConfigurationItemChainHeaderType := int32(cb.HeaderType_CONFIGURATION_ITEM)
	expectedConfigurationItemChainHeaderVersion := msgVersion
	expectedConfigurationItemType := cb.ConfigurationItem_Policy
	expectedConfigEnvelopeSequence := uint64(0)
	expectedConfigurationItemModificationPolicy := configtx.DefaultModificationPolicyID
	expectedConfigurationItemValueSignaturePolicyEnvelope := cauthdsl.RejectAllPolicy

	genesisBlock, _ := New().GenesisBlock() // The error has been checked in a previous test

	if len(genesisBlock.Data.Data) != expectedBlockDataLength {
		t.Fatalf("Expected genesis block data length %d, got %d", expectedBlockDataLength, len(genesisBlock.Data.Data))
	}

	envelope := util.ExtractEnvelopeOrPanic(genesisBlock, 0)

	envelopeSignature := envelope.Signature
	if !bytes.Equal(envelopeSignature, nil) {
		t.Fatalf("Expected envelope signature to be nil, got %x", envelopeSignature)
	}

	payload := util.ExtractPayloadOrPanic(envelope)

	signatureHeader := payload.Header.SignatureHeader
	if !bytes.Equal(signatureHeader.Creator, nil) {
		t.Fatalf("Expected payload signature header creator to be nil, got %x", signatureHeader.Creator)
	}
	if bytes.Equal(signatureHeader.Nonce, nil) {
		t.Fatal("Expected non-nil nonce")
	}

	payloadChainHeader := payload.Header.ChainHeader
	if payloadChainHeader.Type != expectedPayloadChainHeaderType {
		t.Fatalf("Expected payload chain header type %d, got %d", expectedPayloadChainHeaderType, payloadChainHeader.Type)
	}
	if payloadChainHeader.Version != expectedChainHeaderVersion {
		t.Fatalf("Expected payload chain header version %d, got %d", expectedChainHeaderVersion, payloadChainHeader.Version)
	}
	if payloadChainHeader.Epoch != expectedChainHeaderEpoch {
		t.Fatalf("Expected payload chain header header epoch to be %d, got %d", expectedChainHeaderEpoch, payloadChainHeader.Epoch)
	}

	marshaledConfigurationEnvelope := payload.Data
	configurationEnvelope := &cb.ConfigurationEnvelope{}
	if err := proto.Unmarshal(marshaledConfigurationEnvelope, configurationEnvelope); err != nil {
		t.Fatalf("Expected genesis block to carry a ConfigurationEnvelope")
	}
	if len(configurationEnvelope.Items) != expectedConfigEnvelopeItemsLength {
		t.Fatalf("Expected configuration envelope to have %d configuration item(s), got %d", expectedConfigEnvelopeItemsLength, len(configurationEnvelope.Items))
	}

	signedConfigurationItem := configurationEnvelope.Items[2]
	marshaledConfigurationItem := signedConfigurationItem.ConfigurationItem
	configurationItem := &cb.ConfigurationItem{}
	if err := proto.Unmarshal(marshaledConfigurationItem, configurationItem); err != nil {
		t.Fatalf("Expected genesis block to carry a ConfigurationItem")
	}

	configurationItemChainHeader := configurationItem.Header
	if configurationItemChainHeader.Type != expectedConfigurationItemChainHeaderType {
		t.Fatalf("Expected configuration item chain header type %d, got %d", expectedConfigurationItemChainHeaderType, configurationItemChainHeader.Type)
	}
	if configurationItemChainHeader.Version != expectedConfigurationItemChainHeaderVersion {
		t.Fatalf("Expected configuration item chain header version %d, got %d", expectedConfigurationItemChainHeaderVersion, configurationItemChainHeader.Version)
	}
	if configurationItemChainHeader.ChainID != payloadChainHeader.ChainID {
		t.Fatalf("Expected chain ID in chain headers of configuration item and payload to match, got %x and %x respectively", configurationItemChainHeader.ChainID, payloadChainHeader.ChainID)
	}
	if configurationItemChainHeader.Epoch != payloadChainHeader.Epoch {
		t.Fatalf("Expected epoch in chain headers of configuration item and payload to match, got %d, and %d respectively", configurationItemChainHeader.Epoch, payloadChainHeader.Epoch)
	}

	if configurationItem.Type != expectedConfigurationItemType {
		t.Fatalf("Expected configuration item type %s, got %s", expectedConfigurationItemType.String(), configurationItem.Type.String())
	}
	if configurationItem.LastModified != expectedConfigEnvelopeSequence {
		t.Fatalf("Expected configuration item sequence to match configuration envelope sequence %d, got %d", expectedConfigEnvelopeSequence, configurationItem.LastModified)
	}
	if configurationItem.ModificationPolicy != expectedConfigurationItemModificationPolicy {
		t.Fatalf("Expected configuration item modification policy %s, got %s", expectedConfigurationItemModificationPolicy, configurationItem.ModificationPolicy)
	}
	if configurationItem.Key != expectedConfigurationItemModificationPolicy {
		t.Fatalf("Expected configuration item key to be equal to the modification policy %s, got %s", expectedConfigurationItemModificationPolicy, configurationItem.Key)
	}

	marshaledPolicy := configurationItem.Value
	policy := &cb.Policy{}
	if err := proto.Unmarshal(marshaledPolicy, policy); err != nil {
		t.Fatalf("Expected genesis block to carry a policy in its configuration item value")
	}
	switch policy.GetType().(type) {
	case *cb.Policy_SignaturePolicy:
	default:
		t.Fatalf("Got unexpected configuration item value policy type")
	}
	signaturePolicyEnvelope := policy.GetSignaturePolicy()
	if !proto.Equal(signaturePolicyEnvelope, expectedConfigurationItemValueSignaturePolicyEnvelope) {
		t.Fatalf("Expected configuration item value signature policy envelope %s, got %s", expectedConfigurationItemValueSignaturePolicyEnvelope.String(), signaturePolicyEnvelope.String())
	}
}

func TestGenesisMetadata(t *testing.T) {
	genesisBlock, _ := New().GenesisBlock() // The error has been checked in a previous test

	if genesisBlock.Metadata != nil {
		t.Fatalf("Expected metadata nil, got %x", genesisBlock.Metadata)
	}
}
