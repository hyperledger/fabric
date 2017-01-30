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

// BreakOutBlockToConfigurationEnvelope decomposes a configuration transaction Block to its ConfigurationEnvelope
func BreakOutBlockToConfigurationEnvelope(block *pb.Block) (*pb.ConfigurationEnvelope, error) {
	if block == nil || block.Data == nil || len(block.Data.Data) > 1 {
		return nil, fmt.Errorf("Block.BlockData is not an array of 1. This is not a configuration transaction\n")
	}

	payloads, _, _ := BreakOutBlockData(block.Data)

	if payloads[0].Header.ChainHeader.Type != int32(pb.HeaderType_CONFIGURATION_TRANSACTION) {
		return nil, fmt.Errorf("Payload Header type is not configuration_transaction. This is not a configuration transaction\n")
	}
	configEnvelope, err := BreakOutPayloadDataToConfigurationEnvelope(payloads[0].Data)
	if err != nil {
		return nil, fmt.Errorf("Error breaking out configurationEnvelope: %v\n", err)
	}

	return configEnvelope, nil
} // BreakOutPayloadToConfigurationEnvelope

// UnmarshalConfigurationItemOrPanic unmarshals bytes to a ConfigurationItem or panics on error
func UnmarshalConfigurationItemOrPanic(encoded []byte) *pb.ConfigurationItem {
	configItem, err := UnmarshalConfigurationItem(encoded)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling data to ConfigurationItem: %s", err))
	}
	return configItem
}

// UnmarshalConfigurationItem unmarshals bytes to a ConfigurationItem
func UnmarshalConfigurationItem(encoded []byte) (*pb.ConfigurationItem, error) {
	configItem := &pb.ConfigurationItem{}
	err := proto.Unmarshal(encoded, configItem)
	if err != nil {
		return nil, err
	}
	return configItem, nil
}

// UnmarshalConfigurationEnvelopeOrPanic unmarshals bytes to a ConfigurationEnvelope or panics on error
func UnmarshalConfigurationEnvelopeOrPanic(encoded []byte) *pb.ConfigurationEnvelope {
	configEnvelope, err := UnmarshalConfigurationEnvelope(encoded)
	if err != nil {
		panic(fmt.Errorf("Error unmarshaling data to ConfigurationEnvelope: %s", err))
	}
	return configEnvelope
}

// UnmarshalConfigurationEnvelope unmarshals bytes to a ConfigurationEnvelope
func UnmarshalConfigurationEnvelope(encoded []byte) (*pb.ConfigurationEnvelope, error) {
	configEnvelope := &pb.ConfigurationEnvelope{}
	err := proto.Unmarshal(encoded, configEnvelope)
	if err != nil {
		return nil, err
	}
	return configEnvelope, nil
}
