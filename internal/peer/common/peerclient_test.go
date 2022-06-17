/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package common_test

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func initPeerTestEnv(t *testing.T) (cfgPath string, cleanup func()) {
	t.Helper()
	cfgPath = t.TempDir()
	certsDir := filepath.Join(cfgPath, "certs")
	err := os.Mkdir(certsDir, 0o755)
	require.NoError(t, err)

	configFile, err := os.Create(filepath.Join(cfgPath, "test.yaml"))
	require.NoError(t, err)
	defer configFile.Close()

	configStr := `
peer:
  tls:
    rootcert:
      file: "certs/ca.crt"
    clientKey:
      file: "certs/client.key"
    clientCert:
      file: "certs/client.crt"
  client:
    connTimeout: 1s

orderer:
  tls:
    rootcert:
      file: "certs/ca.crt"
    clientKey:
      file: "certs/client.key"
    clientCert:
      file: "certs/client.crt"
  client:
    connTimeout: 1s
`
	_, err = configFile.WriteString(configStr)
	require.NoError(t, err)

	os.Setenv("FABRIC_CFG_PATH", cfgPath)
	viper.Reset()
	_ = common.InitConfig("test")
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)

	caCrtFile := path.Join(certsDir, "ca.crt")
	err = ioutil.WriteFile(caCrtFile, ca.CertBytes(), 0o644)
	require.NoError(t, err)

	kp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	key := path.Join(certsDir, "client.key")
	err = ioutil.WriteFile(key, kp.Key, 0o644)
	require.NoError(t, err)

	crt := path.Join(certsDir, "client.crt")
	err = ioutil.WriteFile(crt, kp.Cert, 0o644)
	require.NoError(t, err)

	ekey := path.Join(certsDir, "empty.key")
	err = ioutil.WriteFile(ekey, []byte{}, 0o644)
	require.NoError(t, err)

	ecrt := path.Join(certsDir, "empty.crt")
	err = ioutil.WriteFile(ecrt, []byte{}, 0o644)
	require.NoError(t, err)

	configFile, err = os.Create(filepath.Join(certsDir, "bad.key"))
	require.NoError(t, err)
	defer configFile.Close()

	_, err = configFile.WriteString(`
-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgg6BAaCpwlmg/hEP4
QjUeWEu3crkxMvjq4vYh3LaDREuhRANCAAR+FujNKcGQW/CEpMU6Yp45ye2cbOwJ
-----END PRIVATE KEY-----
	`)
	require.NoError(t, err)

	return cfgPath, func() {
		err := os.Unsetenv("FABRIC_CFG_PATH")
		require.NoError(t, err)
		viper.Reset()
	}
}

func TestNewPeerClientFromEnv(t *testing.T) {
	_, cleanup := initPeerTestEnv(t)
	defer cleanup()

	pClient, err := common.NewPeerClientFromEnv()
	require.NoError(t, err)
	require.NotNil(t, pClient)

	viper.Set("peer.tls.enabled", true)
	pClient, err = common.NewPeerClientFromEnv()
	require.NoError(t, err)
	require.NotNil(t, pClient)

	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.clientAuthRequired", true)
	pClient, err = common.NewPeerClientFromEnv()
	require.NoError(t, err)
	require.NotNil(t, pClient)

	// bad key file
	// This used to be detected when creating the client instead
	// of when the client connects.
	badKeyFile := filepath.Join("certs", "bad.key")
	viper.Set("peer.tls.clientKey.file", badKeyFile)
	pClient, err = common.NewPeerClientFromEnv()
	require.NoError(t, err)
	_, err = pClient.Dial("127.0.0.1:0")
	require.ErrorContains(t, err, "failed to load client certificate:")
	require.ErrorContains(t, err, "tls: failed to parse private key")

	// bad cert file path
	viper.Set("peer.tls.clientCert.file", "./nocert.crt")
	pClient, err = common.NewPeerClientFromEnv()
	require.ErrorContains(t, err, "unable to load peer.tls.clientCert.file")
	require.Nil(t, pClient)

	// bad key file path
	viper.Set("peer.tls.clientKey.file", "./nokey.key")
	pClient, err = common.NewPeerClientFromEnv()
	require.ErrorContains(t, err, "unable to load peer.tls.clientKey.file")
	require.Nil(t, pClient)

	// bad ca path
	viper.Set("peer.tls.rootcert.file", "noroot.crt")
	pClient, err = common.NewPeerClientFromEnv()
	require.ErrorContains(t, err, "unable to load peer.tls.rootcert.file")
	require.Nil(t, pClient)
}

func TestPeerClient(t *testing.T) {
	_, cleanup := initPeerTestEnv(t)
	defer cleanup()

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("error creating server for test: %v", err)
	}
	defer lis.Close()
	srv, err := comm.NewGRPCServerFromListener(lis, comm.ServerConfig{})
	if err != nil {
		t.Fatalf("error creating gRPC server for test: %v", err)
	}
	go srv.Start()
	defer srv.Stop()
	viper.Set("peer.address", lis.Addr().String())
	pClient1, err := common.NewPeerClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create PeerClient for test: %v", err)
	}
	eClient, err := pClient1.Endorser()
	require.NoError(t, err)
	require.NotNil(t, eClient)
	eClient, err = common.GetEndorserClient("", "")
	require.NoError(t, err)
	require.NotNil(t, eClient)

	dClient, err := pClient1.Deliver()
	require.NoError(t, err)
	require.NotNil(t, dClient)
	dClient, err = common.GetDeliverClient("", "")
	require.NoError(t, err)
	require.NotNil(t, dClient)
}

func TestPeerClientTimeout(t *testing.T) {
	t.Run("PeerClient.GetEndorser() timeout", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		viper.Set("peer.client.connTimeout", 10*time.Millisecond)
		defer cleanup()
		pClient, err := common.NewPeerClientFromEnv()
		if err != nil {
			t.Fatalf("failed to create PeerClient for test: %v", err)
		}
		_, err = pClient.Endorser()
		require.Contains(t, err.Error(), "endorser client failed to connect")
	})
	t.Run("GetEndorserClient() timeout", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		viper.Set("peer.client.connTimeout", 10*time.Millisecond)
		defer cleanup()
		_, err := common.GetEndorserClient("", "")
		require.Contains(t, err.Error(), "endorser client failed to connect")
	})
	t.Run("PeerClient.Deliver() timeout", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		viper.Set("peer.client.connTimeout", 10*time.Millisecond)
		defer cleanup()
		pClient, err := common.NewPeerClientFromEnv()
		if err != nil {
			t.Fatalf("failed to create PeerClient for test: %v", err)
		}
		_, err = pClient.Deliver()
		require.Contains(t, err.Error(), "deliver client failed to connect")
	})
	t.Run("GetDeliverClient() timeout", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		viper.Set("peer.client.connTimeout", 10*time.Millisecond)
		defer cleanup()
		_, err := common.GetDeliverClient("", "")
		require.Contains(t, err.Error(), "deliver client failed to connect")
	})
	t.Run("PeerClient.Certificate()", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		defer cleanup()
		pClient, err := common.NewPeerClientFromEnv()
		if err != nil {
			t.Fatalf("failed to create PeerClient for test: %v", err)
		}
		cert := pClient.Certificate()
		require.NotNil(t, cert)
	})
}

func TestGetClientCertificate(t *testing.T) {
	t.Run("GetClientCertificate_clientAuthRequired_disabled", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		defer cleanup()
		cert, err := common.GetClientCertificate()
		require.NotEqual(t, cert, &tls.Certificate{})
		require.NoError(t, err)
	})

	t.Run("GetClientCertificate_clientAuthRequired_enabled", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		defer cleanup()
		viper.Set("peer.tls.clientAuthRequired", true)
		viper.Set("peer.tls.clientKey.file", filepath.Join("./certs", "client.key"))
		viper.Set("peer.tls.clientCert.file", filepath.Join("./certs", "client.crt"))
		defer viper.Reset()

		cert, err := common.GetClientCertificate()
		require.NoError(t, err)
		require.NotEqual(t, cert, tls.Certificate{})
	})

	t.Run("GetClientCertificate_empty_keyfile", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		defer cleanup()
		viper.Set("peer.tls.clientAuthRequired", true)
		viper.Set("peer.tls.clientKey.file", filepath.Join("./certs", "empty.key"))
		viper.Set("peer.tls.clientCert.file", filepath.Join("./certs", "client.crt"))
		defer viper.Reset()

		cert, err := common.GetClientCertificate()
		require.EqualError(t, err, "failed to load client certificate: tls: failed to find any PEM data in key input")
		require.Equal(t, cert, tls.Certificate{})
	})

	t.Run("GetClientCertificate_empty_certfile", func(t *testing.T) {
		_, cleanup := initPeerTestEnv(t)
		defer cleanup()
		viper.Set("peer.tls.clientAuthRequired", true)
		viper.Set("peer.tls.clientKey.file", filepath.Join("./certs", "client.key"))
		viper.Set("peer.tls.clientCert.file", filepath.Join("./certs", "empty.crt"))
		defer viper.Reset()

		cert, err := common.GetClientCertificate()
		require.EqualError(t, err, "failed to load client certificate: tls: failed to find any PEM data in certificate input")
		require.Equal(t, cert, tls.Certificate{})
	})

	t.Run("GetClientCertificate_bad_keyfilepath", func(t *testing.T) {
		cfgPath, cleanup := initPeerTestEnv(t)
		defer cleanup()
		// bad key file path
		viper.Set("peer.tls.clientAuthRequired", true)
		viper.Set("peer.tls.clientKey.file", filepath.Join("./certs", "nokey.key"))
		viper.Set("peer.tls.clientCert.file", filepath.Join("./certs", "client.crt"))
		defer viper.Reset()

		cert, err := common.GetClientCertificate()
		require.EqualError(t, err, fmt.Sprintf("unable to load peer.tls.clientKey.file: open %s/certs/nokey.key: no such file or directory", cfgPath))
		require.Equal(t, cert, tls.Certificate{})
	})

	t.Run("GetClientCertificate_missing_certfilepath", func(t *testing.T) {
		// missing cert file path
		viper.Set("peer.tls.clientAuthRequired", true)
		viper.Set("peer.tls.clientKey.file", filepath.Join("./testdata/certs", "client.key"))
		defer viper.Reset()

		cert, err := common.GetClientCertificate()
		require.EqualError(t, err, "unable to load peer.tls.clientKey.file: open testdata/certs/client.key: no such file or directory")
		require.Equal(t, cert, tls.Certificate{})
	})
}

func TestNewPeerClientForAddress(t *testing.T) {
	cfgPath, cleanup := initPeerTestEnv(t)
	defer cleanup()

	// TLS disabled
	viper.Set("peer.tls.enabled", false)

	// success case
	pClient, err := common.NewPeerClientForAddress("testPeer", "")
	require.NoError(t, err)
	require.NotNil(t, pClient)

	// failure - no peer address supplied
	pClient, err = common.NewPeerClientForAddress("", "")
	require.Contains(t, err.Error(), "peer address must be set")
	require.Nil(t, pClient)

	// TLS enabled
	viper.Set("peer.tls.enabled", true)

	// Enable clientAuthRequired
	viper.Set("peer.tls.clientAuthRequired", true)

	// success case
	pClient, err = common.NewPeerClientForAddress("tlsPeer", path.Join(cfgPath, "certs", "ca.crt"))
	require.NoError(t, err)
	require.NotNil(t, pClient)

	// failure - bad tls root cert file
	pClient, err = common.NewPeerClientForAddress("badPeer", "bad.crt")
	require.Contains(t, err.Error(), "unable to load TLS root cert file from bad.crt")
	require.Nil(t, pClient)

	// failure - empty tls root cert file
	pClient, err = common.NewPeerClientForAddress("badPeer", "")
	require.Contains(t, err.Error(), "tls root cert file must be set")
	require.Nil(t, pClient)

	// failure - empty tls root cert file
	viper.Set("peer.tls.clientCert.file", "./nocert.crt")
	pClient, err = common.NewPeerClientForAddress("badPeer", "")
	require.Contains(t, err.Error(), "unable to load peer.tls.clientCert.file")
	require.Nil(t, pClient)

	// bad key file
	viper.Set("peer.tls.clientKey.file", "./nokey.key")
	viper.Set("peer.client.connTimeout", time.Duration(0))
	pClient, err = common.NewPeerClientForAddress("badPeer", "")
	require.Contains(t, err.Error(), "unable to load peer.tls.clientKey.file")
	require.Nil(t, pClient)
}

func TestGetClients_AddressError(t *testing.T) {
	_, cleanup := initPeerTestEnv(t)
	defer cleanup()

	viper.Set("peer.tls.enabled", true)

	// failure
	eClient, err := common.GetEndorserClient("peer0", "")
	require.Contains(t, err.Error(), "tls root cert file must be set")
	require.Nil(t, eClient)

	dClient, err := common.GetDeliverClient("peer0", "")
	require.Contains(t, err.Error(), "tls root cert file must be set")
	require.Nil(t, dClient)
}
