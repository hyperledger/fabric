/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient_test

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliverclient"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/stretchr/testify/require"
)

func TestBlockVerifierAssembler(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	config := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, config, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(config)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("Good config envelope", func(t *testing.T) {
		bva := &deliverclient.BlockVerifierAssembler{BCCSP: cryptoProvider}
		verifier, err := bva.VerifierFromConfig(&common.ConfigEnvelope{
			Config: &common.Config{
				ChannelGroup: group,
			},
		}, "mychannel")
		require.NoError(t, err)

		require.Error(t, verifier(nil, nil))
	})

	t.Run("Bad config envelope", func(t *testing.T) {
		bva := &deliverclient.BlockVerifierAssembler{BCCSP: cryptoProvider}
		verifier, err := bva.VerifierFromConfig(&common.ConfigEnvelope{}, "mychannel")
		require.EqualError(t, err, "channelconfig Config cannot be nil")
		err = verifier(nil, nil)
		require.EqualError(t, err, "failed to initialize block verifier function: channelconfig Config cannot be nil")
	})
}

func generateCertificatesSmartBFT(t *testing.T, confAppSmartBFT *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	for i, c := range confAppSmartBFT.Orderer.ConsenterMapping {
		t.Logf("BFT Consenter: %+v", c)
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		require.NoError(t, err)
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = os.WriteFile(srvP, srvC.Cert, 0o644)
		require.NoError(t, err)

		clnC, err := tlsCA.NewClientCertKeyPair()
		require.NoError(t, err)
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = os.WriteFile(clnP, clnC.Cert, 0o644)
		require.NoError(t, err)

		c.Identity = srvP
		c.ServerTLSCert = srvP
		c.ClientTLSCert = clnP
	}
}
