/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cscc

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/configtx"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/aclmgmt"
	aclmocks "github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	policymocks "github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	peergossip "github.com/hyperledger/fabric/internal/peer/gossip"
	"github.com/hyperledger/fabric/internal/peer/gossip/mocks"
	"github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

//go:generate counterfeiter -o mock/config_manager.go --fake-name ConfigManager . configManager

type configManager interface {
	config.Manager
}

//go:generate counterfeiter -o mock/acl_provider.go --fake-name ACLProvider . aclProvider

type aclProvider interface {
	aclmgmt.ACLProvider
}

//go:generate counterfeiter -o mock/configtx_validator.go --fake-name ConfigtxValidator . configtxValidator

type configtxValidator interface {
	configtx.Validator
}

var mockAclProvider *aclmocks.MockACLProvider

func TestMain(m *testing.M) {
	// TODO: remove the transient store and peer setup once we've completed the
	// transition to instances
	tempdir, err := ioutil.TempDir("", "scc-configure")
	if err != nil {
		panic(err)
	}
	viper.Set("peer.fileSystemPath", filepath.Join(tempdir, "transientstore"))
	peer.Default = &peer.Peer{
		StoreProvider: transientstore.NewStoreProvider(),
	}

	msptesttools.LoadMSPSetupForTesting()

	mockAclProvider = &aclmocks.MockACLProvider{}
	mockAclProvider.Reset()

	rc := m.Run()

	os.RemoveAll(tempdir)
	os.Exit(rc)
}

func TestConfigerInit(t *testing.T) {
	e := New(nil, mockAclProvider, nil, nil, nil)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
}

func TestConfigerInvokeInvalidParameters(t *testing.T) {
	e := New(nil, mockAclProvider, nil, nil, nil)
	stub := shim.NewMockStub("PeerConfiger", e)

	res := stub.MockInit("1", nil)
	assert.Equal(t, res.Status, int32(shim.OK), "Init failed")

	res = stub.MockInvoke("2", nil)
	assert.Equal(t, res.Status, int32(shim.ERROR), "CSCC invoke expected to fail having zero arguments")
	assert.Equal(t, res.Message, "Incorrect number of arguments, 0")

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_GetChannels, "", (*pb.SignedProposal)(nil)).Return(errors.New("Failed authorization"))
	args := [][]byte{[]byte("GetChannels")}
	res = stub.MockInvokeWithSignedProposal("3", args, nil)
	assert.Equal(t, res.Status, int32(shim.ERROR), "CSCC invoke expected to fail no signed proposal provided")
	assert.Contains(t, res.Message, "access denied for [GetChannels]")

	args = [][]byte{[]byte("fooFunction"), []byte("testChainID")}
	res = stub.MockInvoke("5", args)
	assert.Equal(t, res.Status, int32(shim.ERROR), "CSCC invoke expected wrong function name provided")
	assert.Equal(t, res.Message, "Requested function fooFunction not found.")

	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_GetConfigBlock, "testChainID", (*pb.SignedProposal)(nil)).Return(errors.New("Nil SignedProposal"))
	args = [][]byte{[]byte("GetConfigBlock"), []byte("testChainID")}
	res = stub.MockInvokeWithSignedProposal("4", args, nil)
	assert.Equal(t, res.Status, int32(shim.ERROR), "CSCC invoke expected to fail no signed proposal provided")
	assert.Contains(t, res.Message, "Nil SignedProposal")
	mockAclProvider.AssertExpectations(t)
}

func TestConfigerInvokeJoinChainMissingParams(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/hyperledgertest/")
	os.Mkdir("/tmp/hyperledgertest", 0755)
	defer os.RemoveAll("/tmp/hyperledgertest/")

	e := New(nil, mockAclProvider, nil, nil, nil)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Failed path: expected to have at least one argument
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_JoinChain, "", (*pb.SignedProposal)(nil)).Return(nil)
	args := [][]byte{[]byte("JoinChain")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain should have failed with invalid number of args: %v", args)
	}
}

func TestConfigerInvokeJoinChainWrongParams(t *testing.T) {

	viper.Set("peer.fileSystemPath", "/tmp/hyperledgertest/")
	os.Mkdir("/tmp/hyperledgertest", 0755)
	defer os.RemoveAll("/tmp/hyperledgertest/")

	e := New(nil, mockAclProvider, nil, nil, nil)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Failed path: wrong parameter type
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_JoinChain, "", (*pb.SignedProposal)(nil)).Return(nil)
	args := [][]byte{[]byte("JoinChain"), []byte("action")}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain should have failed with null genesis block.  args: %v", args)
	}
}

type PackageProviderWrapper struct {
	FS *ccprovider.CCInfoFSImpl
}

func (p *PackageProviderWrapper) GetChaincodeCodePackage(ccci *ccprovider.ChaincodeContainerInfo) ([]byte, error) {
	return p.FS.GetChaincodeCodePackage(ccci.Name, ccci.Version)
}

func TestConfigerInvokeJoinChainCorrectParams(t *testing.T) {
	mp := (&scc.MocksccProviderFactory{}).NewSystemChaincodeProvider()

	viper.Set("chaincode.executetimeout", "3s")

	cleanup, err := peer.MockInitialize()
	if err != nil {
		t.Fatalf("Failed to initialize peer: %s", err)
	}
	defer cleanup()

	e := New(mp, mockAclProvider, nil, nil, nil)
	stub := shim.NewMockStub("PeerConfiger", e)

	peerEndpoint := "127.0.0.1:13611"

	config := chaincode.GlobalConfig()
	config.StartupTimeout = 30 * time.Second

	// Init the policy checker
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"mytestchainid": &policymocks.MockChannelPolicyManager{
				MockPolicy: &policymocks.MockPolicy{
					Deserializer: &policymocks.MockIdentityDeserializer{
						Identity: []byte("Alice"),
						Msg:      []byte("msg1"),
					},
				},
			},
		},
	}

	identityDeserializer := &policymocks.MockIdentityDeserializer{
		Identity: []byte("Alice"),
		Msg:      []byte("msg1"),
	}

	e.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	grpcServer := grpc.NewServer()
	socket, err := net.Listen("tcp", peerEndpoint)
	require.NoError(t, err)

	signer := mgmt.GetLocalSigningIdentityOrPanic()
	messageCryptoService := peergossip.NewMCS(&mocks.ChannelPolicyManagerGetter{}, signer, mgmt.NewDeserializersManager())
	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	var defaultSecureDialOpts = func() []grpc.DialOption {
		var dialOpts []grpc.DialOption
		dialOpts = append(dialOpts, grpc.WithInsecure())
		return dialOpts
	}

	err = service.InitGossipService(
		signer,
		&disabled.Provider{},
		peerEndpoint,
		grpcServer,
		nil,
		messageCryptoService,
		secAdv,
		defaultSecureDialOpts,
	)
	assert.NoError(t, err)

	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke JoinChain failed because invalid block")
	}
	args := [][]byte{[]byte("JoinChain"), blockBytes}
	sProp, _ := protoutil.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	// Try fail path with nil block
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_JoinChain, "", sProp).Return(nil)
	res := stub.MockInvokeWithSignedProposal("2", [][]byte{[]byte("JoinChain"), nil}, sProp)
	assert.Equal(t, res.Status, int32(shim.ERROR))

	// Try fail path with block and nil payload header
	payload, _ := proto.Marshal(&cb.Payload{})
	env, _ := proto.Marshal(&cb.Envelope{
		Payload: payload,
	})
	badBlock := &cb.Block{
		Data: &cb.BlockData{
			Data: [][]byte{env},
		},
	}
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_JoinChain, "", sProp).Return(nil)
	badBlockBytes := protoutil.MarshalOrPanic(badBlock)
	res = stub.MockInvokeWithSignedProposal("2", [][]byte{[]byte("JoinChain"), badBlockBytes}, sProp)
	assert.Equal(t, res.Status, int32(shim.ERROR))

	// Now, continue with valid execution path
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status != shim.OK {
		t.Fatalf("cscc invoke JoinChain failed with: %v", res.Message)
	}

	// This call must fail
	sProp.Signature = nil
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_JoinChain, "", sProp).Return(errors.New("Failed authorization"))
	res = stub.MockInvokeWithSignedProposal("3", args, sProp)
	if res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain must fail : %v", res.Message)
	}
	assert.Contains(t, res.Message, "access denied for [JoinChain][mytestchainid]")
	sProp.Signature = sProp.ProposalBytes

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := protoutil.GetChainIDFromBlockBytes(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}

	// Test an ACL failure on GetConfigBlock
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_GetConfigBlock, "mytestchainid", sProp).Return(errors.New("Failed authorization"))
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	res = stub.MockInvokeWithSignedProposal("2", args, sProp)
	if res.Status == shim.OK {
		t.Fatalf("cscc invoke GetConfigBlock should have failed: %v", res.Message)
	}
	assert.Contains(t, res.Message, "Failed authorization")
	mockAclProvider.AssertExpectations(t)

	// Test with ACL okay
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_GetConfigBlock, "mytestchainid", sProp).Return(nil)
	if res := stub.MockInvokeWithSignedProposal("2", args, sProp); res.Status != shim.OK {
		t.Fatalf("cscc invoke GetConfigBlock failed with: %v", res.Message)
	}

	// get channels for the peer
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_GetChannels, "", sProp).Return(nil)
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

func TestPeerConfiger_SubmittingOrdererGenesis(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/hyperledgertest/")
	os.Mkdir("/tmp/hyperledgertest", 0755)
	defer os.RemoveAll("/tmp/hyperledgertest/")

	e := New(nil, nil, nil, nil, nil)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
	conf := configtxgentest.Load(genesisconfig.SampleSingleMSPSoloProfile)
	conf.Application = nil
	cg, err := encoder.NewChannelGroup(conf)
	assert.NoError(t, err)
	block := genesis.NewFactoryImpl(cg).Block("mytestchainid")
	blockBytes := protoutil.MarshalOrPanic(block)

	// Failed path: wrong parameter type
	mockAclProvider.Reset()
	mockAclProvider.On("CheckACL", resources.Cscc_JoinChain, "", (*pb.SignedProposal)(nil)).Return(nil)
	args := [][]byte{[]byte("JoinChain"), []byte(blockBytes)}
	if res := stub.MockInvoke("2", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain should have failed with wrong genesis block.  args: %v", args)
	} else {
		assert.Contains(t, res.Message, "missing Application configuration group")
	}
}

func mockConfigBlock() []byte {
	var blockBytes []byte = nil
	block, err := configtxtest.MakeGenesisBlock("mytestchainid")
	if err == nil {
		blockBytes = protoutil.MarshalOrPanic(block)
	}
	return blockBytes
}
