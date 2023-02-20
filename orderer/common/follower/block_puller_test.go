/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower_test

import (
	"fmt"
	"io/ioutil"
	"path"
	"sync/atomic"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/follower"
	"github.com/hyperledger/fabric/orderer/common/follower/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

var (
	channelID  string
	mockSigner *mocks.SignerSerializer
	tlsCA      tlsgen.CA
	dialer     *cluster.PredicateDialer
	cryptoProv bccsp.BCCSP
)

func setupBlockPullerTest(t *testing.T) {
	channelID = "my-raft-channel"
	mockSigner = &mocks.SignerSerializer{}

	var err error
	tlsCA, err = tlsgen.NewCA()
	require.NoError(t, err)
	dialer = &cluster.PredicateDialer{
		Config: comm.ClientConfig{
			SecOpts: comm.SecureOptions{
				Certificate: tlsCA.CertBytes(),
			},
		},
	}

	cryptoProv, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
}

func TestNewBlockPullerFactory(t *testing.T) {
	setupBlockPullerTest(t)

	t.Run("good", func(t *testing.T) {
		bpf, err := follower.NewBlockPullerCreator(channelID, testLogger, mockSigner, dialer, localconfig.Cluster{}, cryptoProv)
		require.NoError(t, err)
		require.NotNil(t, bpf)
	})

	t.Run("dialer is nil", func(t *testing.T) {
		require.Panics(t, func() {
			follower.NewBlockPullerCreator(channelID, testLogger, mockSigner, nil, localconfig.Cluster{}, cryptoProv)
		})
	})

	t.Run("dialer has bad cert", func(t *testing.T) {
		badDialer := &cluster.PredicateDialer{
			Config: dialer.Config,
		}
		badDialer.Config.SecOpts.Certificate = []byte("not-a-certificate")
		bpf, err := follower.NewBlockPullerCreator(channelID, testLogger, mockSigner, badDialer, localconfig.Cluster{}, cryptoProv)
		require.EqualError(t, err, "client certificate isn't in PEM format: not-a-certificate")
		require.Nil(t, bpf)
	})
}

func TestBlockPullerFactory_BlockPuller(t *testing.T) {
	setupBlockPullerTest(t)

	factory, err := follower.NewBlockPullerCreator(channelID, testLogger, mockSigner, dialer, localconfig.Cluster{}, cryptoProv)
	require.NotNil(t, factory)
	require.NoError(t, err)

	t.Run("good", func(t *testing.T) {
		joinBlockAppRaft := generateJoinBlock(t, tlsCA, channelID, 10)
		require.NotNil(t, joinBlockAppRaft)
		bp, err := factory.BlockPuller(joinBlockAppRaft, make(chan struct{}))
		require.NoError(t, err)
		require.NotNil(t, bp)
	})

	t.Run("bad join block", func(t *testing.T) {
		bp, err := factory.BlockPuller(&cb.Block{Header: &cb.BlockHeader{}}, make(chan struct{}))
		require.EqualError(t, err, "error extracting endpoints from config block: block data is nil")
		require.Nil(t, bp)
	})
}

func TestBlockPullerFactory_VerifyBlockSequence(t *testing.T) {
	// replaces cluster.VerifyBlocks, count blocks
	var numBlocks int32
	altVerifyBlocks := func(blockBuff []*cb.Block, signatureVerifier protoutil.BlockVerifierFunc, vb protoutil.VerifierBuilder) error { // replaces cluster.VerifyBlocks, count invocations
		if len(blockBuff) == 0 {
			return errors.New("buffer is empty")
		}

		require.NotNil(t, signatureVerifier)
		atomic.StoreInt32(&numBlocks, int32(len(blockBuff)))
		return nil
	}

	t.Run("skip genesis block, alone", func(t *testing.T) {
		setupBlockPullerTest(t)
		atomic.StoreInt32(&numBlocks, 0)
		creator, err := follower.NewBlockPullerCreator(channelID, testLogger, mockSigner, dialer, localconfig.Cluster{}, cryptoProv)
		require.NotNil(t, creator)
		require.NoError(t, err)
		creator.ClusterVerifyBlocks = altVerifyBlocks
		blocks := []*cb.Block{
			generateJoinBlock(t, tlsCA, channelID, 0),
		}

		err = creator.VerifyBlockSequence(blocks, "")
		require.NoError(t, err)
		require.Equal(t, int32(0), atomic.LoadInt32(&numBlocks))
	})

	t.Run("skip genesis block as part of a slice", func(t *testing.T) {
		gb := generateJoinBlock(t, tlsCA, channelID, 0)
		setupBlockPullerTest(t)
		atomic.StoreInt32(&numBlocks, 0)
		creator, err := follower.NewBlockPullerCreator(channelID, testLogger, mockSigner, dialer, localconfig.Cluster{}, cryptoProv)
		require.NotNil(t, creator)
		require.NoError(t, err)
		creator.JoinBlock = gb
		creator.ClusterVerifyBlocks = altVerifyBlocks
		blocks := []*cb.Block{
			gb,
			protoutil.NewBlock(1, nil),
			protoutil.NewBlock(2, nil),
		}

		tx, err := protoutil.CreateSignedEnvelopeWithTLSBinding(0, "mychannel", nil, &cb.Payload{}, 0, 0, nil)
		require.NoError(t, err)

		blocks[1].Data.Data = [][]byte{protoutil.MarshalOrPanic(tx)}
		blocks[2].Data.Data = [][]byte{protoutil.MarshalOrPanic(tx)}

		err = creator.VerifyBlockSequence(blocks, "")
		require.NoError(t, err)
		require.Equal(t, int32(2), atomic.LoadInt32(&numBlocks))
	})

	t.Run("verify all blocks in slice", func(t *testing.T) {
		setupBlockPullerTest(t)
		atomic.StoreInt32(&numBlocks, 0)
		creator, err := follower.NewBlockPullerCreator(channelID, testLogger, mockSigner, dialer, localconfig.Cluster{}, cryptoProv)
		require.NotNil(t, creator)
		require.NoError(t, err)
		creator.ClusterVerifyBlocks = altVerifyBlocks

		blocks := []*cb.Block{protoutil.NewBlock(4, []byte{}), protoutil.NewBlock(5, []byte{}), protoutil.NewBlock(6, []byte{})}

		err = creator.VerifyBlockSequence(blocks, "")
		require.EqualError(t, err, "nil block signature verifier")

		creator.UpdateVerifierFromConfigBlock(generateJoinBlock(t, tlsCA, channelID, 0))
		err = creator.VerifyBlockSequence(blocks, "")
		require.NoError(t, err)
		require.Equal(t, int32(3), atomic.LoadInt32(&numBlocks))
	})
}

func generateJoinBlock(t *testing.T, tlsCA tlsgen.CA, channelID string, number uint64) *cb.Block {
	tmpdir := t.TempDir()

	confAppRaft := genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
	confAppRaft.Consortiums = nil
	confAppRaft.Consortium = ""
	generateCertificates(t, confAppRaft, tlsCA, tmpdir)
	bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
	require.NoError(t, err, "cannot create bootstrapper")

	joinBlockAppRaft := bootstrapper.GenesisBlockForChannel(channelID)
	joinBlockAppRaft.Header.Number = number
	return joinBlockAppRaft
}

func generateCertificates(t *testing.T, confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	for i, c := range confAppRaft.Orderer.EtcdRaft.Consenters {
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		require.NoError(t, err)
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = ioutil.WriteFile(srvP, srvC.Cert, 0o644)
		require.NoError(t, err)

		clnC, err := tlsCA.NewClientCertKeyPair()
		require.NoError(t, err)
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = ioutil.WriteFile(clnP, clnC.Cert, 0o644)
		require.NoError(t, err)

		c.ServerTlsCert = []byte(srvP)
		c.ClientTlsCert = []byte(clnP)
	}
}
