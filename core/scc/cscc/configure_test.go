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
package cscc

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	peergossip "github.com/hyperledger/fabric/peer/gossip"
	"github.com/hyperledger/fabric/peer/gossip/mocks"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type mockDeliveryClient struct {
}

// StartDeliverForChannel dynamically starts delivery of new blocks from ordering service
// to channel peers.
func (ds *mockDeliveryClient) StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo) error {
	return nil
}

// StopDeliverForChannel dynamically stops delivery of new blocks from ordering service
// to channel peers.
func (ds *mockDeliveryClient) StopDeliverForChannel(chainID string) error {
	return nil
}

// Stop terminates delivery service and closes the connection
func (*mockDeliveryClient) Stop() {

}

type mockDeliveryClientFactory struct {
}

func (*mockDeliveryClientFactory) Service(g service.GossipService, endpoints []string, mcs api.MessageCryptoService) (deliverclient.DeliverService, error) {
	return &mockDeliveryClient{}, nil
}

func TestMain(m *testing.M) {
	msptesttools.LoadMSPSetupForTesting()

	os.Exit(m.Run())
}

func TestConfigerInit(t *testing.T) {
	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
}

func TestConfigerInvokeJoinChainMissingParams(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/hyperledgertest/")
	os.Mkdir("/tmp/hyperledgertest", 0755)
	defer os.RemoveAll("/tmp/hyperledgertest/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Failed path: Not enough parameters
	args := [][]byte{[]byte("JoinChain")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain should have failed with invalid number of args: %v", args)
	}
}

func TestConfigerInvokeJoinChainWrongParams(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/hyperledgertest/")
	os.Mkdir("/tmp/hyperledgertest", 0755)
	defer os.RemoveAll("/tmp/hyperledgertest/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Failed path: wrong parameter type
	args := [][]byte{[]byte("JoinChain"), []byte("action")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain should have failed with null genesis block.  args: %v", args)
	}
}

func TestConfigerInvokeJoinChainCorrectParams(t *testing.T) {
	sysccprovider.RegisterSystemChaincodeProviderFactory(&scc.MocksccProviderFactory{})

	viper.Set("peer.fileSystemPath", "/tmp/hyperledgertest/")
	viper.Set("chaincode.executetimeout", "3s")
	os.Mkdir("/tmp/hyperledgertest", 0755)

	peer.MockInitialize()
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()
	defer os.RemoveAll("/tmp/hyperledgerest/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	peerEndpoint := "localhost:13611"
	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{Id: &pb.PeerID{Name: "cscctestpeer"}, Address: peerEndpoint}, nil
	}
	ccStartupTimeout := time.Duration(30000) * time.Millisecond
	chaincode.NewChaincodeSupport(getPeerEndpoint, false, ccStartupTimeout)

	// Init the policy checker
	policyManagerGetter := &policy.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"mytestchainid": &policy.MockChannelPolicyManager{MockPolicy: &policy.MockPolicy{Deserializer: &policy.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}

	identityDeserializer := &policy.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}

	e.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policy.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	identity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()
	messageCryptoService := peergossip.NewMCS(&mocks.ChannelPolicyManagerGetter{}, localmsp.NewSigner(), mgmt.NewDeserializersManager())
	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	service.InitGossipServiceCustomDeliveryFactory(identity, peerEndpoint, nil, &mockDeliveryClientFactory{}, messageCryptoService, secAdv, nil)

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke JoinChain failed because invalid block")
	}
	args := [][]byte{[]byte("JoinChain"), blockBytes}
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status != shim.OK {
		t.Fatalf("cscc invoke JoinChain failed with: %v", res.Message)
	}

	// This call must fail
	sProp.Signature = nil
	res := stub.MockInvokeWithSignedProposal("3", args, sProp)
	if res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain must fail : %v", res.Message)
	}
	assert.True(t, strings.HasPrefix(res.Message, "\"JoinChain\" request failed authorization check for channel"))
	sProp.Signature = sProp.ProposalBytes

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := getChainID(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	policyManagerGetter.Managers["mytestchainid"].(*policy.MockChannelPolicyManager).MockPolicy.(*policy.MockPolicy).Deserializer.(*policy.MockIdentityDeserializer).Msg = sProp.ProposalBytes
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status != shim.OK {
		t.Fatalf("cscc invoke GetConfigBlock failed with: %v", res.Message)
	}

	// get channels for the peer
	args = [][]byte{[]byte(GetChannels)}
	res = stub.MockInvokeWithSignedProposal("2", args, sProp)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr := &pb.ChannelQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}

	// peer joined one channel so query should return an array with one channel
	if len(cqr.GetChannels()) != 1 {
		t.FailNow()
	}
}

func TestConfigerInvokeUpdateConfigBlock(t *testing.T) {
	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	// Init the policy checker
	policyManagerGetter := &policy.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"mytestchainid": &policy.MockChannelPolicyManager{MockPolicy: &policy.MockPolicy{Deserializer: &policy.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}}},
		},
	}

	identityDeserializer := &policy.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}

	e.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policy.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	policyManagerGetter.Managers["mytestchainid"].(*policy.MockChannelPolicyManager).MockPolicy.(*policy.MockPolicy).Deserializer.(*policy.MockIdentityDeserializer).Msg = sProp.ProposalBytes

	// Failed path: Not enough parameters
	args := [][]byte{[]byte("UpdateConfigBlock")}
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status == shim.OK {
		t.Fatalf("cscc invoke UpdateConfigBlock should have failed with invalid number of args: %v", args)
	}

	// Failed path: wrong parameter type
	args = [][]byte{[]byte("UpdateConfigBlock"), []byte("action")}
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status == shim.OK {
		t.Fatalf("cscc invoke UpdateConfigBlock should have failed with null genesis block - args: %v", args)
	}

	// Successful path for UpdateConfigBlock
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke UpdateConfigBlock failed because invalid block")
	}
	args = [][]byte{[]byte("UpdateConfigBlock"), blockBytes}
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status != shim.OK {
		t.Fatalf("cscc invoke UpdateConfigBlock failed with: %v", res.Message)
	}

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := getChainID(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke UpdateConfigBlock failed with: %v", err)
	}
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status != shim.OK {
		t.Fatalf("cscc invoke GetConfigBlock failed with: %v", err)
	}

}

func mockConfigBlock() []byte {
	var blockBytes []byte
	block, err := configtxtest.MakeGenesisBlock("mytestchainid")
	if err != nil {
		blockBytes = nil
	} else {
		blockBytes = utils.MarshalOrPanic(block)
	}
	return blockBytes
}

func getChainID(blockBytes []byte) (string, error) {
	block := &common.Block{}
	if err := proto.Unmarshal(blockBytes, block); err != nil {
		return "", err
	}
	envelope := &common.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], envelope); err != nil {
		return "", err
	}
	payload := &common.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		return "", err
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return "", err
	}
	fmt.Printf("Channel id: %v\n", chdr.ChannelId)
	return chdr.ChannelId, nil
}
