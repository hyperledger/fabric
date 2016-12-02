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
	"bytes"
	"testing"
	"time"

	pb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
)

// added comment to force gerrit commit

func TestBreakOutBlockDataBadData(t *testing.T) {
	fakeBlockData := &pb.BlockData{}
	payloads, sigs, _ := BreakOutBlockData(fakeBlockData)
	if len(payloads) > 0 || len(sigs) > 0 {
		t.Errorf("TestBreakOutBlockData should not work with blank input.\n")
	} // TestBreakOutBlockDataBadData
} // TestBreakOutBlockDataBadData

func TestBreakOutBlockData(t *testing.T) {
	block := testBlock()
	payloads, _, _ := BreakOutBlockData(block.Data) // TODO: test for signature
	if len(payloads) != 1 {
		t.Errorf("TestBreakOutBlock did not unmarshall to array of 1 payloads\n")
	}
	if payloads[0].Header.ChainHeader.Version != 1 || payloads[0].Header.ChainHeader.Type != int32(pb.HeaderType_CONFIGURATION_TRANSACTION) || payloads[0].Header.ChainHeader.ChainID != "test" {
		t.Errorf("TestBreakOutBlockData payload header is %+v . Expected type is %v and Version == 1\n", payloads[0].Header.ChainHeader, int32(pb.HeaderType_CONFIGURATION_TRANSACTION))
	}
	if !bytes.Equal(payloads[0].Data, []byte("test")) {
		t.Errorf("TestBreakOutBlockData payload data is %s . Expected 'test'\n", payloads[0].Data)
	}

} // TesttBreakOutBlockData

func TestBreakOutPayloadDataToConfigurationEnvelopePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("TestBreakOutPayloadDataToConfigurationEnvelopePanic should have panicked")
		}
	}()
	_ = BreakOutPayloadDataToConfigurationEnvelopeOrPanic(nil)
} // TestBreakOutPayloadDataToConfigurationEnvelopePanic

func TestBreakOutPayloadDataToConfigurationEnvelopeBadData(t *testing.T) {
	_, err := BreakOutPayloadDataToConfigurationEnvelope(nil)
	if err == nil {
		t.Errorf("TestBreakOutPayloadDataToConfigurationEnvelopeBadData should have returned error on null input\n ")
	}
} // TestBreakOutPayloadDataToConfigurationEnvelopeBadData

func TestBreakOutPayloadDataToConfigurationEnvelope(t *testing.T) {
	payload := testPayloadConfigEnvelope()
	configEnvelope, _ := BreakOutPayloadDataToConfigurationEnvelope(payload.Data)
	if len(configEnvelope.Items) != 1 {
		t.Errorf("TestBreakOutPayloadDataToConfigurationEnvelope: configEnvelope.Items array should have 1 item")
	}
} // TestBreakOutPayloadDataToConfigurationEnvelope

func TestBreakOutConfigEnvelopeToConfigItemsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("TestBreakOutConfigEnvelopeToConfigItemsPanic should have panicked")
		}
	}()
	_, _ = BreakOutConfigEnvelopeToConfigItemsOrPanic(nil)
} // TestBreakOutConfigEnvelopeToConfigItemsPanic

func TestBreakOutConfigEnvelopeToConfigItemsBadData(t *testing.T) {
	_, _, err := BreakOutConfigEnvelopeToConfigItems(nil)
	if err == nil {
		t.Errorf("TestBreakOutConfigEnvelopeToConfigItemsBadData not handling nil input\n")
	}
} // TestBreakOutConfigEnvelopeToConfigItemsBadData

func TestBreakOutConfigEnvelopeToConfigItems(t *testing.T) {
	configEnv := testConfigurationEnvelope()
	configItems, _, _ := BreakOutConfigEnvelopeToConfigItems(configEnv) // TODO: test signatures
	if len(configItems) != 1 {
		t.Errorf("TestBreakOutPayloadDataToConfigurationEnvelope did not return array of 1 config item\n")
	}
	if configItems[0].Header.Type != int32(pb.HeaderType_CONFIGURATION_TRANSACTION) || configItems[0].Header.ChainID != "test" {
		t.Errorf("TestBreakOutConfigEnvelopeToConfigItems, configItem header does not match original %+v . Expected config_transaction and chainid 'test'\n", configItems[0].Header)
	}
	if configItems[0].Type != pb.ConfigurationItem_Orderer || configItems[0].Key != "abc" || !bytes.Equal(configItems[0].Value, []byte("test")) {
		t.Errorf("TestBreakOutConfigEnvelopeToConfigItems configItem type,Key,Value do not match original %+v\n. Expected orderer, 'abc', 'test'", configItems[0])
	}
} // TestBreakOutConfigEnvelopeToConfigItems

func TestBreakOutBlockToConfigurationEnvelopePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("TestBreakOutBlockToConfigurationEnvelopePanic should have panicked")
		}
	}()
	_, _ = BreakOutBlockToConfigurationEnvelopeOrPanic(nil)
} // TestBreakOutBlockToConfigurationEnvelopePanic

func TestBreakOutBlockToConfigurationEnvelopeBadData(t *testing.T) {
	_, _, err := BreakOutBlockToConfigurationEnvelope(nil)
	if err == nil {
		t.Errorf("TestBreakOutBlockToConfigurationEnvelopeBadData should have rejected null input\n")
	}
} // TestBreakOutBlockToConfigurationEnvelopeBadData

func TestBreakOutBlockToConfigurationEnvelope(t *testing.T) {
	block := testConfigurationBlock()
	configEnvelope, _, _ := BreakOutBlockToConfigurationEnvelope(block) // TODO: test envelope signature
	if len(configEnvelope.Items) != 1 {
		t.Errorf("TestBreakOutBlockToConfigurationEnvelope should have an array of 1 signedConfigurationItems\n")
	}
} // TestBreakOutBlockToConfigurationEnvelopeBadData

// Helper functions
func testChainHeader() *pb.ChainHeader {
	return &pb.ChainHeader{
		Type:    int32(pb.HeaderType_CONFIGURATION_TRANSACTION),
		Version: 1,
		Timestamp: &timestamp.Timestamp{
			Seconds: time.Now().Unix(),
			Nanos:   0,
		},
		ChainID: "test",
	}
}

func testPayloadHeader() *pb.Header {
	return &pb.Header{
		ChainHeader:     testChainHeader(),
		SignatureHeader: nil,
	}
}

func testPayload() *pb.Payload {
	return &pb.Payload{
		Header: testPayloadHeader(),
		Data:   []byte("test"),
	}
}

func testEnvelope() *pb.Envelope {
	// No need to set the signature
	payloadBytes, _ := proto.Marshal(testPayload())
	return &pb.Envelope{Payload: payloadBytes}
}

func testPayloadConfigEnvelope() *pb.Payload {
	data, _ := proto.Marshal(testConfigurationEnvelope())
	return &pb.Payload{
		Header: testPayloadHeader(),
		Data:   data,
	}
}

func testEnvelopePayloadConfigEnv() *pb.Envelope {
	payloadBytes, _ := proto.Marshal(testPayloadConfigEnvelope())
	return &pb.Envelope{Payload: payloadBytes}
} // testEnvelopePayloadConfigEnv

func testConfigurationBlock() *pb.Block {
	envelopeBytes, _ := proto.Marshal(testEnvelopePayloadConfigEnv())
	return &pb.Block{
		Data: &pb.BlockData{
			Data: [][]byte{envelopeBytes},
		},
	}
}

func testConfigurationEnvelope() *pb.ConfigurationEnvelope {
	chainHeader := testChainHeader()
	configItem := makeConfigurationItem(chainHeader, pb.ConfigurationItem_Orderer, 0, "defaultPolicyID", "abc", []byte("test"))
	signedConfigItem, _ := makeSignedConfigurationItem(configItem, nil)
	return makeConfigurationEnvelope(signedConfigItem)
} // testConfigurationEnvelope

func testBlock() *pb.Block {
	// No need to set the block's Header, or Metadata
	envelopeBytes, _ := proto.Marshal(testEnvelope())
	return &pb.Block{
		Data: &pb.BlockData{
			Data: [][]byte{envelopeBytes},
		},
	}
}

func makeConfigurationItem(ch *pb.ChainHeader, configItemType pb.ConfigurationItem_ConfigurationType, lastModified uint64, modPolicyID string, key string, value []byte) *pb.ConfigurationItem {
	return &pb.ConfigurationItem{
		Header:             ch,
		Type:               configItemType,
		LastModified:       lastModified,
		ModificationPolicy: modPolicyID,
		Key:                key,
		Value:              value,
	}
}

func makeSignedConfigurationItem(configItem *pb.ConfigurationItem, signatures []*pb.ConfigurationSignature) (*pb.SignedConfigurationItem, error) {
	configItemBytes, _ := proto.Marshal(configItem)
	return &pb.SignedConfigurationItem{
		ConfigurationItem: configItemBytes,
		Signatures:        signatures,
	}, nil
} // makeSignedConfigurationItem

func makeConfigurationEnvelope(items ...*pb.SignedConfigurationItem) *pb.ConfigurationEnvelope {
	return &pb.ConfigurationEnvelope{Items: items}
}
