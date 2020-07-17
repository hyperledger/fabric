/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/follower"
	"github.com/hyperledger/fabric/orderer/common/follower/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/stretchr/testify/require"
)

func TestBlockPullerFromJoinBlock(t *testing.T) {
	tlsCA, _ := tlsgen.NewCA()
	channelID := "my-raft-channel"
	joinBlockAppRaft := generateJoinBlock(t, tlsCA, channelID, 10)
	require.NotNil(t, joinBlockAppRaft)

	mockSupport := &mocks.BlockPullerSupport{}

	dialer := &cluster.PredicateDialer{
		Config: comm.ClientConfig{
			SecOpts: comm.SecureOptions{
				Certificate: tlsCA.CertBytes(),
			},
		},
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("good", func(t *testing.T) {
		bp, err := follower.BlockPullerFromJoinBlock(
			joinBlockAppRaft,
			channelID,
			mockSupport,
			dialer,
			localconfig.Cluster{},
			cryptoProvider,
		)
		require.NoError(t, err)
		require.NotNil(t, bp)
	})

	t.Run("dialer is nil", func(t *testing.T) {
		bp, err := follower.BlockPullerFromJoinBlock(
			joinBlockAppRaft,
			channelID,
			mockSupport,
			nil,
			localconfig.Cluster{},
			cryptoProvider,
		)
		require.EqualError(t, err, "base dialer is nil")
		require.Nil(t, bp)
	})

	t.Run("dialer has bad cert", func(t *testing.T) {
		badDialer := &cluster.PredicateDialer{
			Config: dialer.Config.Clone(),
		}
		badDialer.Config.SecOpts.Certificate = []byte("not-a-certificate")
		bp, err := follower.BlockPullerFromJoinBlock(
			joinBlockAppRaft,
			channelID,
			mockSupport,
			badDialer,
			localconfig.Cluster{},
			cryptoProvider,
		)
		require.EqualError(t, err, "client certificate isn't in PEM format: not-a-certificate")
		require.Nil(t, bp)
	})

	t.Run("bad join block", func(t *testing.T) {
		bp, err := follower.BlockPullerFromJoinBlock(
			&cb.Block{Header: &cb.BlockHeader{}},
			channelID,
			mockSupport,
			dialer,
			localconfig.Cluster{},
			cryptoProvider,
		)
		require.EqualError(t, err, "error extracting endpoints from config block: block data is nil")
		require.Nil(t, bp)
	})
}

func generateJoinBlock(t *testing.T, tlsCA tlsgen.CA, channelID string, number uint64) *cb.Block {
	tmpdir, err := ioutil.TempDir("", "block-puller-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

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
