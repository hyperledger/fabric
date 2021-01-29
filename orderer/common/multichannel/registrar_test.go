/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel/mocks"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/resources.go --fake-name Resources . resources

type resources interface {
	channelconfig.Resources
}

//go:generate counterfeiter -o mocks/orderer_config.go --fake-name OrdererConfig . ordererConfig

type ordererConfig interface {
	channelconfig.Orderer
}

//go:generate counterfeiter -o mocks/orderer_capabilities.go --fake-name OrdererCapabilities . ordererCapabilities

type ordererCapabilities interface {
	channelconfig.OrdererCapabilities
}

//go:generate counterfeiter -o mocks/channel_config.go --fake-name ChannelConfig . channelConfig

type channelConfig interface {
	channelconfig.Channel
}

//go:generate counterfeiter -o mocks/channel_capabilities.go --fake-name ChannelCapabilities . channelCapabilities

type channelCapabilities interface {
	channelconfig.ChannelCapabilities
}

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

//go:generate counterfeiter -o mocks/consenter.go --fake-name Consenter . consenter
type consenter interface {
	consensus.Consenter
	consensus.ClusterConsenter
}

func mockCrypto() *mocks.SignerSerializer {
	return &mocks.SignerSerializer{}
}

func newFactory(dir string) blockledger.Factory {
	rlf, err := fileledger.New(dir, &disabled.Provider{})
	if err != nil {
		panic(err)
	}

	return rlf
}

func newLedgerAndFactory(dir string, chainID string, genesisBlockSys *cb.Block) (blockledger.Factory, blockledger.ReadWriter) {
	rlf := newFactory(dir)
	rl := newLedger(rlf, chainID, genesisBlockSys)
	return rlf, rl
}

func newLedger(rlf blockledger.Factory, chainID string, genesisBlockSys *cb.Block) blockledger.ReadWriter {
	rl, err := rlf.GetOrCreate(chainID)
	if err != nil {
		panic(err)
	}

	if genesisBlockSys != nil {
		err = rl.Append(genesisBlockSys)
		if err != nil {
			panic(err)
		}
	}
	return rl
}

func testMessageOrderAndRetrieval(maxMessageCount uint32, chainID string, chainSupport *ChainSupport, lr blockledger.ReadWriter, t *testing.T) {
	messages := make([]*cb.Envelope, maxMessageCount)
	for i := uint32(0); i < maxMessageCount; i++ {
		messages[i] = makeNormalTx(chainID, int(i))
	}
	for _, message := range messages {
		chainSupport.Order(message, 0)
	}
	it, _ := lr.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
	defer it.Close()
	block, status := it.Next()
	require.Equal(t, cb.Status_SUCCESS, status, "Could not retrieve block")
	for i := uint32(0); i < maxMessageCount; i++ {
		require.True(t, proto.Equal(messages[i], protoutil.ExtractEnvelopeOrPanic(block, int(i))), "Block contents wrong at index %d", i)
	}
}

func TestConfigTx(t *testing.T) {
	// system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	// Tests for a normal channel which contains 3 config transactions and other
	// normal transactions to make sure the right one returned
	t.Run("GetConfigTx - ok", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		_, rl := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)
		for i := 0; i < 5; i++ {
			rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx("testchannelid", i)}))
		}
		rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{makeConfigTx("testchannelid", 5)}))
		ctx := makeConfigTx("testchannelid", 6)
		rl.Append(blockledger.CreateNextBlock(rl, []*cb.Envelope{ctx}))

		// block with LAST_CONFIG metadata in SIGNATURES field
		block := blockledger.CreateNextBlock(rl, []*cb.Envelope{makeNormalTx("testchannelid", 7)})
		blockSignatureValue := protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			LastConfig: &cb.LastConfig{Index: 7},
		})
		block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{Value: blockSignatureValue})
		rl.Append(block)

		pctx := configTx(rl)
		require.True(t, proto.Equal(pctx, ctx), "Did not select most recent config transaction")
	})
}

func TestNewRegistrar(t *testing.T) {
	//system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	// This test checks to make sure the orderer can come up if it cannot find any chains
	t.Run("No chains", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, err := fileledger.New(tmpdir, &disabled.Provider{})
		require.NoError(t, err)

		consenters := map[string]consensus.Consenter{"etcdraft": &mocks.Consenter{}}

		var manager *Registrar
		require.NotPanics(t, func() {
			manager = NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
			manager.Initialize(consenters)
		}, "Should not panic when starting without a system channel")
		require.NotNil(t, manager)
		list := manager.ChannelList()
		require.Equal(t, types.ChannelList{}, list)
		info, err := manager.ChannelInfo("my-channel")
		require.EqualError(t, err, types.ErrChannelNotExist.Error())
		require.Equal(t, types.ChannelInfo{}, info)
	})

	// This test checks to make sure that the orderer refuses to come up if there are multiple system channels
	t.Run("Multiple system chains - failure", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, err := fileledger.New(tmpdir, &disabled.Provider{})
		require.NoError(t, err)

		for _, id := range []string{"foo", "bar"} {
			rl, err := lf.GetOrCreate(id)
			require.NoError(t, err)

			err = rl.Append(encoder.New(confSys).GenesisBlockForChannel(id))
			require.NoError(t, err)
		}

		consenter := &mocks.Consenter{}
		consenter.HandleChainCalls(handleChain)
		consenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: consenter}

		require.Panics(t, func() {
			NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil).Initialize(consenters)
		}, "Two system channels should have caused panic")
	})

	// This test essentially brings the entire system up and is ultimately what main.go will replicate
	t.Run("Correct flow with system channel", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, rl := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

		consenter := &mocks.Consenter{}
		consenter.HandleChainCalls(handleChain)
		consenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: consenter}

		manager := NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		manager.Initialize(consenters)

		chainSupport := manager.GetChain("Fake")
		require.Nilf(t, chainSupport, "Should not have found a chain that was not created")

		chainSupport = manager.GetChain("testchannelid")
		require.NotNilf(t, chainSupport, "Should have gotten chain which was initialized by ledger")

		list := manager.ChannelList()
		require.NotNil(t, list.SystemChannel)

		require.Equal(
			t,
			types.ChannelList{
				SystemChannel: &types.ChannelInfoShort{Name: "testchannelid", URL: ""},
				Channels:      nil},
			list,
		)

		info, err := manager.ChannelInfo("testchannelid")
		require.NoError(t, err)
		require.Equal(t,
			types.ChannelInfo{Name: "testchannelid", URL: "", ConsensusRelation: "other", Status: "active", Height: 1},
			info,
		)

		testMessageOrderAndRetrieval(confSys.Orderer.BatchSize.MaxMessageCount, "testchannelid", chainSupport, rl, t)
	})
}

func TestRegistrar_Initialize(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)

	dialer := &cluster.PredicateDialer{
		Config: comm.ClientConfig{
			SecOpts: comm.SecureOptions{
				Certificate: tlsCA.CertBytes(),
			},
		},
	}

	config := localconfig.TopLevel{
		General: localconfig.General{
			BootstrapMethod: "none",
			GenesisFile:     "",
			Cluster: localconfig.Cluster{
				ReplicationBufferSize:   1,
				ReplicationPullTimeout:  time.Microsecond,
				ReplicationRetryTimeout: time.Microsecond,
				ReplicationMaxRetries:   2,
			},
		},
		ChannelParticipation: localconfig.ChannelParticipation{
			Enabled: true,
		},
	}

	confAppRaft := genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
	confAppRaft.Consortiums = nil
	confAppRaft.Consortium = ""
	certDir, err := ioutil.TempDir("", "registrar_test-")
	require.NoError(t, err)
	defer os.RemoveAll(certDir)
	generateCertificates(t, confAppRaft, tlsCA, certDir)
	bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
	require.NoError(t, err, "cannot create bootstrapper")
	genesisBlockAppRaft := bootstrapper.GenesisBlockForChannel("my-raft-channel")
	require.NotNil(t, genesisBlockAppRaft)

	confSysRaft := genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
	confSysRaft.Orderer.EtcdRaft.Consenters = confAppRaft.Orderer.EtcdRaft.Consenters
	bootstrapper, err = encoder.NewBootstrapper(confSysRaft)
	require.NoError(t, err, "cannot create bootstrapper")
	genesisBlockSysRaft := bootstrapper.GenesisBlockForChannel("my-sys-channel")
	require.NotNil(t, genesisBlockSysRaft)

	consenter := &mocks.Consenter{}
	consenter.HandleChainCalls(handleChainCluster)
	consenters := map[string]consensus.Consenter{confAppRaft.Orderer.OrdererType: consenter}

	// This test essentially brings the entire system up and is ultimately what main.go will replicate
	t.Run("Correct flow with system channel - etcdraft.Chain", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, _ := newLedgerAndFactory(tmpdir, "my-sys-channel", genesisBlockSysRaft)

		manager := NewRegistrar(config, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		manager.Initialize(consenters)

		chainSupport := manager.GetChain("Fake")
		require.Nilf(t, chainSupport, "Should not have found a chain that was not created")

		chainSupport = manager.GetChain("my-sys-channel")
		require.NotNilf(t, chainSupport, "Should have gotten chain which was initialized by ledger")

		list := manager.ChannelList()
		require.NotNil(t, list.SystemChannel)

		require.Equal(t,
			types.ChannelList{
				SystemChannel: &types.ChannelInfoShort{Name: "my-sys-channel", URL: ""},
				Channels:      nil},
			list,
		)

		info, err := manager.ChannelInfo("my-sys-channel")
		require.NoError(t, err)
		require.Equal(t,
			types.ChannelInfo{Name: "my-sys-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 1},
			info,
		)
	})

	t.Run("Correct flow without system channel - etcdraft.Chain", func(t *testing.T) {
		//TODO
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, _ := newLedgerAndFactory(tmpdir, "my-raft-channel", genesisBlockAppRaft)

		consenter.IsChannelMemberReturns(true, nil)

		manager := NewRegistrar(config, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		manager.Initialize(consenters)

		chainSupport := manager.GetChain("not-there")
		require.Nilf(t, chainSupport, "Should not have found a chain that was not created")

		chainSupport = manager.GetChain("my-raft-channel")
		require.NotNilf(t, chainSupport, "Should have gotten chain which was initialized by ledger")

		list := manager.ChannelList()
		require.Nil(t, list.SystemChannel)

		require.Equal(
			t,
			types.ChannelList{
				SystemChannel: nil,
				Channels:      []types.ChannelInfoShort{{Name: "my-raft-channel", URL: ""}},
			},
			list,
		)

		info, err := manager.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t,
			types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 1},
			info,
		)
	})

	t.Run("Correct flow without system channel - follower.Chain", func(t *testing.T) {
		//TODO
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, _ := newLedgerAndFactory(tmpdir, "my-raft-channel", genesisBlockAppRaft)

		consenter.IsChannelMemberReturns(false, nil)

		manager := NewRegistrar(config, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		manager.Initialize(consenters)

		fChain := manager.GetFollower("not-there")
		require.Nil(t, fChain, "Should not have found a follower that was not created")

		list := manager.ChannelList()
		require.Nil(t, list.SystemChannel)

		require.Equal(
			t,
			types.ChannelList{
				SystemChannel: nil,
				Channels:      []types.ChannelInfoShort{{Name: "my-raft-channel", URL: ""}},
			},
			list,
		)

		info, err := manager.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t,
			types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "follower", Status: "active", Height: 1},
			info,
		)

		fChain = manager.GetFollower("my-raft-channel")
		require.NotNil(t, fChain, "Should have gotten follower which was initialized by ledger")
		fChain.Halt()
	})

	t.Run("Correct flow without system channel - follower.Chain with join block", func(t *testing.T) {
		//TODO
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		config.FileLedger = localconfig.FileLedger{Location: tmpdir}

		lf, _ := newLedgerAndFactory(tmpdir, "my-raft-channel", genesisBlockAppRaft)

		consenter.IsChannelMemberReturns(false, nil)

		manager := NewRegistrar(config, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)

		// Emulate saved join-block from before the restart
		joinBlockAppRaft := protoutil.UnmarshalBlockOrPanic(protoutil.MarshalOrPanic(genesisBlockAppRaft))
		joinBlockAppRaft.Header.Number = 10
		manager.joinBlockFileRepo.Save("my-raft-channel", protoutil.MarshalOrPanic(joinBlockAppRaft))

		manager.Initialize(consenters)

		fChain := manager.GetFollower("not-there")
		require.Nil(t, fChain, "Should not have found a follower that was not created")

		list := manager.ChannelList()
		require.Nil(t, list.SystemChannel)

		require.Equal(t,
			types.ChannelList{
				SystemChannel: nil,
				Channels:      []types.ChannelInfoShort{{Name: "my-raft-channel", URL: ""}},
			},
			list,
		)

		info, err := manager.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t,
			types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "follower", Status: "onboarding", Height: 1},
			info,
		)

		fChain = manager.GetFollower("my-raft-channel")
		require.NotNil(t, fChain, "Should have gotten follower which was initialized by ledger")
		fChain.Halt()
	})
}

func TestNewRegistrarWithFileRepo(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	consenter := &mocks.Consenter{}
	consenter.HandleChainCalls(handleChain)
	consenter.IsChannelMemberReturns(true, nil)
	consenters := map[string]consensus.Consenter{"etcdraft": consenter}

	t.Run("Correct flow with valid file repo dir, one existing channel, two joinblocks", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		tlsCA, err := tlsgen.NewCA()
		require.NoError(t, err)

		confAppRaft := genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
		confAppRaft.Consortiums = nil
		confAppRaft.Consortium = ""

		generateCertificates(t, confAppRaft, tlsCA, tmpdir)

		bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
		require.NoError(t, err, "cannot create bootstrapper")

		genesisBlockAppRaft := bootstrapper.GenesisBlockForChannel("my-cft-channel")
		require.NotNil(t, genesisBlockAppRaft)
		genesisBlockAppRaft2 := bootstrapper.GenesisBlockForChannel("my-other-cft-channel")
		require.NotNil(t, genesisBlockAppRaft2)

		// create joinblock file repo directory with two existing joinblocks
		createJoinBlockFileRepoDirWithBlocks(
			t,
			tmpdir,
			&joinBlock{
				channel: "my-cft-channel",
				block:   genesisBlockAppRaft,
			},
			&joinBlock{
				channel: "my-other-cft-channel",
				block:   genesisBlockAppRaft2,
			},
		)

		// create one existing channel
		genesisBlockAppExisting := bootstrapper.GenesisBlockForChannel("my-existing-channel")
		require.NotNil(t, genesisBlockAppExisting)
		lf, _ := newLedgerAndFactory(tmpdir, "my-existing-channel", genesisBlockAppExisting)
		require.NoError(t, err)

		// create the ledger for one of the channels with a joinblock in the file repo
		_, err = lf.GetOrCreate("my-other-cft-channel")
		require.NoError(t, err)

		config := localconfig.TopLevel{
			ChannelParticipation: localconfig.ChannelParticipation{Enabled: true},
			FileLedger:           localconfig.FileLedger{Location: tmpdir},
		}
		var manager *Registrar
		require.NotPanics(t, func() {
			manager = NewRegistrar(config, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
			manager.Initialize(consenters)
		}, "Should not panic when file repo dir exists and is read writable")
		require.NotNil(t, manager)
		require.NotNil(t, manager.joinBlockFileRepo)
		require.DirExists(t, filepath.Join(tmpdir, "pendingops"))

		list := manager.ChannelList()
		require.Nil(t, list.SystemChannel)
		require.ElementsMatch(
			t,
			[]types.ChannelInfoShort{
				{Name: "my-cft-channel", URL: ""},
				{Name: "my-other-cft-channel", URL: ""},
				{Name: "my-existing-channel", URL: ""},
			},
			list.Channels,
		)
	})
}

type joinBlock struct {
	channel string
	block   *cb.Block
}

func createJoinBlockFileRepoDirWithBlocks(t *testing.T, tmpdir string, joinBlocks ...*joinBlock) {
	joinBlockRepoPath := filepath.Join(tmpdir, "pendingops", "join")
	err := os.MkdirAll(joinBlockRepoPath, 0755)
	require.NoError(t, err)
	for _, jb := range joinBlocks {
		blockBytes, err := proto.Marshal(jb.block)
		require.NoError(t, err)
		err = ioutil.WriteFile(filepath.Join(joinBlockRepoPath, fmt.Sprintf("%s.join", jb.channel)), blockBytes, 0600)
		require.NoError(t, err)
	}
}

func TestCreateChain(t *testing.T) {
	//system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("Create chain", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, _ := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

		consenter := &mocks.Consenter{}
		consenter.HandleChainCalls(handleChainCluster)
		consenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: consenter}

		manager := NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		manager.Initialize(consenters)

		ledger, err := lf.GetOrCreate("mychannel")
		require.NoError(t, err)

		genesisBlock := encoder.New(confSys).GenesisBlockForChannel("mychannel")
		ledger.Append(genesisBlock)

		// Before creating the chain, it doesn't exist
		require.Nil(t, manager.GetChain("mychannel"))
		// After creating the chain, it exists
		manager.CreateChain("mychannel")
		chain := manager.GetChain("mychannel")
		require.NotNil(t, chain)

		list := manager.ChannelList()
		require.Equal(
			t,
			types.ChannelList{
				SystemChannel: &types.ChannelInfoShort{Name: "testchannelid", URL: ""},
				Channels:      []types.ChannelInfoShort{{Name: "mychannel", URL: ""}}},
			list,
		)

		info, err := manager.ChannelInfo("testchannelid")
		require.NoError(t, err)
		require.Equal(t,
			types.ChannelInfo{Name: "testchannelid", URL: "", ConsensusRelation: types.ConsensusRelationConsenter, Status: types.StatusActive, Height: 1},
			info,
		)

		info, err = manager.ChannelInfo("mychannel")
		require.NoError(t, err)
		require.Equal(t,
			types.ChannelInfo{Name: "mychannel", URL: "", ConsensusRelation: types.ConsensusRelationConsenter, Status: types.StatusActive, Height: 1},
			info,
		)

		// A subsequent creation, replaces the chain.
		manager.CreateChain("mychannel")
		chain2 := manager.GetChain("mychannel")
		require.NotNil(t, chain2)
		// They are not the same
		require.NotEqual(t, chain, chain2)
		// The old chain is halted
		_, ok := <-chain.Chain.(*mockChainCluster).queue
		require.False(t, ok)

		// The new chain is not halted: Close the channel to prove that.
		close(chain2.Chain.(*mockChainCluster).queue)
	})

	// This test brings up the entire system, with the mock consenter, including the broadcasters etc. and creates a new chain
	t.Run("New chain", func(t *testing.T) {
		expectedLastConfigSeq := uint64(1)
		newChainID := "test-new-chain"

		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		lf, rl := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)

		consenter := &mocks.Consenter{}
		consenter.HandleChainCalls(handleChain)
		consenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: consenter}

		manager := NewRegistrar(localconfig.TopLevel{}, lf, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		manager.Initialize(consenters)
		orglessChannelConf := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile, configtest.GetDevConfigDir())
		orglessChannelConf.Application.Organizations = nil
		envConfigUpdate, err := encoder.MakeChannelCreationTransaction(newChainID, mockCrypto(), orglessChannelConf)
		require.NoError(t, err, "Constructing chain creation tx")

		res, err := manager.NewChannelConfig(envConfigUpdate)
		require.NoError(t, err, "Constructing initial channel config")

		configEnv, err := res.ConfigtxValidator().ProposeConfigUpdate(envConfigUpdate)
		require.NoError(t, err, "Proposing initial update")
		require.Equal(t, expectedLastConfigSeq, configEnv.GetConfig().Sequence, "Sequence of config envelope for new channel should always be set to %d", expectedLastConfigSeq)

		ingressTx, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, newChainID, mockCrypto(), configEnv, msgVersion, epoch)
		require.NoError(t, err, "Creating ingresstx")

		wrapped := wrapConfigTx(ingressTx)

		chainSupport := manager.GetChain(manager.SystemChannelID())
		require.NotNilf(t, chainSupport, "Could not find system channel")

		chainSupport.Configure(wrapped, 0)
		func() {
			it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 1}}})
			defer it.Close()
			block, status := it.Next()
			if status != cb.Status_SUCCESS {
				t.Fatalf("Could not retrieve block")
			}
			if len(block.Data.Data) != 1 {
				t.Fatalf("Should have had only one message in the orderer transaction block")
			}

			require.True(t, proto.Equal(wrapped, protoutil.UnmarshalEnvelopeOrPanic(block.Data.Data[0])), "Orderer config block contains wrong transaction")
		}()

		chainSupport = manager.GetChain(newChainID)
		if chainSupport == nil {
			t.Fatalf("Should have gotten new chain which was created")
		}

		messages := make([]*cb.Envelope, confSys.Orderer.BatchSize.MaxMessageCount)
		for i := 0; i < int(confSys.Orderer.BatchSize.MaxMessageCount); i++ {
			messages[i] = makeNormalTx(newChainID, i)
		}

		for _, message := range messages {
			chainSupport.Order(message, 0)
		}

		it, _ := chainSupport.Reader().Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: 0}}})
		defer it.Close()
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve new chain genesis block")
		}
		if len(block.Data.Data) != 1 {
			t.Fatalf("Should have had only one message in the new genesis block")
		}

		require.True(t, proto.Equal(ingressTx, protoutil.UnmarshalEnvelopeOrPanic(block.Data.Data[0])), "Genesis block contains wrong transaction")

		block, status = it.Next()
		if status != cb.Status_SUCCESS {
			t.Fatalf("Could not retrieve block on new chain")
		}
		for i := 0; i < int(confSys.Orderer.BatchSize.MaxMessageCount); i++ {
			if !proto.Equal(protoutil.ExtractEnvelopeOrPanic(block, i), messages[i]) {
				t.Errorf("Block contents wrong at index %d in new chain", i)
			}
		}

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		rcs, err := newChainSupport(manager, chainSupport.ledgerResources, consenters, mockCrypto(), blockcutter.NewMetrics(&disabled.Provider{}), cryptoProvider)
		require.NoError(t, err)
		require.Equal(t, expectedLastConfigSeq, rcs.lastConfigSeq, "On restart, incorrect lastConfigSeq")
	})
}

func TestResourcesCheck(t *testing.T) {
	mockOrderer := &mocks.OrdererConfig{}
	mockOrdererCaps := &mocks.OrdererCapabilities{}
	mockOrderer.CapabilitiesReturns(mockOrdererCaps)
	mockChannel := &mocks.ChannelConfig{}
	mockChannelCaps := &mocks.ChannelCapabilities{}
	mockChannel.CapabilitiesReturns(mockChannelCaps)

	mockResources := &mocks.Resources{}
	mockResources.PolicyManagerReturns(&policies.ManagerImpl{})

	t.Run("GoodResources", func(t *testing.T) {
		mockResources.OrdererConfigReturns(mockOrderer, true)
		mockResources.ChannelConfigReturns(mockChannel)

		err := checkResources(mockResources)
		require.NoError(t, err)
	})

	t.Run("MissingOrdererConfigPanic", func(t *testing.T) {
		mockResources.OrdererConfigReturns(nil, false)

		err := checkResources(mockResources)
		require.Error(t, err)
		require.Regexp(t, "config does not contain orderer config", err.Error())
	})

	t.Run("MissingOrdererCapability", func(t *testing.T) {
		mockResources.OrdererConfigReturns(mockOrderer, true)
		mockOrdererCaps.SupportedReturns(errors.New("An error"))

		err := checkResources(mockResources)
		require.Error(t, err)
		require.Regexp(t, "config requires unsupported orderer capabilities:", err.Error())

		// reset
		mockOrdererCaps.SupportedReturns(nil)
	})

	t.Run("MissingChannelCapability", func(t *testing.T) {
		mockChannelCaps.SupportedReturns(errors.New("An error"))

		err := checkResources(mockResources)
		require.Error(t, err)
		require.Regexp(t, "config requires unsupported channel capabilities:", err.Error())
	})

	t.Run("MissingOrdererConfigPanic", func(t *testing.T) {
		mockResources.OrdererConfigReturns(nil, false)

		require.Panics(t, func() {
			checkResourcesOrPanic(mockResources)
		})
	})
}

// The registrar's BroadcastChannelSupport implementation should reject message types which should not be processed directly.
func TestBroadcastChannelSupport(t *testing.T) {
	// system channel
	confSys := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	genesisBlockSys := encoder.New(confSys).GenesisBlock()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("Rejection", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		ledgerFactory, _ := newLedgerAndFactory(tmpdir, "testchannelid", genesisBlockSys)
		consenter := &mocks.Consenter{}
		consenter.HandleChainCalls(handleChain)
		mockConsenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: consenter}
		registrar := NewRegistrar(localconfig.TopLevel{}, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)
		randomValue := 1
		configTx := makeConfigTx("testchannelid", randomValue)
		_, _, _, err = registrar.BroadcastChannelSupport(configTx)
		require.Error(t, err, "Messages of type HeaderType_CONFIG should return an error.")
	})

	t.Run("No system channel", func(t *testing.T) {
		tmpdir, err := ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpdir)

		ledgerFactory, _ := newLedgerAndFactory(tmpdir, "", nil)
		consenter := &mocks.Consenter{}
		consenter.HandleChainCalls(handleChain)
		mockConsenters := map[string]consensus.Consenter{confSys.Orderer.OrdererType: consenter, "etcdraft": &mocks.Consenter{}}
		registrar := NewRegistrar(localconfig.TopLevel{
			General: localconfig.General{
				BootstrapMethod: "none",
				GenesisFile:     "",
			},
		}, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)
		configTx := makeConfigTxFull("testchannelid", 1)
		_, _, _, err = registrar.BroadcastChannelSupport(configTx)
		require.Error(t, err)
		require.Equal(t, "channel creation request not allowed because the orderer system channel is not defined", err.Error())
	})
}

func TestRegistrar_JoinChannel(t *testing.T) {
	var (
		tmpdir              string
		tlsCA               tlsgen.CA
		confAppRaft         *genesisconfig.Profile
		genesisBlockAppRaft *cb.Block
		confSysRaft         *genesisconfig.Profile
		genesisBlockSysRaft *cb.Block
		cryptoProvider      bccsp.BCCSP
		config              localconfig.TopLevel
		dialer              *cluster.PredicateDialer
		ledgerFactory       blockledger.Factory
		consenter           *mocks.Consenter
		mockConsenters      map[string]consensus.Consenter
	)

	setup := func(t *testing.T) {
		var err error
		tmpdir, err = ioutil.TempDir("", "registrar_test-")
		require.NoError(t, err)

		tlsCA, err = tlsgen.NewCA()
		require.NoError(t, err)

		confAppRaft = genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
		confAppRaft.Consortiums = nil
		confAppRaft.Consortium = ""
		generateCertificates(t, confAppRaft, tlsCA, tmpdir)
		bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
		require.NoError(t, err, "cannot create bootstrapper")
		genesisBlockAppRaft = bootstrapper.GenesisBlockForChannel("my-raft-channel")
		require.NotNil(t, genesisBlockAppRaft)

		confSysRaft = genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
		generateCertificates(t, confSysRaft, tlsCA, tmpdir)
		bootstrapper, err = encoder.NewBootstrapper(confSysRaft)
		require.NoError(t, err, "cannot create bootstrapper")
		genesisBlockSysRaft = bootstrapper.GenesisBlockForChannel("sys-raft-channel")
		require.NotNil(t, genesisBlockSysRaft)

		cryptoProvider, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)

		config = localconfig.TopLevel{
			General: localconfig.General{
				BootstrapMethod: "none",
				Cluster: localconfig.Cluster{
					ReplicationBufferSize:   1,
					ReplicationPullTimeout:  time.Microsecond,
					ReplicationRetryTimeout: time.Microsecond,
					ReplicationMaxRetries:   2,
				},
			},
			ChannelParticipation: localconfig.ChannelParticipation{
				Enabled: true,
			},
			FileLedger: localconfig.FileLedger{
				Location: tmpdir,
			},
		}

		dialer = &cluster.PredicateDialer{
			Config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate: tlsCA.CertBytes(),
				},
			},
		}

		ledgerFactory = newFactory(tmpdir)
		consenter = &mocks.Consenter{}
		consenter.HandleChainCalls(handleChainCluster)
		mockConsenters = map[string]consensus.Consenter{confAppRaft.Orderer.OrdererType: consenter}
	}

	cleanup := func() {
		ledgerFactory.Close()
		os.RemoveAll(tmpdir)
	}

	t.Run("Reject join when removal is occurring", func(t *testing.T) {
		setup(t)
		defer cleanup()

		newLedger(ledgerFactory, "sys-raft-channel", genesisBlockSysRaft)

		registrar := NewRegistrar(localconfig.TopLevel{}, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		registrar.pendingRemoval["some-app-channel"] = consensus.StaticStatusReporter{ConsensusRelation: types.ConsensusRelationFollower, Status: types.StatusInactive}

		info, err := registrar.JoinChannel("some-app-channel", &cb.Block{}, true)
		require.Equal(t, err, types.ErrChannelPendingRemoval)
		require.Equal(t, types.ChannelInfo{}, info)
		joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "some-app-channel.join")
		_, err = os.Stat(joinBlockPath)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Reject join when removal previously failed", func(t *testing.T) {
		setup(t)
		defer cleanup()

		newLedger(ledgerFactory, "sys-raft-channel", genesisBlockSysRaft)

		registrar := NewRegistrar(localconfig.TopLevel{}, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		registrar.pendingRemoval["some-app-channel"] = consensus.StaticStatusReporter{ConsensusRelation: types.ConsensusRelationFollower, Status: types.StatusFailed}

		info, err := registrar.JoinChannel("some-app-channel", &cb.Block{}, true)
		require.Equal(t, types.ErrChannelRemovalFailure, err)
		require.Equal(t, types.ChannelInfo{}, info)
	})

	t.Run("Reject join when system channel exists", func(t *testing.T) {
		setup(t)
		defer cleanup()

		newLedger(ledgerFactory, "sys-raft-channel", genesisBlockSysRaft)

		registrar := NewRegistrar(localconfig.TopLevel{}, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		info, err := registrar.JoinChannel("some-app-channel", &cb.Block{}, true)
		require.EqualError(t, err, "system channel exists")
		require.Equal(t, types.ChannelInfo{}, info)
		joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "some-app-channel.join")
		_, err = os.Stat(joinBlockPath)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Reject join when channel exists", func(t *testing.T) {
		setup(t)
		defer cleanup()

		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		ledger, err := ledgerFactory.GetOrCreate("my-raft-channel")
		require.NoError(t, err)
		ledger.Append(genesisBlockAppRaft)

		// Before creating the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("my-raft-channel"))
		// After creating the chain, it exists
		registrar.CreateChain("my-raft-channel")
		require.NotNil(t, registrar.GetChain("my-raft-channel"))

		info, err := registrar.JoinChannel("my-raft-channel", &cb.Block{}, true)
		require.EqualError(t, err, "channel already exists")
		require.Equal(t, types.ChannelInfo{}, info)
		joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "my-channel.join")
		_, err = os.Stat(joinBlockPath)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Reject system channel join when app channels exist", func(t *testing.T) {
		setup(t)
		defer cleanup()

		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		ledger, err := ledgerFactory.GetOrCreate("my-raft-channel")
		require.NoError(t, err)
		ledger.Append(genesisBlockAppRaft)

		// Before creating the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("my-raft-channel"))
		// After creating the chain, it exists
		registrar.CreateChain("my-raft-channel")
		require.NotNil(t, registrar.GetChain("my-raft-channel"))

		info, err := registrar.JoinChannel("sys-channel", &cb.Block{}, false)
		require.EqualError(t, err, "application channels already exist")
		require.Equal(t, types.ChannelInfo{}, info)
		joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "sys-channel.join")
		_, err = os.Stat(joinBlockPath)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("Join app channel as member without on-boarding", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(true, nil)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		fakeFields := newFakeMetricsFields()
		registrar.channelParticipationMetrics = newFakeMetrics(fakeFields)

		t.Run("failure - consenter channel membership error", func(t *testing.T) {
			badConsenter := &mocks.Consenter{}
			badConsenter.IsChannelMemberReturns(false, errors.New("apple"))
			mockConsenters := map[string]consensus.Consenter{confAppRaft.Orderer.OrdererType: badConsenter}
			registrar.Initialize(mockConsenters)

			// Before joining the channel, it doesn't exist
			require.Nil(t, registrar.GetChain("my-raft-channel"))
			require.Empty(t, ledgerFactory.ChannelIDs())

			_, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
			require.EqualError(t, err, "failed to determine cluster membership from join-block: apple")

			// After join failure, check that everything has been cleaned up
			require.Eventually(t, func() bool { return len(ledgerFactory.ChannelIDs()) == 0 }, time.Minute, time.Second)
			require.Empty(t, ledgerFactory.ChannelIDs())
			joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "my-raft-channel.join")
			_, err = os.Stat(joinBlockPath)
			require.True(t, os.IsNotExist(err))
			require.Nil(t, registrar.GetChain("my-raft-channel"))
			require.Empty(t, ledgerFactory.ChannelIDs())
		})

		t.Run("failure - consenter error", func(t *testing.T) {
			badConsenter := &mocks.Consenter{}
			badConsenter.IsChannelMemberReturns(true, nil)
			badConsenter.HandleChainReturns(nil, errors.New("banana"))
			mockConsenters := map[string]consensus.Consenter{confAppRaft.Orderer.OrdererType: badConsenter}
			registrar.Initialize(mockConsenters)

			// Before joining the channel, it doesn't exist
			require.Nil(t, registrar.GetChain("my-raft-channel"))
			require.Empty(t, ledgerFactory.ChannelIDs())

			_, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
			require.EqualError(t, err, "failed to create chain support: error creating consenter for channel: my-raft-channel: banana")

			// After join failure, check that everything has been cleaned up
			require.Eventually(t, func() bool { return len(ledgerFactory.ChannelIDs()) == 0 }, time.Minute, time.Second)
			require.Empty(t, ledgerFactory.ChannelIDs())
			joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "my-raft-channel.join")
			_, err = os.Stat(joinBlockPath)
			require.True(t, os.IsNotExist(err))
			require.Nil(t, registrar.GetChain("my-raft-channel"))
			require.Empty(t, ledgerFactory.ChannelIDs())
		})

		t.Run("success", func(t *testing.T) {
			registrar.Initialize(mockConsenters)
			// Before joining the channel, it doesn't exist
			require.Nil(t, registrar.GetChain("my-raft-channel"))
			info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
			require.NoError(t, err)
			require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 0x1}, info)
			// After creating the channel, it exists
			require.NotNil(t, registrar.GetChain("my-raft-channel"))

			// ChannelInfo() and ChannelList() are working fine
			info, err = registrar.ChannelInfo("my-raft-channel")
			require.NoError(t, err)
			require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 0x1}, info)
			channelList := registrar.ChannelList()
			require.Equal(t, 1, len(channelList.Channels))
			require.Equal(t, "my-raft-channel", channelList.Channels[0].Name)
			require.Nil(t, channelList.SystemChannel)
			joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "my-raft-channel.join")
			_, err = os.Stat(joinBlockPath)
			require.True(t, os.IsNotExist(err))
			checkMetrics(t, fakeFields, []string{"channel", "my-raft-channel"}, 1, 1, 1)
		})
	})

	t.Run("Join app channel as member with on-boarding", func(t *testing.T) {
		setup(t)
		defer cleanup()

		genesisBlockAppRaft.Header.Number = 10
		consenter.IsChannelMemberReturns(true, nil)

		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		registrar.Initialize(mockConsenters)

		// Before join the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("my-raft-channel"))

		info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "onboarding", Height: 0x0}, info)
		// After creating the follower.Chain, it not in the chains map.
		require.Nil(t, registrar.GetChain("my-raft-channel"))

		// ChannelInfo() and ChannelList() are working fine
		info, err = registrar.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "onboarding", Height: 0x0}, info)
		channelList := registrar.ChannelList()
		require.Equal(t, 1, len(channelList.Channels))
		require.Equal(t, "my-raft-channel", channelList.Channels[0].Name)
		require.Nil(t, channelList.SystemChannel)

		fChain := registrar.GetFollower("my-raft-channel")
		require.NotNil(t, fChain)
		fChain.Halt()
	})

	t.Run("Join app channel as follower, with on-boarding", func(t *testing.T) {
		setup(t)
		defer cleanup()

		genesisBlockAppRaft.Header.Number = 10
		consenter.IsChannelMemberReturns(false, nil)

		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		fakeFields := newFakeMetricsFields()
		registrar.channelParticipationMetrics = newFakeMetrics(fakeFields)
		registrar.Initialize(mockConsenters)

		// Before join the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("my-raft-channel"))

		info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "follower", Status: "onboarding", Height: 0x0}, info)
		// After creating the follower.Chain, it not in the chains map.
		require.Nil(t, registrar.GetChain("my-raft-channel"))
		// ChannelInfo() and ChannelList() are working fine
		info, err = registrar.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "follower", Status: "onboarding", Height: 0x0}, info)
		channelList := registrar.ChannelList()
		require.Equal(t, 1, len(channelList.Channels))
		require.Equal(t, "my-raft-channel", channelList.Channels[0].Name)
		require.Nil(t, channelList.SystemChannel)

		fChain := registrar.GetFollower("my-raft-channel")
		require.NotNil(t, fChain)
		fChain.Halt()

		checkMetrics(t, fakeFields, []string{"channel", "my-raft-channel"}, 2, 2, 1)
	})

	t.Run("Join app channel as follower then switch to member", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(false, nil)

		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		fakeFields := newFakeMetricsFields()
		registrar.channelParticipationMetrics = newFakeMetrics(fakeFields)
		registrar.Initialize(mockConsenters)

		// Before join the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("my-raft-channel"))
		require.Nil(t, registrar.GetFollower("my-raft-channel"))

		genesisBlockAppRaft.Header.Number = 1
		info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "follower", Status: "onboarding", Height: 0x0}, info)

		// After creating the follower.Chain, it not in the chains map, it is in the followers map.
		require.Nil(t, registrar.GetChain("my-raft-channel"))
		fChain := registrar.GetFollower("my-raft-channel")
		require.NotNil(t, fChain)
		fChain.Halt()

		// join-block exists before switching from follower to member
		joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "my-raft-channel.join")
		_, err = os.Stat(joinBlockPath)
		require.NoError(t, err)
		checkMetrics(t, fakeFields, []string{"channel", "my-raft-channel"}, 2, 2, 1)

		// Let's assume the follower appended a block
		genesisBlockAppRaft.Header.Number = 0
		newLedger(ledgerFactory, "my-raft-channel", genesisBlockAppRaft)

		// Now Switch => a chain is created and the follower removed
		require.NotPanics(t, func() { registrar.SwitchFollowerToChain("my-raft-channel") })
		// Now the chain is in the chains map, the follower is gone
		require.NotNil(t, registrar.GetChain("my-raft-channel"))
		require.Nil(t, registrar.GetFollower("my-raft-channel"))
		// ChannelInfo() and ChannelList() are still working fine
		info, err = registrar.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 0x1}, info)
		channelList := registrar.ChannelList()
		require.Equal(t, 1, len(channelList.Channels))
		require.Equal(t, "my-raft-channel", channelList.Channels[0].Name)
		require.Nil(t, channelList.SystemChannel)

		// join-block removed after switching from follower to member
		_, err = os.Stat(joinBlockPath)
		require.True(t, os.IsNotExist(err))

		checkMetrics(t, fakeFields, []string{"channel", "my-raft-channel"}, 1, 1, 2)
	})

	t.Run("Join app channel as member, then switch to follower", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(true, nil)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		fakeFields := newFakeMetricsFields()
		registrar.channelParticipationMetrics = newFakeMetrics(fakeFields)
		registrar.Initialize(mockConsenters)

		// Before join the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("my-raft-channel"))

		info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 0x1}, info)
		// After creating the chain, it exists
		cs := registrar.GetChain("my-raft-channel")
		require.NotNil(t, cs)

		// ChannelInfo() and ChannelList() are working fine
		info, err = registrar.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 0x1}, info)
		channelList := registrar.ChannelList()
		require.Equal(t, 1, len(channelList.Channels))
		require.Equal(t, "my-raft-channel", channelList.Channels[0].Name)
		require.Nil(t, channelList.SystemChannel)
		checkMetrics(t, fakeFields, []string{"channel", "my-raft-channel"}, 1, 1, 1)

		// Let's assume the chain appended another config block
		genesisBlockAppRaft.Header.PreviousHash = protoutil.BlockHeaderHash(genesisBlockAppRaft.Header)
		genesisBlockAppRaft.Header.Number = 1
		require.NoError(t, cs.Append(genesisBlockAppRaft))
		consenter.IsChannelMemberReturns(false, nil)
		require.Equal(t, uint64(2), cs.Height())

		//Now halt and switch, as if the orderer was evicted
		cs.Halt()
		require.NotPanics(t, func() { registrar.SwitchChainToFollower("my-raft-channel") })
		// Now the follower is in the followers map, the chain is gone
		fChain := registrar.GetFollower("my-raft-channel")
		require.NotNil(t, fChain)
		require.Nil(t, registrar.GetChain("my-raft-channel"))
		// ChannelInfo() and ChannelList() are still working fine
		info, err = registrar.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "follower", Status: "active", Height: 0x2}, info)
		channelList = registrar.ChannelList()
		require.Equal(t, 1, len(channelList.Channels))
		require.Equal(t, "my-raft-channel", channelList.Channels[0].Name)
		require.Nil(t, channelList.SystemChannel)
		fChain.Halt()
		require.False(t, fChain.IsRunning())
		checkMetrics(t, fakeFields, []string{"channel", "my-raft-channel"}, 2, 1, 3)
	})

	t.Run("Join system channel without on-boarding", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(true, nil)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		// Before join the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("sys-raft-channel"))

		info, err := registrar.JoinChannel("sys-raft-channel", genesisBlockSysRaft, false)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "sys-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "inactive", Height: 0x1}, info)
		// After creating the chain, it exists
		cs := registrar.GetChain("sys-raft-channel")
		require.NotNil(t, cs)

		// join-block exists
		joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "sys-raft-channel.join")
		_, err = os.Stat(joinBlockPath)
		require.NoError(t, err)

		// ChannelInfo() and ChannelList() are working fine
		info, err = registrar.ChannelInfo("sys-raft-channel")
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "sys-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "inactive", Height: 0x1}, info)
		channelList := registrar.ChannelList()
		require.Equal(t, 0, len(channelList.Channels))
		require.NotNil(t, channelList.SystemChannel)
		require.Equal(t, "sys-raft-channel", channelList.SystemChannel.Name)
		ledgerRW, err := ledgerFactory.GetOrCreate("sys-raft-channel")
		require.NoError(t, err)
		require.Equal(t, uint64(1), ledgerRW.Height(), "block was appended")
	})

	t.Run("Join system channel with on-boarding", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(true, nil)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		// Before join the chain, it doesn't exist
		require.Nil(t, registrar.GetChain("sys-raft-channel"))

		genesisBlockSysRaft.Header.Number = 7
		info, err := registrar.JoinChannel("sys-raft-channel", genesisBlockSysRaft, false)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "sys-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "inactive", Height: 0x0}, info)
		// After creating the chain, it exists
		cs := registrar.GetChain("sys-raft-channel")
		require.NotNil(t, cs)

		// join-block exists
		joinBlockPath := filepath.Join(tmpdir, "pendingops", "join", "sys-raft-channel.join")
		_, err = os.Stat(joinBlockPath)
		require.NoError(t, err)

		// ChannelInfo() and ChannelList() are working fine
		info, err = registrar.ChannelInfo("sys-raft-channel")
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "sys-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "inactive", Height: 0x0}, info)
		channelList := registrar.ChannelList()
		require.Equal(t, 0, len(channelList.Channels))
		require.NotNil(t, channelList.SystemChannel)
		require.Equal(t, "sys-raft-channel", channelList.SystemChannel.Name)
		ledgerRW, err := ledgerFactory.GetOrCreate("sys-raft-channel")
		require.NoError(t, err)
		require.Equal(t, uint64(0), ledgerRW.Height(), "block was not appended")
	})
}

func checkMetrics(t *testing.T, fakeFields *fakeMetricsFields, expectedLabels []string, expectedRelation, expectedStatus, expectedCallCount int) {
	require.Equal(t, expectedCallCount, fakeFields.fakeConsensusRelation.SetCallCount())
	require.Equal(t, float64(expectedRelation), fakeFields.fakeConsensusRelation.SetArgsForCall(expectedCallCount-1))
	require.Equal(t, expectedCallCount, fakeFields.fakeConsensusRelation.WithCallCount())
	require.Equal(t, expectedLabels, fakeFields.fakeConsensusRelation.WithArgsForCall(expectedCallCount-1))
	require.Equal(t, expectedCallCount, fakeFields.fakeStatus.SetCallCount())
	require.Equal(t, float64(expectedStatus), fakeFields.fakeStatus.SetArgsForCall(expectedCallCount-1))
	require.Equal(t, expectedCallCount, fakeFields.fakeStatus.WithCallCount())
	require.Equal(t, expectedLabels, fakeFields.fakeStatus.WithArgsForCall(expectedCallCount-1))
}

func TestRegistrar_RemoveChannel(t *testing.T) {
	var (
		tmpdir              string
		tlsCA               tlsgen.CA
		confAppRaft         *genesisconfig.Profile
		genesisBlockAppRaft *cb.Block
		confSysRaft         *genesisconfig.Profile
		genesisBlockSysRaft *cb.Block

		cryptoProvider  bccsp.BCCSP
		config          localconfig.TopLevel
		dialer          *cluster.PredicateDialer
		appBootstrapper *encoder.Bootstrapper
		ledgerFactory   blockledger.Factory
		consenter       *mocks.Consenter
		mockConsenters  map[string]consensus.Consenter
	)

	setup := func(t *testing.T) {
		var err error
		tmpdir, err = ioutil.TempDir("", "remove-channel")
		require.NoError(t, err)

		tlsCA, err = tlsgen.NewCA()
		require.NoError(t, err)

		confAppRaft = genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
		confAppRaft.Consortiums = nil
		confAppRaft.Consortium = ""
		generateCertificates(t, confAppRaft, tlsCA, tmpdir)
		appBootstrapper, err = encoder.NewBootstrapper(confAppRaft)
		require.NoError(t, err, "cannot create bootstrapper")
		genesisBlockAppRaft = appBootstrapper.GenesisBlockForChannel("my-raft-channel")
		require.NotNil(t, genesisBlockAppRaft)

		confSysRaft = genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
		generateCertificates(t, confSysRaft, tlsCA, tmpdir)
		sysBootstrapper, err := encoder.NewBootstrapper(confSysRaft)
		require.NoError(t, err, "cannot create bootstrapper")
		genesisBlockSysRaft = sysBootstrapper.GenesisBlockForChannel("raft-sys-channel")
		require.NotNil(t, genesisBlockSysRaft)

		cryptoProvider, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)

		config = localconfig.TopLevel{
			ChannelParticipation: localconfig.ChannelParticipation{
				Enabled: true,
			},
			General: localconfig.General{
				BootstrapMethod: "none",
				Cluster: localconfig.Cluster{
					ReplicationBufferSize:   1,
					ReplicationPullTimeout:  time.Microsecond,
					ReplicationRetryTimeout: time.Microsecond,
					ReplicationMaxRetries:   2,
				},
			},
			FileLedger: localconfig.FileLedger{
				Location: tmpdir,
			},
		}
		dialer = &cluster.PredicateDialer{
			Config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate: tlsCA.CertBytes(),
				},
			},
		}

		ledgerFactory = newFactory(tmpdir)
		consenter = &mocks.Consenter{}
		consenter.HandleChainCalls(handleChainCluster)
		mockConsenters = map[string]consensus.Consenter{"etcdraft": consenter}
	}

	cleanup := func() {
		ledgerFactory.Close()
		os.RemoveAll(tmpdir)
	}

	t.Run("kafka system channel exists", func(t *testing.T) {
		setup(t)
		defer cleanup()

		confSysKafka := genesisconfig.Load(genesisconfig.SampleInsecureKafkaProfile, configtest.GetDevConfigDir())
		genesisBlockSysKafka := encoder.New(confSysKafka).GenesisBlockForChannel("kafka-sys-channel")
		newLedger(ledgerFactory, "kafka-sys-channel", genesisBlockSysKafka)

		kafkaConsenter := &mocks.Consenter{}
		kafkaConsenter.HandleChainCalls(handleChain)
		mockConsenters["kafka"] = kafkaConsenter

		registrar := NewRegistrar(localconfig.TopLevel{}, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		t.Run("reject removal of app channel", func(t *testing.T) {
			err := registrar.RemoveChannel("some-app-channel")
			require.EqualError(t, err, "system channel exists")
		})

		t.Run("reject removal of kafka system channel", func(t *testing.T) {
			err := registrar.RemoveChannel("kafka-sys-channel")
			require.EqualError(t, err, "cannot remove kafka system channel: kafka-sys-channel")
		})
	})

	t.Run("without a system channel failures", func(t *testing.T) {
		setup(t)
		defer cleanup()

		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, nil)
		registrar.Initialize(mockConsenters)

		t.Run("when channel id does not exist", func(t *testing.T) {
			err := registrar.RemoveChannel("some-raft-channel")
			require.EqualError(t, err, "channel does not exist")
		})

		t.Run("when channel id is blank", func(t *testing.T) {
			err := registrar.RemoveChannel("")
			require.EqualError(t, err, "channel does not exist")
		})
	})

	t.Run("remove app channel", func(t *testing.T) {
		setup(t)
		defer cleanup()

		t.Run("consenter", func(t *testing.T) {
			consenter.IsChannelMemberReturns(true, nil)
			registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
			registrar.Initialize(mockConsenters)

			require.Nil(t, registrar.GetChain("my-raft-channel"))
			require.NotContains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")
			info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
			require.NoError(t, err)
			require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 1}, info)
			require.NotNil(t, registrar.GetChain("my-raft-channel"))
			require.Contains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")

			err = registrar.RemoveChannel("my-raft-channel")
			require.NoError(t, err)

			// After removing the channel, it no longer exists in the registrar or the ledger
			require.Nil(t, registrar.GetChain("my-raft-channel"))
			require.Eventually(t, func() bool { return len(ledgerFactory.ChannelIDs()) == 0 }, time.Minute, time.Second)
			require.NotContains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")
		})

		t.Run("follower", func(t *testing.T) {
			consenter.IsChannelMemberReturns(false, nil)
			registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
			registrar.Initialize(mockConsenters)

			genesisBlockAppRaftFollower := appBootstrapper.GenesisBlockForChannel("my-follower-raft-channel")
			require.NotNil(t, genesisBlockAppRaftFollower)

			require.Nil(t, registrar.GetChain("my-follower-raft-channel"))
			require.NotContains(t, ledgerFactory.ChannelIDs(), "my-follower-raft-channel")
			info, err := registrar.JoinChannel("my-follower-raft-channel", genesisBlockAppRaftFollower, true)
			require.NoError(t, err)
			require.Equal(t, types.ChannelInfo{Name: "my-follower-raft-channel", URL: "", ConsensusRelation: "follower", Status: "onboarding", Height: 0}, info)
			require.NotNil(t, registrar.GetFollower("my-follower-raft-channel"))
			require.Contains(t, ledgerFactory.ChannelIDs(), "my-follower-raft-channel")

			err = registrar.RemoveChannel("my-follower-raft-channel")
			require.NoError(t, err)

			// After removing the channel, it no longer exists in the registrar or the ledger
			require.Nil(t, registrar.GetFollower("my-follower-raft-channel"))
			require.Eventually(t, func() bool { return len(ledgerFactory.ChannelIDs()) == 0 }, time.Minute, time.Second)
			require.NotContains(t, ledgerFactory.ChannelIDs(), "my-follower-raft-channel")

			channelInfo, err := registrar.ChannelInfo("my-follower-raft-channel")
			require.Equal(t, err, types.ErrChannelNotExist)
			require.Equal(t, channelInfo, types.ChannelInfo{})
		})
	})

	t.Run("remove system channel", func(t *testing.T) {
		setup(t)
		defer cleanup()

		newLedger(ledgerFactory, "raft-sys-channel", genesisBlockSysRaft)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		registrar.Initialize(mockConsenters)
		require.NotNil(t, registrar.GetChain("raft-sys-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "raft-sys-channel")

		createLedgerAndChain(t, registrar, ledgerFactory, genesisBlockAppRaft, "my-raft-channel")
		require.NotNil(t, registrar.GetChain("my-raft-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")
		genesisBlockAppRaftNonMember := appBootstrapper.GenesisBlockForChannel("channel-im-not-a-member-of")
		require.NotNil(t, genesisBlockAppRaftNonMember)
		createLedgerAndChain(t, registrar, ledgerFactory, genesisBlockAppRaftNonMember, "channel-im-not-a-member-of")

		require.NotNil(t, registrar.GetChain("channel-im-not-a-member-of"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "channel-im-not-a-member-of")
		consenter.IsChannelMemberStub = func(b *cb.Block) (bool, error) {
			if bytes.Equal(b.Header.DataHash, genesisBlockAppRaft.Header.DataHash) {
				return true, nil
			}
			return false, nil
		}

		err := registrar.RemoveChannel("raft-sys-channel")
		require.NoError(t, err)

		// After removing the system channel, it no longer exists in the registrar or the ledger
		require.Empty(t, registrar.SystemChannelID())
		require.Nil(t, registrar.SystemChannel())
		require.Nil(t, registrar.GetChain("raft-sys-channel"))
		require.NotContains(t, ledgerFactory.ChannelIDs(), "raft-sys-channel")

		// application channel members still exist
		require.NotNil(t, registrar.GetChain("my-raft-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")
		// channels the orderer wasn't a member no longer exist
		require.Nil(t, registrar.GetChain("channel-im-not-a-member-of"))
		require.NotContains(t, ledgerFactory.ChannelIDs(), "channel-im-not-a-member-of")
	})

	t.Run("remove system channel ledger fails", func(t *testing.T) {
		setup(t)
		defer cleanup()

		newLedger(ledgerFactory, "raft-sys-channel", genesisBlockSysRaft)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		registrar.Initialize(mockConsenters)
		require.NotNil(t, registrar.GetChain("raft-sys-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "raft-sys-channel")

		os.RemoveAll(filepath.Join(tmpdir, "pendingops", "remove"))

		err := registrar.RemoveChannel("raft-sys-channel")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed removing ledger for system channel raft-sys-channel")

		require.NotNil(t, registrar.SystemChannelID())
		require.Contains(t, ledgerFactory.ChannelIDs(), "raft-sys-channel")
	})

	t.Run("remove non-systemchannel members fails", func(t *testing.T) {
		setup(t)
		defer cleanup()

		newLedger(ledgerFactory, "raft-sys-channel", genesisBlockSysRaft)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		registrar.Initialize(mockConsenters)
		require.NotNil(t, registrar.GetChain("raft-sys-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "raft-sys-channel")

		genesisBlockAppRaftNonMember := appBootstrapper.GenesisBlockForChannel("channel-im-not-a-member-of")
		require.NotNil(t, genesisBlockAppRaftNonMember)
		createLedgerAndChain(t, registrar, ledgerFactory, genesisBlockAppRaftNonMember, "channel-im-not-a-member-of")

		require.NotNil(t, registrar.GetChain("channel-im-not-a-member-of"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "channel-im-not-a-member-of")
		consenter.IsChannelMemberStub = func(b *cb.Block) (bool, error) {
			os.RemoveAll(filepath.Join(tmpdir, "pendingops", "remove"))
			if bytes.Equal(b.Header.DataHash, genesisBlockAppRaft.Header.DataHash) {
				return true, nil
			}
			return false, nil
		}

		err := registrar.RemoveChannel("raft-sys-channel")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed removing ledger for channel(s): channel-im-not-a-member-of")

		require.Empty(t, registrar.SystemChannelID())
		require.Nil(t, registrar.SystemChannel())
		require.Nil(t, registrar.GetChain("raft-sys-channel"))
		require.NotContains(t, ledgerFactory.ChannelIDs(), "raft-sys-channel")

		require.NotNil(t, registrar.GetChain("channel-im-not-a-member-of"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "channel-im-not-a-member-of")
	})

	t.Run("app channel is already being removed", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(true, nil)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		registrar.Initialize(mockConsenters)

		require.Nil(t, registrar.GetChain("my-raft-channel"))
		require.NotContains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")
		info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 1}, info)
		require.NotNil(t, registrar.GetChain("my-raft-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")

		registrar.pendingRemoval["my-raft-channel"] = consensus.StaticStatusReporter{ConsensusRelation: types.ConsensusRelationFollower, Status: types.StatusInactive}

		err = registrar.RemoveChannel("my-raft-channel")
		require.Error(t, err)
		require.Equal(t, types.ErrChannelPendingRemoval, err)

		require.Contains(t, registrar.ChannelList().Channels, types.ChannelInfoShort{Name: "my-raft-channel"})
	})

	t.Run("app channel removal previously failed", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(true, nil)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		registrar.Initialize(mockConsenters)

		require.Nil(t, registrar.GetChain("my-raft-channel"))
		require.NotContains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")
		info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 1}, info)
		require.NotNil(t, registrar.GetChain("my-raft-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")

		registrar.pendingRemoval["my-raft-channel"] = consensus.StaticStatusReporter{ConsensusRelation: types.ConsensusRelationFollower, Status: types.StatusFailed}

		err = registrar.RemoveChannel("my-raft-channel")
		require.NoError(t, err)

		require.Eventually(t, func() bool { return len(ledgerFactory.ChannelIDs()) == 0 }, time.Minute, time.Second)
		require.NotContains(t, registrar.ChannelList().Channels, types.ChannelInfoShort{Name: "my-raft-channel"})
	})

	t.Run("remove channel fails", func(t *testing.T) {
		setup(t)
		defer cleanup()

		consenter.IsChannelMemberReturns(true, nil)
		registrar := NewRegistrar(config, ledgerFactory, mockCrypto(), &disabled.Provider{}, cryptoProvider, dialer)
		registrar.Initialize(mockConsenters)

		require.Nil(t, registrar.GetChain("my-raft-channel"))
		require.NotContains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")
		info, err := registrar.JoinChannel("my-raft-channel", genesisBlockAppRaft, true)
		require.NoError(t, err)
		require.Equal(t, types.ChannelInfo{Name: "my-raft-channel", URL: "", ConsensusRelation: "consenter", Status: "active", Height: 1}, info)
		require.NotNil(t, registrar.GetChain("my-raft-channel"))
		require.Contains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")

		// This will force the failure of the underlying factory.Remove() because the pendingops/remove will not exist
		os.RemoveAll(filepath.Join(tmpdir, "pendingops", "remove"))

		err = registrar.RemoveChannel("my-raft-channel")
		require.NoError(t, err)

		require.Nil(t, registrar.GetChain("my-raft-channel"))
		require.Eventually(t, func() bool { return len(ledgerFactory.ChannelIDs()) == 1 }, time.Minute, time.Second)
		require.Contains(t, ledgerFactory.ChannelIDs(), "my-raft-channel")

		// Confirm removal failure by checking channel status
		channelInfo, err := registrar.ChannelInfo("my-raft-channel")
		require.NoError(t, err)
		require.Equal(t, channelInfo, types.ChannelInfo{
			Name:              "my-raft-channel",
			ConsensusRelation: types.ConsensusRelationConsenter,
			Status:            types.StatusFailed})
	})
}

func generateCertificates(t *testing.T, confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	for i, c := range confAppRaft.Orderer.EtcdRaft.Consenters {
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		require.NoError(t, err)
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = ioutil.WriteFile(srvP, srvC.Cert, 0644)
		require.NoError(t, err)

		clnC, err := tlsCA.NewClientCertKeyPair()
		require.NoError(t, err)
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = ioutil.WriteFile(clnP, clnC.Cert, 0644)
		require.NoError(t, err)

		c.ServerTlsCert = []byte(srvP)
		c.ClientTlsCert = []byte(clnP)
	}
}

func createLedgerAndChain(t *testing.T, r *Registrar, lf blockledger.Factory, b *cb.Block, channel string) {
	require.Nil(t, r.GetChain(channel))
	require.NotContains(t, lf.ChannelIDs(), channel)
	ledger, err := lf.GetOrCreate(channel)
	require.NoError(t, err)
	ledger.Append(b)
	require.Contains(t, lf.ChannelIDs(), channel)
	r.CreateChain(channel)
	require.NotNil(t, r.GetChain(channel))
}
