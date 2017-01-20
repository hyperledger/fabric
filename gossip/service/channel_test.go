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

package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protos/utils"
)

type blockInvalidator func(*common.Block) *common.Block


// unMarshaller is a mock for proto.Unmarshal
// that fails on an Nth attempt
type unMarshaller struct {
	attemptsLeft int
}

func (um *unMarshaller) unMarshal(buf []byte, pb proto.Message) error {
	if um.attemptsLeft == 1 {
		return fmt.Errorf("Failed unmarshalling")
	}
	um.attemptsLeft--
	return proto.Unmarshal(buf, pb)
}

func failUnmarshallAtAttempt(attempts int) func (buf []byte, pb proto.Message) error {
	um := &unMarshaller{attemptsLeft: attempts}
	return um.unMarshal
}

func TestInvalidJoinChannelBlock(t *testing.T) {
	nilHeader := func(block *common.Block) *common.Block {
		block.Header = nil
		return block
	}
	testJoinChannelFails(t, nilHeader, nil)
	nilData := func(block *common.Block) *common.Block {
		block.Data = nil
		return block
	}
	testJoinChannelFails(t, nilData, nil)
	emptyData := func(block *common.Block) *common.Block {
		block.Data.Data = [][]byte{}
		return block
	}
	testJoinChannelFails(t, emptyData, nil)
	tooMuchInData := func(block *common.Block) *common.Block {
		block.Data.Data = [][]byte{{}, {}}
		return block
	}
	testJoinChannelFails(t, tooMuchInData, nil)
	// Payload unmarshalling
	testJoinChannelFails(t, nil, failUnmarshallAtAttempt(1))
	// Configuration Envelope unmarshalling
	testJoinChannelFails(t, nil, failUnmarshallAtAttempt(2))
	// ConfigurationEnvelope unmarshalling
	testJoinChannelFails(t, nil, failUnmarshallAtAttempt(3))
	// ConfigurationItem unmarshalling
	testJoinChannelFails(t, nil, failUnmarshallAtAttempt(4))
	// Anchor peer unmarshalling
	testJoinChannelFails(t, nil, failUnmarshallAtAttempt(5))

	emptyConfEnvelope := func(_ *common.Block) *common.Block {
		block, _ := genesis.NewFactoryImpl(configtx.NewSimpleTemplate()).Block("TEST")
		return block
	}
	testJoinChannelFails(t, emptyConfEnvelope, nil)

	multipleAnchorPeers := func(_ *common.Block) *common.Block {
		cert := []byte("cert")
		host := "localhost"
		port := 5611
		anchorPeers := &peer.AnchorPeers{
			AnchorPees: []*peer.AnchorPeer{{Cert: cert, Host: host, Port: int32(port)}},
		}
		confItem1 := createAnchorPeerConfItem(t, anchorPeers, utils.AnchorPeerConfItemKey)
		confItem2 := createAnchorPeerConfItem(t, anchorPeers, utils.AnchorPeerConfItemKey)
		block, _ := genesis.NewFactoryImpl(configtx.NewSimpleTemplate(confItem1, confItem2)).Block("TEST")
		return block
	}
	testJoinChannelFails(t, multipleAnchorPeers, nil)


	noAnchorPeers := func(_ *common.Block) *common.Block {
		anchorPeers := &peer.AnchorPeers{
			AnchorPees: []*peer.AnchorPeer{},
		}
		confItem := createAnchorPeerConfItem(t, anchorPeers, utils.AnchorPeerConfItemKey)
		block, _ := genesis.NewFactoryImpl(configtx.NewSimpleTemplate(confItem)).Block("TEST")
		return block
	}
	testJoinChannelFails(t, noAnchorPeers, nil)

	noAnchorPeerItemType := func(_ *common.Block) *common.Block {
		anchorPeers := &peer.AnchorPeers{
			AnchorPees: []*peer.AnchorPeer{},
		}
		confItem := createAnchorPeerConfItem(t, anchorPeers, "MSP configuration")
		block, _ := genesis.NewFactoryImpl(configtx.NewSimpleTemplate(confItem)).Block("TEST")
		return block
	}
	testJoinChannelFails(t, noAnchorPeerItemType, nil)
}

func TestValidJoinChannelBlock(t *testing.T) {
	block := createValidJoinChanMessage(t, 100, "localhost", 5611, []byte("cert"))
	jcm, err := JoinChannelMessageFromBlock(block)
	assert.NoError(t, err)
	assert.Len(t, jcm.AnchorPeers(), 1)
	assert.Equal(t, uint64(100), jcm.SequenceNumber())
	assert.Equal(t, "localhost", jcm.AnchorPeers()[0].Host)
	assert.Equal(t, 5611, jcm.AnchorPeers()[0].Port)
	assert.Equal(t, []byte("cert"), []byte(jcm.AnchorPeers()[0].Cert))
}


func testJoinChannelFails(t *testing.T, invalidator blockInvalidator, unMarshallFunc func(buf []byte, pb proto.Message) error) {
	if unMarshallFunc != nil {
		unMarshal = unMarshallFunc
	}

	block := createValidJoinChanMessage(t, 100, "localhost", 5611, []byte("cert"))

	if invalidator != nil {
		block = invalidator(block)
	}
	jcm, err := JoinChannelMessageFromBlock(block)
	assert.Error(t, err)
	assert.Nil(t, jcm)
	// Restore previous unmarshal function anyway
	unMarshal = proto.Unmarshal
}


func createValidJoinChanMessage(t *testing.T, seqNum int, host string, port int, cert []byte) *common.Block {
	anchorPeers := &peer.AnchorPeers{
		AnchorPees: []*peer.AnchorPeer{{Cert: cert, Host: host, Port: int32(port)}},
	}
	confItem := createAnchorPeerConfItem(t, anchorPeers, utils.AnchorPeerConfItemKey)
	block, err := genesis.NewFactoryImpl(configtx.NewSimpleTemplate(confItem)).Block("TEST")
	block.Header.Number = uint64(seqNum)
	assert.NoError(t, err, "Failed creating genesis block")
	return block
}

func createAnchorPeerConfItem(t *testing.T, anchorPeers *peer.AnchorPeers, key string) *common.ConfigurationItem {
	serializedAnchorPeers, err := proto.Marshal(anchorPeers)
	assert.NoError(t, err, "Failed marshalling AnchorPeers")
	return &common.ConfigurationItem{
		Header: &common.ChainHeader{
			ChainID: "TEST",
			Type:    int32(common.HeaderType_CONFIGURATION_ITEM),
		},
		Key:                key,
		LastModified:       uint64(time.Now().UnixNano()),
		ModificationPolicy: "",
		Type:               common.ConfigurationItem_Peer,
		Value:              serializedAnchorPeers,
	}
}