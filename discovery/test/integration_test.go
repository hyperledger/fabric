/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	bccsp "github.com/hyperledger/fabric/bccsp/utils"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	policiesmocks "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/cclifecycle"
	lifecyclemocks "github.com/hyperledger/fabric/core/cclifecycle/mocks"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/discovery"
	disc "github.com/hyperledger/fabric/discovery/client"
	"github.com/hyperledger/fabric/discovery/endorsement"
	discsupport "github.com/hyperledger/fabric/discovery/support"
	discacl "github.com/hyperledger/fabric/discovery/support/acl"
	ccsupport "github.com/hyperledger/fabric/discovery/support/chaincode"
	"github.com/hyperledger/fabric/discovery/support/config"
	"github.com/hyperledger/fabric/discovery/support/mocks"
	"github.com/hyperledger/fabric/gossip/api"
	gcommon "github.com/hyperledger/fabric/gossip/common"
	gdisc "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	. "github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	msprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var (
	testPeers testPeerSet

	cryptogen, idemixgen, testdir string

	collectionConfigBytes = buildCollectionConfig(map[string][]*msprotos.MSPPrincipal{
		"col1":  {orgPrincipal("Org1MSP")},
		"col12": {orgPrincipal("Org1MSP"), orgPrincipal("Org2MSP")},
	})

	cc1Bytes = utils.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc1",
		Version: "1.0",
		Id:      []byte{42},
		Policy:  utils.MarshalOrPanic(policyFromString("AND('Org1MSP.member', 'Org1MSP.member')")),
	})

	cc2Bytes = utils.MarshalOrPanic(&ccprovider.ChaincodeData{
		Name:    "cc2",
		Version: "1.0",
		Id:      []byte{43},
		Policy:  utils.MarshalOrPanic(policyFromString("AND('Org1MSP.member', 'Org2MSP.member')")),
	})
)

func TestMain(m *testing.M) {
	var err error
	if err := buildBinaries(); err != nil {
		fmt.Printf("failed generating artifacts: +%v", err)
		return
	}

	testdir, err = generateChannelArtifacts()
	if err != nil {
		fmt.Printf("failed generating artifacts: +%v", err)
		return
	}

	peerDirPrefix := filepath.Join(testdir, "crypto-config", "peerOrganizations")
	testPeers = testPeerSet{
		newPeer(peerDirPrefix, "Org1MSP", 1, 0),
		newPeer(peerDirPrefix, "Org1MSP", 1, 1),
		newPeer(peerDirPrefix, "Org2MSP", 2, 0),
		newPeer(peerDirPrefix, "Org2MSP", 2, 1),
	}

	rc := m.Run()
	os.RemoveAll(testdir)
	gexec.CleanupBuildArtifacts()
	os.Exit(rc)
}

func TestGreenPath(t *testing.T) {
	t.Parallel()
	client, service := createClientAndService(t, testdir)
	defer service.Stop()
	defer client.conn.Close()

	service.lc.query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil)
	service.lc.query.On("GetState", "lscc", "cc2").Return(cc2Bytes, nil)
	service.lc.query.On("GetState", "lscc", "cc2~collection").Return(collectionConfigBytes, nil)

	ccWithCollection := &ChaincodeInterest{
		Chaincodes: []*ChaincodeCall{
			{Name: "cc2", CollectionNames: []string{"col12"}},
		},
	}
	cc2cc := &ChaincodeInterest{
		Chaincodes: []*ChaincodeCall{
			{Name: "cc1"}, {Name: "cc2"},
		},
	}

	// Send all queries
	req := disc.NewRequest().AddLocalPeersQuery().OfChannel("mychannel")
	col1 := &ChaincodeCall{Name: "cc2", CollectionNames: []string{"col1"}}
	nonExistentCollection := &ChaincodeCall{Name: "cc2", CollectionNames: []string{"col3"}}
	_ = nonExistentCollection
	req, err := req.AddPeersQuery().AddPeersQuery(col1).AddPeersQuery(nonExistentCollection).AddConfigQuery().AddEndorsersQuery(cc2cc, ccWithCollection)
	assert.NoError(t, err)
	res, err := client.Send(context.Background(), req, client.AuthInfo)
	assert.NoError(t, err)

	t.Run("Local peer query", func(t *testing.T) {
		returnedPeers, err := res.ForLocal().Peers()
		assert.NoError(t, err)
		assert.True(t, peersToTestPeers(returnedPeers).Equal(testPeers.withoutStateInfo()))
	})

	t.Run("Channel peer queries", func(t *testing.T) {
		returnedPeers, err := res.ForChannel("mychannel").Peers()
		assert.NoError(t, err)
		assert.True(t, peersToTestPeers(returnedPeers).Equal(testPeers))

		returnedPeers, err = res.ForChannel("mychannel").Peers(col1)
		assert.NoError(t, err)
		// Ensure only peers from Org1 are returned
		for _, p := range returnedPeers {
			assert.Equal(t, "Org1MSP", p.MSPID)
		}

		// Ensure that the client handles correctly errors returned from the server
		// in case of a bad request
		returnedPeers, err = res.ForChannel("mychannel").Peers(nonExistentCollection)
		assert.Equal(t, "collection col3 doesn't exist in collection config for chaincode cc2", err.Error())
	})

	t.Run("Endorser chaincode to chaincode", func(t *testing.T) {
		endorsers, err := res.ForChannel("mychannel").Endorsers(cc2cc.Chaincodes, disc.NoFilter)
		assert.NoError(t, err)
		endorsersByMSP := map[string][]string{}

		for _, endorser := range endorsers {
			endorsersByMSP[endorser.MSPID] = append(endorsersByMSP[endorser.MSPID], string(endorser.Identity))
		}
		// For cc2cc we expect 2 peers from Org1MSP and 1 from Org2MSP
		assert.Equal(t, 2, len(endorsersByMSP["Org1MSP"]))
		assert.Equal(t, 1, len(endorsersByMSP["Org2MSP"]))
	})

	t.Run("Endorser chaincode with collection", func(t *testing.T) {
		endorsers, err := res.ForChannel("mychannel").Endorsers(ccWithCollection.Chaincodes, disc.NoFilter)
		assert.NoError(t, err)

		endorsersByMSP := map[string][]string{}
		for _, endorser := range endorsers {
			endorsersByMSP[endorser.MSPID] = append(endorsersByMSP[endorser.MSPID], string(endorser.Identity))
		}
		assert.Equal(t, 1, len(endorsersByMSP["Org1MSP"]))
		assert.Equal(t, 1, len(endorsersByMSP["Org2MSP"]))
	})

	t.Run("Config query", func(t *testing.T) {
		conf, err := res.ForChannel("mychannel").Config()
		assert.NoError(t, err)
		// Ensure MSP Configs are exactly as they appear in the config block
		for mspID, mspConfig := range conf.Msps {
			expectedConfig := service.sup.mspConfigs[mspID]
			assert.Equal(t, expectedConfig, mspConfig)
		}
		// Ensure orderer endpoints are as they appear in the config block
		for mspID, endpoints := range conf.Orderers {
			assert.Equal(t, "OrdererMSP", mspID)
			endpoints := endpoints.Endpoint
			assert.Len(t, endpoints, 1)
			assert.Equal(t, "orderer.example.com", endpoints[0].Host)
			assert.Equal(t, uint32(7050), endpoints[0].Port)
		}
	})
}

func TestEndorsementComputationFailure(t *testing.T) {
	t.Parallel()
	client, service := createClientAndService(t, testdir)
	defer service.Stop()
	defer client.conn.Close()

	service.lc.query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil)
	service.lc.query.On("GetState", "lscc", "cc2").Return(cc2Bytes, nil)
	service.lc.query.On("GetState", "lscc", "cc2~collection").Return(collectionConfigBytes, nil)

	// Now test a collection query that should fail because cc2's endorsement policy is Org1MSP AND org2MSP
	// but the collection is configured only to have peers from Org1MSP
	ccWithCollection := &ChaincodeInterest{
		Chaincodes: []*ChaincodeCall{
			{Name: "cc2", CollectionNames: []string{"col1"}},
		},
	}
	req, err := disc.NewRequest().OfChannel("mychannel").AddEndorsersQuery(ccWithCollection)
	assert.NoError(t, err)
	res, err := client.Send(context.Background(), req, client.AuthInfo)
	assert.NoError(t, err)

	endorsers, err := res.ForChannel("mychannel").Endorsers(ccWithCollection.Chaincodes, disc.NoFilter)
	assert.Empty(t, endorsers)
	assert.Contains(t, err.Error(), "failed constructing descriptor")
}

func TestLedgerFailure(t *testing.T) {
	t.Parallel()
	client, service := createClientAndService(t, testdir)
	defer service.Stop()
	defer client.conn.Close()

	service.lc.query.On("GetState", "lscc", "cc1").Return(cc1Bytes, nil)
	service.lc.query.On("GetState", "lscc", "cc2").Return(nil, errors.New("IO error"))
	service.lc.query.On("GetState", "lscc", "cc12~collection").Return(collectionConfigBytes, nil)

	ccWithCollection := &ChaincodeInterest{
		Chaincodes: []*ChaincodeCall{
			{Name: "cc1"},
			{Name: "cc2", CollectionNames: []string{"col1"}},
		},
	}
	req, err := disc.NewRequest().OfChannel("mychannel").AddEndorsersQuery(ccWithCollection)
	assert.NoError(t, err)
	res, err := client.Send(context.Background(), req, client.AuthInfo)
	assert.NoError(t, err)

	endorsers, err := res.ForChannel("mychannel").Endorsers(ccWithCollection.Chaincodes, disc.NoFilter)
	assert.Empty(t, endorsers)
	assert.Contains(t, err.Error(), "failed constructing descriptor")
}

func TestRevocation(t *testing.T) {
	t.Parallel()
	client, service := createClientAndService(t, testdir)
	defer service.Stop()
	defer client.conn.Close()

	req := disc.NewRequest().OfChannel("mychannel").AddPeersQuery()
	res, err := client.Send(context.Background(), req, client.AuthInfo)
	assert.NoError(t, err)
	// Record number of times we deserialized the identity
	firstCount := atomic.LoadUint32(&service.sup.deserializeIdentityCount)

	// Do the same query again
	peers, err := res.ForChannel("mychannel").Peers()
	assert.NotEmpty(t, peers)
	assert.NoError(t, err)

	res, err = client.Send(context.Background(), req, client.AuthInfo)
	assert.NoError(t, err)
	// The amount of times deserializeIdentity was called should not have changed
	// because requests should have hit the cache
	secondCount := atomic.LoadUint32(&service.sup.deserializeIdentityCount)
	assert.Equal(t, firstCount, secondCount)

	// Now, increment the config sequence
	oldSeq := service.sup.sequenceWrapper.Sequence()
	v := &mocks.ConfigtxValidator{}
	v.SequenceReturns(oldSeq + 1)
	service.sup.sequenceWrapper.instance.Store(v)

	// Revoke all identities inside the MSP manager
	atomic.AddUint32(&service.sup.mspWrapper.blocks, uint32(1))

	// Send the query for the third time
	res, err = client.Send(context.Background(), req, client.AuthInfo)
	assert.NoError(t, err)
	// The cache should have been purged, thus deserializeIdentity should have been
	// called an additional time
	thirdCount := atomic.LoadUint32(&service.sup.deserializeIdentityCount)
	assert.NotEqual(t, thirdCount, secondCount)

	// We should be denied access
	peers, err = res.ForChannel("mychannel").Peers()
	assert.Empty(t, peers)
	assert.Contains(t, err.Error(), "access denied")
}

type client struct {
	*disc.Client
	*AuthInfo
	conn *grpc.ClientConn
}

func (c *client) newConnection() (*grpc.ClientConn, error) {
	return c.conn, nil
}

type mspWrapper struct {
	deserializeIdentityCount uint32
	msp.MSPManager
	mspConfigs map[string]*msprotos.FabricMSPConfig
	blocks     uint32
}

func (w *mspWrapper) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	atomic.AddUint32(&w.deserializeIdentityCount, 1)
	if atomic.LoadUint32(&w.blocks) == uint32(1) {
		return nil, errors.New("failed deserializing identity")
	}
	return w.MSPManager.DeserializeIdentity(serializedIdentity)
}

type lifeCycle struct {
	*cc.Lifecycle
	query *lifecyclemocks.Query
}

func newLifeCycle() *lifeCycle {
	enumerator := &lifecyclemocks.Enumerator{}
	enumerator.On("Enumerate").Return(nil, nil)
	lc, err := cc.NewLifeCycle(enumerator)
	if err != nil {
		panic(err)
	}
	qc := &lifecyclemocks.QueryCreator{}
	query := &lifecyclemocks.Query{}
	query.On("Done").Return()
	qc.On("NewQuery").Return(query, nil)
	_, err = lc.NewChannelSubscription("mychannel", qc)
	if err != nil {
		panic(err)
	}
	return &lifeCycle{
		Lifecycle: lc,
		query:     query,
	}
}

type principalEvaluator struct {
	*discacl.DiscoverySupport
	msp.MSPManager
}

type service struct {
	*grpc.Server
	lc  *lifeCycle
	sup *support
}

type support struct {
	discovery.Support
	*mspWrapper
	*sequenceWrapper
}

type sequenceWrapper struct {
	configtx.Validator
	instance atomic.Value
}

func (s *sequenceWrapper) Sequence() uint64 {
	return s.instance.Load().(*mocks.ConfigtxValidator).Sequence()
}

func createSupport(t *testing.T, dir string, lc *lifeCycle) *support {
	configs := make(map[string]*msprotos.FabricMSPConfig)
	mspMgr := createMSPManager(t, dir, configs)
	mspManagerWrapper := &mspWrapper{
		MSPManager: mspMgr,
		mspConfigs: configs,
	}
	mspMgr = mspManagerWrapper
	msps, _ := mspMgr.GetMSPs()
	org1MSP := msps["Org1MSP"]
	s := &sequenceWrapper{}
	v := &mocks.ConfigtxValidator{}
	v.SequenceReturns(1)
	s.instance.Store(v)
	chConfig := createChannelConfigGetter(s, mspMgr)
	polMgr := createPolicyManagerGetter(t, mspMgr)

	channelVerifier := discacl.NewChannelVerifier(policies.ChannelApplicationWriters, polMgr)

	org1Admin, err := cauthdsl.FromString("OR('Org1MSP.admin')")
	assert.NoError(t, err)
	org1AdminPolicy, _, err := cauthdsl.NewPolicyProvider(org1MSP).NewPolicy(utils.MarshalOrPanic(org1Admin))
	assert.NoError(t, err)
	acl := discacl.NewDiscoverySupport(channelVerifier, org1AdminPolicy, chConfig)

	gSup := &mocks.GossipSupport{}
	gSup.On("ChannelExists", "mychannel").Return(true)
	gSup.On("PeersOfChannel", gcommon.ChainID("mychannel")).Return(testPeers.toStateInfoSet())
	gSup.On("Peers").Return(testPeers.toMembershipSet())
	gSup.On("IdentityInfo").Return(testPeers.toIdentitySet())

	pe := &principalEvaluator{
		MSPManager: mspMgr,
		DiscoverySupport: &discacl.DiscoverySupport{
			ChannelConfigGetter: createChannelConfigGetter(s, mspMgr),
		},
	}

	ccSup := ccsupport.NewDiscoverySupport(lc)
	ea := endorsement.NewEndorsementAnalyzer(gSup, ccSup, pe, lc)

	fakeBlockGetter := &mocks.ConfigBlockGetter{}
	fakeBlockGetter.GetCurrConfigBlockReturns(createGenesisBlock(filepath.Join(dir, "crypto-config")))
	confSup := config.NewDiscoverySupport(fakeBlockGetter)
	return &support{
		Support:         discsupport.NewDiscoverySupport(acl, gSup, ea, confSup, acl),
		mspWrapper:      mspManagerWrapper,
		sequenceWrapper: s,
	}
}

func createClientAndService(t *testing.T, testdir string) (*client, *service) {
	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)

	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	assert.NoError(t, err)

	// Create a server on an ephemeral port
	gRPCServer, err := comm.NewGRPCServer("127.0.0.1:", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			Key:         serverKeyPair.Key,
			Certificate: serverKeyPair.Cert,
			UseTLS:      true,
		},
	})

	lc := newLifeCycle()
	sup := createSupport(t, testdir, lc)
	svc := discovery.NewService(discovery.Config{
		TLS:                          gRPCServer.TLSEnabled(),
		AuthCacheEnabled:             true,
		AuthCacheMaxSize:             10,
		AuthCachePurgeRetentionRatio: 0.5,
	}, sup)

	RegisterDiscoveryServer(gRPCServer.Server(), svc)

	assert.NoError(t, err)
	go gRPCServer.Start()

	clientKeyPair, err := ca.NewClientCertKeyPair()
	assert.NoError(t, err)

	signer := createSigner(t)

	authInfo := &AuthInfo{
		ClientIdentity:    signer.Creator,
		ClientTlsCertHash: util.ComputeSHA256(clientKeyPair.TLSCert.Raw),
	}

	dialer, err := comm.NewGRPCClient(comm.ClientConfig{
		Timeout: time.Second * 3,
		SecOpts: &comm.SecureOptions{
			UseTLS:        true,
			Certificate:   clientKeyPair.Cert,
			Key:           clientKeyPair.Key,
			ServerRootCAs: [][]byte{ca.CertBytes()},
		},
	})
	assert.NoError(t, err)

	conn, err := dialer.NewConnection(gRPCServer.Address(), "")
	assert.NoError(t, err)

	wrapperClient := &client{AuthInfo: authInfo, conn: conn}
	var signerCacheSize uint = 10
	c := disc.NewClient(wrapperClient.newConnection, signer.Sign, signerCacheSize)
	wrapperClient.Client = c
	service := &service{Server: gRPCServer.Server(), lc: lc, sup: sup}
	return wrapperClient, service
}

func createSigner(t *testing.T) *signer {
	identityDir := filepath.Join(testdir, "crypto-config", "peerOrganizations", "org1.example.com", "users", "Admin@org1.example.com", "msp")
	certPath := filepath.Join(identityDir, "signcerts", "Admin@org1.example.com-cert.pem")
	keyPath := filepath.Join(identityDir, "keystore")
	keys, err := ioutil.ReadDir(keyPath)
	assert.NoError(t, err)
	assert.Len(t, keys, 1)
	keyPath = filepath.Join(keyPath, keys[0].Name())
	signer, err := newSigner("Org1MSP", certPath, keyPath)
	assert.NoError(t, err)
	return signer
}

func createMSP(t *testing.T, dir, mspID string) (msp.MSP, *msprotos.FabricMSPConfig) {
	channelMSP, err := msp.New(&msp.BCCSPNewOpts{
		NewBaseOpts: msp.NewBaseOpts{Version: msp.MSPv1_1},
	})
	assert.NoError(t, err)

	mspConf, err := msp.GetVerifyingMspConfig(dir, mspID, "bccsp")
	assert.NoError(t, err)

	fabConf := &msprotos.FabricMSPConfig{}
	proto.Unmarshal(mspConf.Config, fabConf)

	channelMSP.Setup(mspConf)
	return channelMSP, fabConf
}

func createMSPManager(t *testing.T, dir string, configs map[string]*msprotos.FabricMSPConfig) msp.MSPManager {
	var mspsOfChannel []msp.MSP
	for _, mspConf := range []struct {
		mspID  string
		mspDir string
	}{
		{
			mspID:  "Org1MSP",
			mspDir: filepath.Join(dir, "crypto-config", "peerOrganizations", "org1.example.com", "msp"),
		},
		{
			mspID:  "Org2MSP",
			mspDir: filepath.Join(dir, "crypto-config", "peerOrganizations", "org2.example.com", "msp"),
		},
		{
			mspID:  "OrdererMSP",
			mspDir: filepath.Join(dir, "crypto-config", "ordererOrganizations", "example.com", "msp"),
		},
	} {
		mspInstance, conf := createMSP(t, mspConf.mspDir, mspConf.mspID)
		configs[mspConf.mspID] = conf
		mspsOfChannel = append(mspsOfChannel, mspInstance)
	}

	mspMgr := msp.NewMSPManager()
	mspMgr.Setup(mspsOfChannel)
	return mspMgr
}

func createChannelConfigGetter(s *sequenceWrapper, mspMgr msp.MSPManager) discacl.ChannelConfigGetter {
	resources := &mocks.Resources{}
	resources.ConfigtxValidatorReturns(s)
	resources.MSPManagerReturns(mspMgr)
	chConfig := &mocks.ChanConfig{}
	chConfig.GetChannelConfigReturns(resources)
	return chConfig
}

func createPolicyManagerGetter(t *testing.T, mspMgr msp.MSPManager) *mocks.ChannelPolicyManagerGetter {
	org1Org2Members, err := cauthdsl.FromString("OR('Org1MSP.client', 'Org2MSP.client')")
	assert.NoError(t, err)
	org1Org2MembersPolicy, _, err := cauthdsl.NewPolicyProvider(mspMgr).NewPolicy(utils.MarshalOrPanic(org1Org2Members))
	assert.NoError(t, err)
	_ = org1Org2MembersPolicy
	polMgr := &mocks.ChannelPolicyManagerGetter{}
	policyMgr := &policiesmocks.Manager{
		PolicyMap: map[string]policies.Policy{
			policies.ChannelApplicationWriters: org1Org2MembersPolicy,
		},
	}
	polMgr.On("Manager", "mychannel").Return(policyMgr, false)
	return polMgr
}

func buildBinaries() error {
	var err error
	cryptogen, err = gexec.Build("github.com/hyperledger/fabric/common/tools/cryptogen")
	if err != nil {
		return errors.WithStack(err)
	}

	idemixgen, err = gexec.Build("github.com/hyperledger/fabric/common/tools/idemixgen")
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func generateChannelArtifacts() (string, error) {
	dir, err := ioutil.TempDir("", "TestMSPIDMapping")
	if err != nil {
		return "", errors.WithStack(err)
	}
	cryptoConfigDir := filepath.Join(dir, "crypto-config")
	args := []string{
		"generate",
		fmt.Sprintf("--output=%s", cryptoConfigDir),
		fmt.Sprintf("--config=%s", filepath.Join("..", "..", "examples", "e2e_cli", "crypto-config.yaml")),
	}
	b, err := exec.Command(cryptogen, args...).CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, string(b))
	}

	idemixConfigDir := filepath.Join(dir, "crypto-config", "idemix")
	b, err = exec.Command(idemixgen, "ca-keygen", fmt.Sprintf("--output=%s", idemixConfigDir)).CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, string(b))
	}
	return dir, nil
}

func createGenesisBlock(cryptoConfigDir string) *common.Block {
	appConfig := genesisconfig.Load("TwoOrgsChannel", filepath.Join("..", "..", "examples", "e2e_cli"))
	ordererConfig := genesisconfig.Load("TwoOrgsOrdererGenesis", filepath.Join("..", "..", "examples", "e2e_cli"))
	// Glue the two parts together, without loss of generality - to the application parts
	appConfig.Orderer = ordererConfig.Orderer
	channelConfig := appConfig

	idemixConfigDir := filepath.Join(cryptoConfigDir, "idemix")
	// Override the MSP directories
	for _, org := range channelConfig.Orderer.Organizations {
		org.MSPDir = filepath.Join(cryptoConfigDir, "ordererOrganizations", "example.com", "msp")
	}
	for i, org := range channelConfig.Application.Organizations {
		if org.MSPType != "bccsp" {
			org.MSPDir = filepath.Join(idemixConfigDir)
			continue
		}
		org.MSPDir = filepath.Join(cryptoConfigDir, "peerOrganizations", fmt.Sprintf("org%d.example.com", i+1), "msp")
	}

	channelGenerator := encoder.New(channelConfig)
	return channelGenerator.GenesisBlockForChannel("mychannel")
}

type testPeer struct {
	mspID        string
	identity     []byte
	stateInfoMsg gdisc.NetworkMember
	aliveMsg     gdisc.NetworkMember
}

func (tp testPeer) Equal(other testPeer) bool {
	if tp.mspID != other.mspID || !bytes.Equal(tp.identity, other.identity) {
		return false
	}
	if tp.aliveMsg.Endpoint != other.aliveMsg.Endpoint || !bytes.Equal(tp.aliveMsg.PKIid, other.aliveMsg.PKIid) {
		return false
	}
	if !proto.Equal(tp.aliveMsg.Envelope, other.aliveMsg.Envelope) {
		return false
	}
	if !bytes.Equal(tp.stateInfoMsg.PKIid, other.stateInfoMsg.PKIid) {
		return false
	}
	if !proto.Equal(tp.stateInfoMsg.Envelope, other.stateInfoMsg.Envelope) {
		return false
	}
	if !proto.Equal(tp.stateInfoMsg.Properties, other.stateInfoMsg.Properties) {
		return false
	}
	return true
}

type testPeerSet []*testPeer

func (ps testPeerSet) withoutStateInfo() testPeerSet {
	var res testPeerSet
	for _, p := range ps {
		peer := *p
		peer.stateInfoMsg = gdisc.NetworkMember{}
		res = append(res, &peer)
	}
	return res
}

func (ps testPeerSet) toStateInfoSet() gdisc.Members {
	var members gdisc.Members
	for _, p := range ps {
		members = append(members, p.stateInfoMsg)
	}
	return members
}

func (ps testPeerSet) toMembershipSet() gdisc.Members {
	var members gdisc.Members
	for _, p := range ps {
		members = append(members, p.aliveMsg)
	}
	return members
}

func (ps testPeerSet) toIdentitySet() api.PeerIdentitySet {
	var idSet api.PeerIdentitySet
	for _, p := range ps {
		idSet = append(idSet, api.PeerIdentityInfo{
			Identity:     p.identity,
			PKIId:        p.aliveMsg.PKIid,
			Organization: api.OrgIdentityType(p.mspID),
		})
	}
	return idSet
}

func (ps testPeerSet) Equal(that testPeerSet) bool {
	this := ps
	return this.SubsetEqual(that) && that.SubsetEqual(this)
}

func (ps testPeerSet) SubsetEqual(that testPeerSet) bool {
	this := ps
	for _, p := range this {
		if !that.Contains(p) {
			return false
		}
	}
	return true
}

func (ps testPeerSet) Contains(peer *testPeer) bool {
	for _, p := range ps {
		if peer.Equal(*p) {
			return true
		}
	}
	return false
}

func peersToTestPeers(peers []*disc.Peer) testPeerSet {
	var res testPeerSet
	for _, p := range peers {
		pkiID := gcommon.PKIidType(hex.EncodeToString(util.ComputeSHA256(p.Identity)))
		var stateInfoMember gdisc.NetworkMember
		if p.StateInfoMessage != nil {
			stateInfo, _ := p.StateInfoMessage.ToGossipMessage()
			stateInfoMember = gdisc.NetworkMember{
				PKIid:      pkiID,
				Envelope:   p.StateInfoMessage.Envelope,
				Properties: stateInfo.GetStateInfo().Properties,
			}
		}

		tp := &testPeer{
			mspID:    p.MSPID,
			identity: p.Identity,
			aliveMsg: gdisc.NetworkMember{
				PKIid:    pkiID,
				Endpoint: string(pkiID),
				Envelope: p.AliveMessage.Envelope,
			},
			stateInfoMsg: stateInfoMember,
		}
		res = append(res, tp)
	}
	return res
}

func newPeer(dir, mspID string, org, id int) *testPeer {
	peerStr := fmt.Sprintf("peer%d.org%d.example.com", id, org)
	certFile := filepath.Join(dir, fmt.Sprintf("org%d.example.com", org),
		"peers", peerStr, "msp", "signcerts", fmt.Sprintf("%s-cert.pem", peerStr))
	certBytes, err := ioutil.ReadFile(certFile)
	if err != nil {
		panic(fmt.Sprintf("failed reading file %s: %v", certFile, err))
	}
	sID := &msprotos.SerializedIdentity{
		Mspid:   mspID,
		IdBytes: certBytes,
	}
	identityBytes := utils.MarshalOrPanic(sID)
	pkiID := gcommon.PKIidType(hex.EncodeToString(util.ComputeSHA256(identityBytes)))
	return &testPeer{
		mspID:        mspID,
		identity:     identityBytes,
		aliveMsg:     aliveMsg(pkiID),
		stateInfoMsg: stateInfoMsg(pkiID),
	}
}

func stateInfoMsg(pkiID gcommon.PKIidType) gdisc.NetworkMember {
	si := &gossip.StateInfo{
		Properties: &gossip.Properties{
			LedgerHeight: 100,
			Chaincodes: []*gossip.Chaincode{
				{
					Name:     "cc1",
					Version:  "1.0",
					Metadata: []byte{42},
				},
				{
					Name:     "cc2",
					Version:  "1.0",
					Metadata: []byte{43},
				},
			},
		},
		PkiId:     pkiID,
		Timestamp: &gossip.PeerTime{},
	}
	gm := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: si,
		},
	}
	sm, _ := gm.NoopSign()
	return gdisc.NetworkMember{
		Properties: si.Properties,
		PKIid:      pkiID,
		Envelope:   sm.Envelope,
	}
}

func aliveMsg(pkiID gcommon.PKIidType) gdisc.NetworkMember {
	am := &gossip.AliveMessage{
		Membership: &gossip.Member{
			PkiId:    pkiID,
			Endpoint: string(pkiID),
		},
		Timestamp: &gossip.PeerTime{},
	}
	gm := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_AliveMsg{
			AliveMsg: am,
		},
	}
	sm, _ := gm.NoopSign()
	return gdisc.NetworkMember{
		PKIid:    pkiID,
		Endpoint: string(pkiID),
		Envelope: sm.Envelope,
	}
}

func buildCollectionConfig(col2principals map[string][]*msprotos.MSPPrincipal) []byte {
	collections := &common.CollectionConfigPackage{}
	for col, principals := range col2principals {
		collections.Config = append(collections.Config, &common.CollectionConfig{
			Payload: &common.CollectionConfig_StaticCollectionConfig{
				StaticCollectionConfig: &common.StaticCollectionConfig{
					Name: col,
					MemberOrgsPolicy: &common.CollectionPolicyConfig{
						Payload: &common.CollectionPolicyConfig_SignaturePolicy{
							SignaturePolicy: &common.SignaturePolicyEnvelope{
								Identities: principals,
							},
						},
					},
				},
			},
		})
	}
	return utils.MarshalOrPanic(collections)
}

func orgPrincipal(mspID string) *msprotos.MSPPrincipal {
	return &msprotos.MSPPrincipal{
		PrincipalClassification: msprotos.MSPPrincipal_ROLE,
		Principal: utils.MarshalOrPanic(&msprotos.MSPRole{
			MspIdentifier: mspID,
			Role:          msprotos.MSPRole_MEMBER,
		}),
	}
}

func policyFromString(s string) *common.SignaturePolicyEnvelope {
	p, err := cauthdsl.FromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

type signer struct {
	key     *ecdsa.PrivateKey
	Creator []byte
}

func newSigner(msp, certPath, keyPath string) (*signer, error) {
	sId, err := serializeIdentity(certPath, msp)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	key, err := loadPrivateKey(keyPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &signer{
		Creator: sId,
		key:     key,
	}, nil
}

func serializeIdentity(clientCert string, mspID string) ([]byte, error) {
	b, err := ioutil.ReadFile(clientCert)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	sId := &msprotos.SerializedIdentity{
		Mspid:   mspID,
		IdBytes: b,
	}
	return utils.MarshalOrPanic(sId), nil
}

func (si *signer) Sign(msg []byte) ([]byte, error) {
	digest := util.ComputeSHA256(msg)
	return signECDSA(si.key, digest)
}

func loadPrivateKey(file string) (*ecdsa.PrivateKey, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bl, _ := pem.Decode(b)
	key, err := x509.ParsePKCS8PrivateKey(bl.Bytes)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return key.(*ecdsa.PrivateKey), nil
}

func signECDSA(k *ecdsa.PrivateKey, digest []byte) (signature []byte, err error) {
	r, s, err := ecdsa.Sign(rand.Reader, k, digest)
	if err != nil {
		return nil, err
	}

	s, _, err = bccsp.ToLowS(&k.PublicKey, s)
	if err != nil {
		return nil, err
	}
	return bccsp.MarshalECDSASignature(r, s)
}
