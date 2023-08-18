/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/core/deliverservice/fake"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o fake/ledger_info.go --fake-name LedgerInfo . ledgerInfo
type ledgerInfo interface {
	blocksprovider.LedgerInfo
}

func TestStartDeliverForChannel(t *testing.T) {
	fakeLedgerInfo := &fake.LedgerInfo{}
	fakeLedgerInfo.LedgerHeightReturns(0, fmt.Errorf("fake-ledger-error"))

	secOpts := comm.SecureOptions{
		UseTLS:            true,
		RequireClientCert: true,
		// The below certificates were taken from the peer TLS
		// dir as output by cryptogen.
		// They are server.crt and server.key respectively.
		Certificate: []byte(`-----BEGIN CERTIFICATE-----
MIIChTCCAiygAwIBAgIQOrr7/tDzKhhCba04E6QVWzAKBggqhkjOPQQDAjB2MQsw
CQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZy
YW5jaXNjbzEZMBcGA1UEChMQb3JnMS5leGFtcGxlLmNvbTEfMB0GA1UEAxMWdGxz
Y2Eub3JnMS5leGFtcGxlLmNvbTAeFw0xOTA4MjcyMDA2MDBaFw0yOTA4MjQyMDA2
MDBaMFsxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQH
Ew1TYW4gRnJhbmNpc2NvMR8wHQYDVQQDExZwZWVyMC5vcmcxLmV4YW1wbGUuY29t
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAExglppLxiAYSasrdFsrZJDxRULGBb
wHlArrap9SmAzGIeeIuqe9t3F23Q5Jry9lAnIh8h3UlkvZZpClXcjRiCeqOBtjCB
szAOBgNVHQ8BAf8EBAMCBaAwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMC
MAwGA1UdEwEB/wQCMAAwKwYDVR0jBCQwIoAgL35aqafj6SNnWdI4aMLh+oaFJvsA
aoHgYMkcPvvkiWcwRwYDVR0RBEAwPoIWcGVlcjAub3JnMS5leGFtcGxlLmNvbYIF
cGVlcjCCFnBlZXIwLm9yZzEuZXhhbXBsZS5jb22CBXBlZXIwMAoGCCqGSM49BAMC
A0cAMEQCIAiAGoYeKPMd3bqtixZji8q2zGzLmIzq83xdTJoZqm50AiAKleso2EVi
2TwsekWGpMaCOI6JV1+ZONyti6vBChhUYg==
-----END CERTIFICATE-----`),
		Key: []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgxiyAFyD0Eg1NxjbS
U2EKDLoTQr3WPK8z7WyeOSzr+GGhRANCAATGCWmkvGIBhJqyt0WytkkPFFQsYFvA
eUCutqn1KYDMYh54i6p723cXbdDkmvL2UCciHyHdSWS9lmkKVdyNGIJ6
-----END PRIVATE KEY-----`,
		),
	}

	confAppRaft := genesisconfig.Load(genesisconfig.SampleAppChannelEtcdRaftProfile, configtest.GetDevConfigDir())
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	generateCertificates(t, confAppRaft, tlsCA, certDir)
	bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
	require.NoError(t, err)
	channelConfigProto := &cb.Config{ChannelGroup: bootstrapper.GenesisChannelGroup()}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("Green Path With Mutual TLS", func(t *testing.T) {
		ds := NewDeliverService(&Config{
			DeliverServiceConfig: &DeliverServiceConfig{
				SecOpts: secOpts,
			},
			ChannelConfig:  channelConfigProto,
			CryptoProvider: cryptoProvider,
		}).(*deliverServiceImpl)

		finalized := make(chan struct{})
		err = ds.StartDeliverForChannel("channel-id", fakeLedgerInfo, func() {
			close(finalized)
		})
		require.NoError(t, err)

		select {
		case <-finalized:
		case <-time.After(time.Second):
			require.FailNow(t, "finalizer should have executed")
		}

		require.NotNil(t, ds.blockDeliverer)
		bpd := ds.blockDeliverer.(*blocksprovider.Deliverer)

		require.Equal(t, "76f7a03f8dfdb0ef7c4b28b3901fe163c730e906c70e4cdf887054ad5f608bed", fmt.Sprintf("%x", bpd.TLSCertHash))
	})

	t.Run("Green Path without mutual TLS", func(t *testing.T) {
		ds := NewDeliverService(&Config{
			DeliverServiceConfig: &DeliverServiceConfig{},
			ChannelConfig:        channelConfigProto,
			CryptoProvider:       cryptoProvider,
		}).(*deliverServiceImpl)

		finalized := make(chan struct{})
		err := ds.StartDeliverForChannel("channel-id", fakeLedgerInfo, func() {
			close(finalized)
		})
		require.NoError(t, err)

		select {
		case <-finalized:
		case <-time.After(time.Second):
			require.FailNow(t, "finalizer should have executed")
		}

		require.NotNil(t, ds.blockDeliverer)
		bpd := ds.blockDeliverer.(*blocksprovider.Deliverer)
		require.Nil(t, bpd.TLSCertHash)
	})

	t.Run("Leader yields and re-elected: Start->Stop->Start", func(t *testing.T) {
		ds := NewDeliverService(&Config{
			DeliverServiceConfig: &DeliverServiceConfig{},
			ChannelConfig:        channelConfigProto,
			CryptoProvider:       cryptoProvider,
		}).(*deliverServiceImpl)

		finalized := make(chan struct{})
		err := ds.StartDeliverForChannel("channel-id", fakeLedgerInfo, func() {
			close(finalized)
		})
		require.NoError(t, err)

		select {
		case <-finalized:
		case <-time.After(time.Second):
			require.FailNow(t, "finalizer should have executed")
		}

		require.NotNil(t, ds.blockDeliverer)
		bpd := ds.blockDeliverer.(*blocksprovider.Deliverer)
		require.Nil(t, bpd.TLSCertHash)

		err = ds.StopDeliverForChannel()
		require.NoError(t, err)
		require.Nil(t, ds.blockDeliverer)

		finalized2 := make(chan struct{})
		err = ds.StartDeliverForChannel("channel-id", fakeLedgerInfo, func() {
			close(finalized2)
		})
		require.NoError(t, err)
		select {
		case <-finalized2:
		case <-time.After(time.Second):
			require.FailNow(t, "finalizer should have executed")
		}
	})

	t.Run("Exists", func(t *testing.T) {
		ds := NewDeliverService(&Config{
			DeliverServiceConfig: &DeliverServiceConfig{},
			ChannelConfig:        channelConfigProto,
			CryptoProvider:       cryptoProvider,
		}).(*deliverServiceImpl)

		err := ds.StartDeliverForChannel("channel-id", fakeLedgerInfo, func() {})
		require.NoError(t, err)

		err = ds.StartDeliverForChannel("channel-id", fakeLedgerInfo, func() {})
		require.EqualError(t, err, "block deliverer for channel `channel-id` already exists")
	})

	t.Run("Stopping", func(t *testing.T) {
		ds := NewDeliverService(&Config{
			DeliverServiceConfig: &DeliverServiceConfig{},
		}).(*deliverServiceImpl)

		ds.Stop()

		err := ds.StartDeliverForChannel("channel-id", fakeLedgerInfo, func() {})
		require.EqualError(t, err, "block deliverer for channel `channel-id` is stopping")
	})
}

func TestStopDeliverForChannel(t *testing.T) {
	t.Run("Green path", func(t *testing.T) {
		ds := NewDeliverService(&Config{}).(*deliverServiceImpl)
		doneA := make(chan struct{})
		ds.blockDeliverer = &blocksprovider.Deliverer{
			DoneC:  doneA,
			Logger: flogging.MustGetLogger("deliveryclient.test"),
		}
		ds.channelID = "channel-id"

		err := ds.StopDeliverForChannel()
		require.NoError(t, err)

		select {
		case <-doneA:
		default:
			require.Fail(t, "should have stopped the blocksprovider")
		}
	})

	t.Run("Already stopping", func(t *testing.T) {
		ds := NewDeliverService(&Config{}).(*deliverServiceImpl)
		ds.blockDeliverer = &blocksprovider.Deliverer{
			DoneC:  make(chan struct{}),
			Logger: flogging.MustGetLogger("deliveryclient.test"),
		}
		ds.channelID = "channel-id"

		ds.Stop()
		err := ds.StopDeliverForChannel()
		require.EqualError(t, err, "block deliverer for channel `channel-id` is already stopped")
	})

	t.Run("Already stopped", func(t *testing.T) {
		ds := NewDeliverService(&Config{}).(*deliverServiceImpl)
		ds.blockDeliverer = &blocksprovider.Deliverer{
			DoneC:  make(chan struct{}),
			Logger: flogging.MustGetLogger("deliveryclient.test"),
		}
		ds.channelID = "channel-id"

		ds.StopDeliverForChannel()
		err := ds.StopDeliverForChannel()
		require.EqualError(t, err, "block deliverer for channel `channel-id` is <nil>, can't stop delivery")
	})
}

func TestStop(t *testing.T) {
	ds := NewDeliverService(&Config{}).(*deliverServiceImpl)
	ds.blockDeliverer = &blocksprovider.Deliverer{
		DoneC:  make(chan struct{}),
		Logger: flogging.MustGetLogger("deliveryclient.test"),
	}

	require.False(t, ds.stopping)
	bpd := ds.blockDeliverer.(*blocksprovider.Deliverer)
	select {
	case <-bpd.DoneC:
		require.Fail(t, "block providers should not be closed")
	default:
	}

	ds.Stop()
	require.True(t, ds.stopping)

	select {
	case <-bpd.DoneC:
	default:
		require.Fail(t, "block providers should te closed")
	}
}

// TODO this pattern repeats itself in several places. Make it common in the 'genesisconfig' package to easily create
// Raft genesis blocks
func generateCertificates(t *testing.T, confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	for i, c := range confAppRaft.Orderer.EtcdRaft.Consenters {
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

		c.ServerTlsCert = []byte(srvP)
		c.ClientTlsCert = []byte(clnP)
	}
}
