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

package utils

import (
	"fmt"

	pb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

// no need to break out block header as none of its parts are serialized

// no need to break out block metadata as it's just a byte slice

// no need to break Block into constituents. Nothing to unmarshall

// BreakOutBlockDataOrPanic executes BreakOutBlockData() but panics on error
func BreakOutBlockDataOrPanic(blockData *pb.BlockData) ([]*pb.Payload, [][]byte) {
	payloads, envelopeSignatures, err := BreakOutBlockData(blockData)
	if err != nil {
		panic(err)
	}
	return payloads, envelopeSignatures
} // BreakOutBlockDataOrPanic

// BreakOutBlockData decomposes a blockData into its payloads and signatures.
// since a BlockData contains a slice of Envelopes, this functions returns slices of Payloads and Envelope.Signatures.
// the Payload/Signature pair has the same array index
func BreakOutBlockData(blockData *pb.BlockData) ([]*pb.Payload, [][]byte, error) {
	var err error

	var envelopeSignatures [][]byte
	var payloads []*pb.Payload

	var envelope *pb.Envelope
	var payload *pb.Payload
	for _, envelopeBytes := range blockData.Data {
		envelope = &pb.Envelope{}
		err = proto.Unmarshal(envelopeBytes, envelope)
		if err != nil {
			return nil, nil, err
		}
		payload = &pb.Payload{}
		err = proto.Unmarshal(envelope.Payload, payload)
		if err != nil {
			return nil, nil, err
		}
		envelopeSignatures = append(envelopeSignatures, envelope.Signature)
		payloads = append(payloads, payload)
	}

	return payloads, envelopeSignatures, nil
} // BreakOutBlockData

// BreakOutPayloadDataToConfigurationEnvelopeOrPanic calls BreakOutPayloadDataToConfigurationEnvelope() but panics on error
func BreakOutPayloadDataToConfigurationEnvelopeOrPanic(payloadData []byte) *pb.ConfigurationEnvelope {
	configEnvelope, err := BreakOutPayloadDataToConfigurationEnvelope(payloadData)
	if err != nil {
		panic(err)
	}
	return configEnvelope
} // BreakOutPayloadDataToConfigurationEnvelopeOrPanic

// BreakOutPayloadDataToConfigurationEnvelope decomposes a Payload.Data item into its constituent ConfigurationEnvelope
func BreakOutPayloadDataToConfigurationEnvelope(payloadData []byte) (*pb.ConfigurationEnvelope, error) {
	if payloadData == nil {
		return nil, fmt.Errorf("input Payload data is null\n")
	}

	configEnvelope := &pb.ConfigurationEnvelope{}
	err := proto.Unmarshal(payloadData, configEnvelope)
	if err != nil {
		return nil, err
	}

	return configEnvelope, nil
} //BreakOutPayloadToConfigurationEnvelope

// BreakOutConfigEnvelopeToConfigItemsOrPanic calls BreakOutConfigEnvelopeToConfigItems() but panics on error
func BreakOutConfigEnvelopeToConfigItemsOrPanic(configEnvelope *pb.ConfigurationEnvelope) ([]*pb.ConfigurationItem, [][]*pb.ConfigurationSignature) {
	configItems, configSignatures, err := BreakOutConfigEnvelopeToConfigItems(configEnvelope)
	if err != nil {
		panic(err)
	}
	return configItems, configSignatures
} // BreakOutConfigEnvelopeToConfigItemsOrPanic

// BreakOutConfigEnvelopeToConfigItems decomposes a ConfigurationEnvelope to its constituent ConfigurationItems and ConfigurationSignatures
// Note that a ConfigurationItem can have multiple signatures so each element in the returned ConfigurationItems slice is associated with a slice of ConfigurationSignatures
func BreakOutConfigEnvelopeToConfigItems(configEnvelope *pb.ConfigurationEnvelope) ([]*pb.ConfigurationItem, [][]*pb.ConfigurationSignature, error) {
	if configEnvelope == nil {
		return nil, nil, fmt.Errorf("BreakOutConfigEnvelopeToConfigItems received null input\n")
	}

	var configItems []*pb.ConfigurationItem
	var configSignatures [][]*pb.ConfigurationSignature

	var err error
	var configItem *pb.ConfigurationItem
	for i, signedConfigItem := range configEnvelope.Items {
		configItem = &pb.ConfigurationItem{}
		err = proto.Unmarshal(signedConfigItem.ConfigurationItem, configItem)
		if err != nil {
			return nil, nil, fmt.Errorf("BreakOutConfigEnvelopToConfigItems cannot unmarshall signedConfigurationItem: %v\n", err)
		}
		configItems = append(configItems, configItem)
		for _, signedConfigItemSignature := range signedConfigItem.Signatures {
			configSignatures[i] = append(configSignatures[i], signedConfigItemSignature)
		}
	}

	return configItems, configSignatures, nil
} // BreakOutConfigEnvelopeToConfigItems

// BreakOutBlockToConfigurationEnvelopeOrPanic calls BreakOutBlockToConfigurationEnvelope() but panics on error
func BreakOutBlockToConfigurationEnvelopeOrPanic(block *pb.Block) (*pb.ConfigurationEnvelope, []byte) {
	configEnvelope, envelopeSignature, err := BreakOutBlockToConfigurationEnvelope(block)
	if err != nil {
		panic(err)
	}
	return configEnvelope, envelopeSignature
} // BreakOutBlockToConfigurationEnvelopeOrPanic

// BreakOutBlockToConfigurationEnvelope decomposes a configuration transaction Block to its ConfigurationEnvelope
func BreakOutBlockToConfigurationEnvelope(block *pb.Block) (*pb.ConfigurationEnvelope, []byte, error) {
	if block == nil || block.Data == nil || len(block.Data.Data) > 1 {
		return nil, nil, fmt.Errorf("Block.BlockData is not an array of 1. This is not a configuration transaction\n")
	}

	payloads, envelopeSignatures, err := BreakOutBlockData(block.Data)

	if payloads[0].Header.ChainHeader.Type != int32(pb.HeaderType_CONFIGURATION_TRANSACTION) {
		return nil, nil, fmt.Errorf("Payload Header type is not configuration_transaction. This is not a configuration transaction\n")
	}
	var configEnvelope *pb.ConfigurationEnvelope
	configEnvelope, err = BreakOutPayloadDataToConfigurationEnvelope(payloads[0].Data)
	if err != nil {
		return nil, nil, fmt.Errorf("Error breaking out configurationEnvelope: %v\n", err)
	}

	return configEnvelope, envelopeSignatures[0], nil
} // BreakOutPayloadToConfigurationEnvelope
