/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cscc

import (
	"errors"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	"github.com/hyperledger/fabric/core/scc/cscc/mocks"
	"github.com/hyperledger/fabric/core/transientstore"
	gossipmetrics "github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	peergossip "github.com/hyperledger/fabric/internal/peer/gossip"
	"github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

//go:generate counterfeiter -o mocks/acl_provider.go --fake-name ACLProvider . aclProvider

type aclProvider interface {
	aclmgmt.ACLProvider
}

//go:generate counterfeiter -o mocks/chaincode_stub.go --fake-name ChaincodeStub . chaincodeStub

type chaincodeStub interface {
	shim.ChaincodeStubInterface
}

//go:generate counterfeiter -o mocks/channel_policy_manager_getter.go --fake-name ChannelPolicyManagerGetter . channelPolicyManagerGetter

type channelPolicyManagerGetter interface {
	policies.ChannelPolicyManagerGetter
}

//go:generate counterfeiter -o mocks/policy_checker.go --fake-name PolicyChecker . policyChecker

type policyChecker interface {
	policy.PolicyChecker
}

//go:generate counterfeiter -o mocks/store_provider.go --fake-name StoreProvider . storeProvider

type storeProvider interface {
	transientstore.StoreProvider
}

func TestMain(m *testing.M) {
	// TODO: remove the transient store and peer setup once we've completed the
	// transition to instances
	tempdir, err := ioutil.TempDir("", "scc-configure")
	if err != nil {
		panic(err)
	}
	peer.Default = &peer.Peer{
		StoreProvider: transientstore.NewStoreProvider(filepath.Join(tempdir, "transientstore")),
	}

	msptesttools.LoadMSPSetupForTesting()

	rc := m.Run()

	os.RemoveAll(tempdir)
	os.Exit(rc)

}

func TestConfigerInit(t *testing.T) {
	mockACLProvider := &mocks.ACLProvider{}
	mockStub := &mocks.ChaincodeStub{}
	cscc := &PeerConfiger{
		aclProvider: mockACLProvider,
	}
	res := cscc.Init(mockStub)
	assert.Equal(t, int32(shim.OK), res.Status)
}

func TestConfigerInvokeInvalidParameters(t *testing.T) {
	mockACLProvider := &mocks.ACLProvider{}
	cscc := &PeerConfiger{
		aclProvider: mockACLProvider,
	}
	mockStub := &mocks.ChaincodeStub{}

	mockStub.GetArgsReturns(nil)
	res := cscc.Invoke(mockStub)
	assert.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"cscc invoke expected to fail having zero arguments",
	)
	assert.Equal(t, "Incorrect number of arguments, 0", res.Message)

	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	args := [][]byte{[]byte("GetChannels")}
	mockStub.GetArgsReturns(args)
	res = cscc.Invoke(mockStub)
	assert.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke expected to fail no signed proposal provided",
	)
	assert.Equal(t, "access denied for [GetChannels]: Failed authorization", res.Message)

	mockACLProvider.CheckACLReturns(nil)
	args = [][]byte{[]byte("fooFunction"), []byte("testChainID")}
	mockStub.GetArgsReturns(args)
	res = cscc.Invoke(mockStub)
	assert.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke invoke expected wrong function name provided",
	)
	assert.Equal(t, "Requested function fooFunction not found.", res.Message)

	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	args = [][]byte{[]byte("GetConfigBlock"), []byte("testChainID")}
	mockStub.GetArgsReturns(args)
	res = cscc.Invoke(mockStub)
	assert.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke expected to fail no signed proposal provided",
	)
	assert.Equal(
		t,
		"access denied for [GetConfigBlock][testChainID]: Failed authorization",
		res.Message,
	)
}

func TestConfigerInvokeJoinChainMissingParams(t *testing.T) {
	cscc := &PeerConfiger{
		aclProvider: &mocks.ACLProvider{},
	}
	mockStub := &mocks.ChaincodeStub{}
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChain")})
	res := cscc.Invoke(mockStub)
	assert.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"cscc invoke JoinChain should have failed with invalid number of args",
	)
}

func TestConfigerInvokeJoinChainWrongParams(t *testing.T) {
	cscc := &PeerConfiger{
		aclProvider: &mocks.ACLProvider{},
	}
	mockStub := &mocks.ChaincodeStub{}
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChain"), []byte("action")})
	res := cscc.Invoke(mockStub)
	assert.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"cscc invoke JoinChain should have failed with null genesis block",
	)
}

type PackageProviderWrapper struct {
	FS *ccprovider.CCInfoFSImpl
}

func (p *PackageProviderWrapper) GetChaincodeCodePackage(ccci *ccprovider.ChaincodeContainerInfo) ([]byte, error) {
	return p.FS.GetChaincodeCodePackage(ccci.Name, ccci.Version)
}

func TestConfigerInvokeJoinChainCorrectParams(t *testing.T) {
	viper.Set("chaincode.executetimeout", "3s")

	cleanup, err := peer.MockInitialize()
	if err != nil {
		t.Fatalf("Failed to initialize peer: %s", err)
	}
	defer cleanup()

	peerEndpoint := "127.0.0.1:13611"

	config := chaincode.GlobalConfig()
	config.StartupTimeout = 30 * time.Second

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

	gossipService, err := service.New(
		signer,
		gossipmetrics.NewGossipMetrics(&disabled.Provider{}),
		peerEndpoint,
		grpcServer,
		nil,
		messageCryptoService,
		secAdv,
		defaultSecureDialOpts,
		nil,
	)
	assert.NoError(t, err)
	peer.Default.GossipService = gossipService
	defer func() { peer.Default.GossipService = nil }()

	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	// setup cscc instance
	mockACLProvider := &mocks.ACLProvider{}
	cscc := &PeerConfiger{
		policyChecker: &mocks.PolicyChecker{},
		aclProvider:   mockACLProvider,
		peer: &peer.Peer{
			StoreProvider: &mocks.StoreProvider{},
			GossipService: gossipService,
		},
	}
	mockStub := &mocks.ChaincodeStub{}

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke JoinChain failed because invalid block")
	}
	args := [][]byte{[]byte("JoinChain"), blockBytes}
	sProp, _ := protoutil.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.Signature = sProp.ProposalBytes

	// Try fail path with nil block
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChain"), nil})
	res := cscc.Invoke(mockStub)
	//res := stub.MockInvokeWithSignedProposal("2", [][]byte{[]byte("JoinChain"), nil}, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status)

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
	badBlockBytes := protoutil.MarshalOrPanic(badBlock)
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChain"), badBlockBytes})
	res = cscc.Invoke(mockStub)
	//res = stub.MockInvokeWithSignedProposal("2", [][]byte{[]byte("JoinChain"), badBlockBytes}, sProp)
	assert.Equal(t, int32(shim.ERROR), res.Status)

	// Now, continue with valid execution path
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(sProp, nil)
	res = cscc.Invoke(mockStub)
	assert.Equal(t, int32(shim.OK), res.Status, "invoke JoinChain failed with: %v", res.Message)

	// This call must fail
	sProp.Signature = nil
	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(sProp, nil)

	res = cscc.Invoke(mockStub)
	assert.Equal(t, int32(shim.ERROR), res.Status)
	assert.Contains(t, res.Message, "access denied for [JoinChain][mytestchainid]")
	sProp.Signature = sProp.ProposalBytes

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := protoutil.GetChainIDFromBlockBytes(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}

	// Test an ACL failure on GetConfigBlock
	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(sProp, nil)
	res = cscc.Invoke(mockStub)
	assert.Equal(t, int32(shim.ERROR), res.Status, "invoke GetConfigBlock should have failed: %v", res.Message)
	assert.Contains(t, res.Message, "Failed authorization")

	// Test with ACL okay
	mockACLProvider.CheckACLReturns(nil)
	res = cscc.Invoke(mockStub)
	assert.Equal(t, int32(shim.OK), res.Status, "invoke GetConfigBlock failed with: %v", res.Message)

	// get channels for the peer
	mockACLProvider.CheckACLReturns(nil)
	args = [][]byte{[]byte(GetChannels)}
	mockStub.GetArgsReturns(args)
	res = cscc.Invoke(mockStub)
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
	conf := configtxgentest.Load(genesisconfig.SampleSingleMSPSoloProfile)
	conf.Application = nil
	cg, err := encoder.NewChannelGroup(conf)
	assert.NoError(t, err)
	block := genesis.NewFactoryImpl(cg).Block("mytestchainid")
	blockBytes := protoutil.MarshalOrPanic(block)

	mockACLProvider := &mocks.ACLProvider{}
	cscc := &PeerConfiger{
		aclProvider: mockACLProvider,
	}
	mockStub := &mocks.ChaincodeStub{}
	// Failed path: wrong parameter type
	args := [][]byte{[]byte("JoinChain"), []byte(blockBytes)}
	mockStub.GetArgsReturns(args)
	res := cscc.Invoke(mockStub)

	assert.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke JoinChain should have failed with wrong genesis block",
	)
	assert.Contains(t, res.Message, "missing Application configuration group")
}

func mockConfigBlock() []byte {
	var blockBytes []byte = nil
	block, err := configtxtest.MakeGenesisBlock("mytestchainid")
	if err == nil {
		blockBytes = protoutil.MarshalOrPanic(block)
	}
	return blockBytes
}
