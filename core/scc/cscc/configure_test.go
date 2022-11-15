/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cscc

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/genesis"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc/cscc/mocks"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/gossip"
	gossipmetrics "github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/privdata"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	peergossip "github.com/hyperledger/fabric/internal/peer/gossip"
	"github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
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

//go:generate counterfeiter -o mocks/store_provider.go --fake-name StoreProvider . storeProvider

type storeProvider interface {
	transientstore.StoreProvider
}

func TestMain(m *testing.M) {
	msptesttools.LoadMSPSetupForTesting()
	rc := m.Run()
	os.Exit(rc)
}

func TestConfigerInit(t *testing.T) {
	mockACLProvider := &mocks.ACLProvider{}
	mockStub := &mocks.ChaincodeStub{}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cscc := &PeerConfiger{
		aclProvider: mockACLProvider,
		bccsp:       cryptoProvider,
	}
	res := cscc.Init(mockStub)
	require.Equal(t, int32(shim.OK), res.Status)
}

func TestConfigerInvokeInvalidParameters(t *testing.T) {
	mockACLProvider := &mocks.ACLProvider{}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cscc := &PeerConfiger{
		aclProvider: mockACLProvider,
		bccsp:       cryptoProvider,
	}
	mockStub := &mocks.ChaincodeStub{}

	mockStub.GetArgsReturns(nil)
	mockStub.GetSignedProposalReturns(validSignedProposal(), nil)
	res := cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"cscc invoke expected to fail having zero arguments",
	)
	require.Equal(t, "Incorrect number of arguments, 0", res.Message)

	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	args := [][]byte{[]byte("GetChannels")}
	mockStub.GetArgsReturns(args)
	res = cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke expected to fail no signed proposal provided",
	)
	require.Equal(t, "access denied for [GetChannels]: Failed authorization", res.Message)

	mockACLProvider.CheckACLReturns(nil)
	args = [][]byte{[]byte("fooFunction"), []byte("testChannelID")}
	mockStub.GetArgsReturns(args)
	res = cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke expected wrong function name provided",
	)
	require.Equal(t, "Requested function fooFunction not found.", res.Message)

	mockACLProvider.CheckACLReturns(nil)
	args = [][]byte{[]byte("GetConfigBlock"), []byte("testChannelID")}
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(&pb.SignedProposal{
		ProposalBytes: []byte("garbage"),
	}, nil)
	res = cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke expected to fail in ccc2cc context",
	)
	require.Contains(t, res.Message, "Failed to identify the called chaincode: could not unmarshal proposal")

	mockACLProvider.CheckACLReturns(nil)
	args = [][]byte{[]byte("GetConfigBlock"), []byte("testChannelID")}
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(&pb.SignedProposal{
		ProposalBytes: protoutil.MarshalOrPanic(&pb.Proposal{
			Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
				Input: protoutil.MarshalOrPanic(&pb.ChaincodeInvocationSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						ChaincodeId: &pb.ChaincodeID{
							Name: "fake-cc2cc",
						},
					},
				}),
			}),
		}),
	}, nil)
	res = cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke expected to fail in ccc2cc context",
	)
	require.Equal(
		t,
		"Rejecting invoke of CSCC from another chaincode, original invocation for 'fake-cc2cc'",
		res.Message,
	)

	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	mockStub.GetSignedProposalReturns(validSignedProposal(), nil)
	args = [][]byte{[]byte("GetConfigBlock"), []byte("testChannelID")}
	mockStub.GetArgsReturns(args)
	res = cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke expected to fail no signed proposal provided",
	)
	require.Equal(
		t,
		"access denied for [GetConfigBlock][testChannelID]: Failed authorization",
		res.Message,
	)
}

func TestConfigerInvokeJoinChainMissingParams(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cscc := &PeerConfiger{
		aclProvider: &mocks.ACLProvider{},
		bccsp:       cryptoProvider,
	}
	mockStub := &mocks.ChaincodeStub{}
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChain")})
	res := cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"cscc invoke JoinChain should have failed with invalid number of args",
	)
}

func TestConfigerInvokeJoinChainWrongParams(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	cscc := &PeerConfiger{
		aclProvider: &mocks.ACLProvider{},
		bccsp:       cryptoProvider,
	}
	mockStub := &mocks.ChaincodeStub{}
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChain"), []byte("action")})
	mockStub.GetSignedProposalReturns(validSignedProposal(), nil)
	res := cscc.Invoke(mockStub)
	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"cscc invoke JoinChain should have failed with null genesis block",
	)
}

func TestConfigerInvokeJoinChainCorrectParams(t *testing.T) {
	testDir := t.TempDir()

	ledgerInitializer := ledgermgmttest.NewInitializer(testDir)
	ledgerInitializer.CustomTxProcessors = map[cb.HeaderType]ledger.CustomTxProcessor{
		cb.HeaderType_CONFIG: &peer.ConfigTxProcessor{},
	}
	ledgerMgr := ledgermgmt.NewLedgerMgr(ledgerInitializer)
	defer ledgerMgr.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:")
	require.NoError(t, err)
	grpcServer := grpc.NewServer()

	cscc := newPeerConfiger(t, ledgerMgr, grpcServer, listener.Addr().String())

	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	mockACLProvider := cscc.aclProvider.(*mocks.ACLProvider)
	mockStub := &mocks.ChaincodeStub{}

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke JoinChain failed because invalid block")
	}
	args := [][]byte{[]byte("JoinChain"), blockBytes}
	sProp := validSignedProposal()
	sProp.Signature = sProp.ProposalBytes

	// Try fail path with nil block
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChain"), nil})
	mockStub.GetSignedProposalReturns(sProp, nil)
	res := cscc.Invoke(mockStub)
	// res := stub.MockInvokeWithSignedProposal("2", [][]byte{[]byte("JoinChain"), nil}, sProp)
	require.Equal(t, int32(shim.ERROR), res.Status)

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
	// res = stub.MockInvokeWithSignedProposal("2", [][]byte{[]byte("JoinChain"), badBlockBytes}, sProp)
	require.Equal(t, int32(shim.ERROR), res.Status)

	// Now, continue with valid execution path
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(sProp, nil)
	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.OK), res.Status, "invoke JoinChain failed with: %v", res.Message)

	// This call must fail
	sProp.Signature = nil
	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(sProp, nil)

	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.ERROR), res.Status)
	require.Contains(t, res.Message, "access denied for [JoinChain][mytestchannelid]")
	sProp.Signature = sProp.ProposalBytes

	// Query the configuration block
	// channelID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	channelID, err := protoutil.GetChannelIDFromBlockBytes(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}

	// Test an ACL failure on GetConfigBlock
	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	args = [][]byte{[]byte("GetConfigBlock"), []byte(channelID)}
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(sProp, nil)
	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.ERROR), res.Status, "invoke GetConfigBlock should have failed: %v", res.Message)
	require.Contains(t, res.Message, "Failed authorization")

	// Test with ACL okay
	mockACLProvider.CheckACLReturns(nil)
	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.OK), res.Status, "invoke GetConfigBlock failed with: %v", res.Message)

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

func TestConfigerInvokeJoinChainBySnapshot(t *testing.T) {
	testDir := t.TempDir()

	ledgerInitializer := ledgermgmttest.NewInitializer(testDir)
	ledgerInitializer.CustomTxProcessors = map[cb.HeaderType]ledger.CustomTxProcessor{
		cb.HeaderType_CONFIG: &peer.ConfigTxProcessor{},
	}
	ledgerMgr := ledgermgmt.NewLedgerMgr(ledgerInitializer)
	defer ledgerMgr.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:")
	require.NoError(t, err)
	grpcServer := grpc.NewServer()

	cscc := newPeerConfiger(t, ledgerMgr, grpcServer, listener.Addr().String())

	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	channelID := "testjoinchainbysnapshot"
	sProp := validSignedProposal()
	sProp.Signature = sProp.ProposalBytes

	// set mocked ACLProcider and Stub
	mockACLProvider := cscc.aclProvider.(*mocks.ACLProvider)
	mockStub := &mocks.ChaincodeStub{}

	snapshotDir := ledgermgmttest.CreateSnapshotWithGenesisBlock(t, testDir, channelID, &peer.ConfigTxProcessor{})

	// successful path
	mockACLProvider.CheckACLReturns(nil)
	mockStub.GetSignedProposalReturns(sProp, nil)
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChainBySnapshot"), []byte(snapshotDir)})
	res := cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.OK), res.Status)

	// wait until ledger creation is done
	ledgerCreationDone := func() bool {
		resp := cscc.joinBySnapshotStatus()
		require.Equal(t, shim.OK, int(resp.Status))
		status := &pb.JoinBySnapshotStatus{}
		err := proto.Unmarshal(resp.Payload, status)
		require.NoError(t, err)
		return !status.InProgress
	}
	require.Eventually(t, ledgerCreationDone, time.Minute, time.Second)

	// verify get channels
	mockStub.GetArgsReturns([][]byte{[]byte(GetChannels)})
	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.OK), res.Status)

	cqr := &pb.ChannelQueryResponse{}
	require.NoError(t, proto.Unmarshal(res.Payload, cqr))
	require.Equal(t, 1, len(cqr.GetChannels()))
	require.Equal(t, channelID, cqr.GetChannels()[0].GetChannelId())

	// verify ledger is created
	lgr := cscc.peer.GetLedger(channelID)
	require.NotNil(t, lgr)
	bcInfo, err := lgr.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, uint64(1), bcInfo.Height)

	// error path due to missing argument
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChainBySnapshot")})
	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.ERROR), res.Status)
	require.Equal(t, "Incorrect number of arguments, 1", res.Message)

	// error path due to invalid snapshot dir (ledger creation fails in this case)
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChainBySnapshot"), []byte("invalid-snapshot")})
	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.ERROR), res.Status)
	require.Contains(t, res.Message, "no such file or directory")

	// error path due to CheckACL error
	mockACLProvider.CheckACLReturns(errors.New("Failed authorization"))
	mockStub.GetArgsReturns([][]byte{[]byte("JoinChainBySnapshot"), []byte(snapshotDir)})
	res = cscc.Invoke(mockStub)
	require.Equal(t, int32(shim.ERROR), res.Status)
	require.Contains(t, res.Message, "access denied for [JoinChainBySnapshot]")
}

func TestConfigerInvokeGetChannelConfig(t *testing.T) {
	testDir := t.TempDir()

	ledgerInitializer := ledgermgmttest.NewInitializer(testDir)
	ledgerInitializer.CustomTxProcessors = map[cb.HeaderType]ledger.CustomTxProcessor{
		cb.HeaderType_CONFIG: &peer.ConfigTxProcessor{},
	}
	ledgerMgr := ledgermgmt.NewLedgerMgr(ledgerInitializer)
	defer ledgerMgr.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:")
	require.NoError(t, err)
	grpcServer := grpc.NewServer()

	cscc := newPeerConfiger(t, ledgerMgr, grpcServer, listener.Addr().String())

	go grpcServer.Serve(listener)
	defer grpcServer.Stop()

	block, err := configtxtest.MakeGenesisBlock("test-channel-id")
	require.NoError(t, err)
	require.NoError(t,
		cscc.peer.CreateChannel("test-channel-id", block, cscc.deployedCCInfoProvider, cscc.legacyLifecycle, cscc.newLifecycle),
	)

	initMocks := func() (*mocks.ACLProvider, *mocks.ChaincodeStub) {
		mockACLProvider := cscc.aclProvider.(*mocks.ACLProvider)
		mockACLProvider.CheckACLReturns(nil)

		mockStub := &mocks.ChaincodeStub{}
		mockStub.GetSignedProposalReturns(validSignedProposal(), nil)
		mockStub.GetArgsReturns([][]byte{[]byte("GetChannelConfig"), []byte("test-channel-id")})
		return mockACLProvider, mockStub
	}

	t.Run("green-path", func(t *testing.T) {
		_, mockStub := initMocks()
		res := cscc.Invoke(mockStub)
		require.Equal(t, int32(shim.OK), res.Status)

		retrievedChannelConfig := &cb.Config{}
		require.NoError(t, proto.Unmarshal(res.Payload, retrievedChannelConfig))
		require.True(t,
			proto.Equal(
				channelConfigFromBlock(t, block),
				retrievedChannelConfig,
			),
		)
	})

	t.Run("acl-error", func(t *testing.T) {
		mockACLProvider, mockStub := initMocks()
		mockACLProvider.CheckACLReturns(errors.New("auth error"))
		res := cscc.Invoke(mockStub)
		require.Equal(t, int32(shim.ERROR), res.Status)
		require.Equal(t, res.Message, "access denied for [GetChannelConfig][test-channel-id]: auth error")
	})

	t.Run("missing-channel-name-error", func(t *testing.T) {
		_, mockStub := initMocks()
		mockStub.GetArgsReturns([][]byte{[]byte("GetChannelConfig")})
		res := cscc.Invoke(mockStub)
		require.Equal(t, int32(shim.ERROR), res.Status)
		require.Equal(t, "Incorrect number of arguments, 1", res.Message)
	})

	t.Run("nil-channel-name-error", func(t *testing.T) {
		_, mockStub := initMocks()
		mockStub.GetArgsReturns([][]byte{[]byte("GetChannelConfig"), {}})
		res := cscc.Invoke(mockStub)
		require.Equal(t, int32(shim.ERROR), res.Status)
		require.Equal(t, "empty channel name provided", res.Message)
	})

	t.Run("non-existing-channel-name-error", func(t *testing.T) {
		_, mockStub := initMocks()
		mockStub.GetArgsReturns([][]byte{[]byte("GetChannelConfig"), []byte("non-existing-channel")})
		res := cscc.Invoke(mockStub)
		require.Equal(t, int32(shim.ERROR), res.Status)
		require.Equal(t, "unknown channel ID, non-existing-channel", res.Message)
	})
}

func TestPeerConfiger_SubmittingOrdererGenesis(t *testing.T) {
	conf := genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile, configtest.GetDevConfigDir())
	conf.Application = nil
	cg, err := encoder.NewChannelGroup(conf)
	require.NoError(t, err)
	block := genesis.NewFactoryImpl(cg).Block("mytestchannelid")
	blockBytes := protoutil.MarshalOrPanic(block)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mockACLProvider := &mocks.ACLProvider{}
	cscc := &PeerConfiger{
		aclProvider: mockACLProvider,
		bccsp:       cryptoProvider,
	}
	mockStub := &mocks.ChaincodeStub{}
	// Failed path: wrong parameter type
	args := [][]byte{[]byte("JoinChain"), []byte(blockBytes)}
	mockStub.GetArgsReturns(args)
	mockStub.GetSignedProposalReturns(validSignedProposal(), nil)
	res := cscc.Invoke(mockStub)

	require.NotEqual(
		t,
		int32(shim.OK),
		res.Status,
		"invoke JoinChain should have failed with wrong genesis block",
	)
	require.Contains(t, res.Message, "missing Application configuration group")
}

func newPeerConfiger(t *testing.T, ledgerMgr *ledgermgmt.LedgerMgr, grpcServer *grpc.Server, peerEndpoint string) *PeerConfiger {
	viper.Set("chaincode.executetimeout", "3s")

	config := chaincode.GlobalConfig()
	config.StartupTimeout = 30 * time.Second

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	localMSP := mgmt.GetLocalMSP(cryptoProvider)
	signer, err := localMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	deserManager := peergossip.NewDeserializersManager(localMSP)

	messageCryptoService := peergossip.NewMCS(
		&mocks.ChannelPolicyManagerGetter{},
		signer,
		deserManager,
		cryptoProvider,
		nil,
	)
	secAdv := peergossip.NewSecurityAdvisor(deserManager)
	defaultSecureDialOpts := func() []grpc.DialOption {
		return []grpc.DialOption{grpc.WithInsecure()}
	}
	gossipConfig, err := gossip.GlobalConfig(peerEndpoint, nil)
	require.NoError(t, err)

	gossipService, err := service.New(
		signer,
		gossipmetrics.NewGossipMetrics(&disabled.Provider{}),
		peerEndpoint,
		grpcServer,
		messageCryptoService,
		secAdv,
		defaultSecureDialOpts,
		nil,
		gossipConfig,
		&service.ServiceConfig{},
		&privdata.PrivdataConfig{},
		&deliverservice.DeliverServiceConfig{
			ReConnectBackoffThreshold:   deliverservice.DefaultReConnectBackoffThreshold,
			ReconnectTotalTimeThreshold: deliverservice.DefaultReConnectTotalTimeThreshold,
		},
	)
	require.NoError(t, err)

	// setup cscc instance
	mockACLProvider := &mocks.ACLProvider{}
	cscc := &PeerConfiger{
		aclProvider: mockACLProvider,
		peer: &peer.Peer{
			StoreProvider:  &mocks.StoreProvider{},
			GossipService:  gossipService,
			LedgerMgr:      ledgerMgr,
			CryptoProvider: cryptoProvider,
		},
		bccsp: cryptoProvider,
	}

	return cscc
}

func mockConfigBlock() []byte {
	var blockBytes []byte = nil
	block, err := configtxtest.MakeGenesisBlock("mytestchannelid")
	if err == nil {
		blockBytes = protoutil.MarshalOrPanic(block)
	}
	return blockBytes
}

func validSignedProposal() *pb.SignedProposal {
	return &pb.SignedProposal{
		ProposalBytes: protoutil.MarshalOrPanic(&pb.Proposal{
			Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
				Input: protoutil.MarshalOrPanic(&pb.ChaincodeInvocationSpec{
					ChaincodeSpec: &pb.ChaincodeSpec{
						ChaincodeId: &pb.ChaincodeID{
							Name: "cscc",
						},
					},
				}),
			}),
		}),
	}
}

func channelConfigFromBlock(t *testing.T, configBlock *cb.Block) *cb.Config {
	envelopeConfig, err := protoutil.ExtractEnvelope(configBlock, 0)
	require.NoError(t, err)
	configEnv := &cb.ConfigEnvelope{}
	_, err = protoutil.UnmarshalEnvelopeOfType(envelopeConfig, cb.HeaderType_CONFIG, configEnv)
	require.NoError(t, err)
	return configEnv.Config
}
