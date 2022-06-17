/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package common_test

import (
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

func initOrdererTestEnv(t *testing.T) (cleanup func()) {
	t.Helper()
	cfgPath := t.TempDir()
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

	return func() {
		err := os.Unsetenv("FABRIC_CFG_PATH")
		require.NoError(t, err)
		viper.Reset()
	}
}

func TestNewOrdererClientFromEnv(t *testing.T) {
	cleanup := initOrdererTestEnv(t)
	defer cleanup()

	oClient, err := common.NewOrdererClientFromEnv()
	require.NoError(t, err)
	require.NotNil(t, oClient)

	viper.Set("orderer.tls.enabled", true)
	oClient, err = common.NewOrdererClientFromEnv()
	require.NoError(t, err)
	require.NotNil(t, oClient)

	viper.Set("orderer.tls.enabled", true)
	viper.Set("orderer.tls.clientAuthRequired", true)
	oClient, err = common.NewOrdererClientFromEnv()
	require.NoError(t, err)
	require.NotNil(t, oClient)

	// bad key file
	badKeyFile := filepath.Join("certs", "bad.key")
	viper.Set("orderer.tls.clientKey.file", badKeyFile)
	oClient, err = common.NewOrdererClientFromEnv()
	require.NoError(t, err)
	_, err = oClient.Dial("127.0.0.1:0")
	require.ErrorContains(t, err, "failed to load client certificate:")
	require.ErrorContains(t, err, "tls: failed to parse private key")

	// bad cert file path
	viper.Set("orderer.tls.clientCert.file", "./nocert.crt")
	oClient, err = common.NewOrdererClientFromEnv()
	require.ErrorContains(t, err, "unable to load orderer.tls.clientCert.file")
	require.ErrorContains(t, err, "failed to load config for OrdererClient")
	require.Nil(t, oClient)

	// bad key file path
	viper.Set("orderer.tls.clientKey.file", "./nokey.key")
	oClient, err = common.NewOrdererClientFromEnv()
	require.ErrorContains(t, err, "unable to load orderer.tls.clientKey.file")
	require.Nil(t, oClient)

	// bad ca path
	viper.Set("orderer.tls.rootcert.file", "noroot.crt")
	oClient, err = common.NewOrdererClientFromEnv()
	require.ErrorContains(t, err, "unable to load orderer.tls.rootcert.file")
	require.Nil(t, oClient)
}

func TestOrdererClient(t *testing.T) {
	cleanup := initOrdererTestEnv(t)
	defer cleanup()

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("error creating listener for test: %v", err)
	}
	defer lis.Close()
	srv, err := comm.NewGRPCServerFromListener(lis, comm.ServerConfig{})
	if err != nil {
		t.Fatalf("error creating gRPC server for test: %v", err)
	}
	go srv.Start()
	defer srv.Stop()
	viper.Set("orderer.address", lis.Addr().String())
	oClient, err := common.NewOrdererClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create OrdererClient for test: %v", err)
	}
	bc, err := oClient.Broadcast()
	require.NoError(t, err)
	require.NotNil(t, bc)
	dc, err := oClient.Deliver()
	require.NoError(t, err)
	require.NotNil(t, dc)
}

func TestOrdererClientTimeout(t *testing.T) {
	t.Run("OrdererClient.Broadcast() timeout", func(t *testing.T) {
		cleanup := initOrdererTestEnv(t)
		viper.Set("orderer.client.connTimeout", 10*time.Millisecond)
		defer cleanup()
		oClient, err := common.NewOrdererClientFromEnv()
		if err != nil {
			t.Fatalf("failed to create OrdererClient for test: %v", err)
		}
		_, err = oClient.Broadcast()
		require.Contains(t, err.Error(), "orderer client failed to connect")
	})
	t.Run("OrdererClient.Deliver() timeout", func(t *testing.T) {
		cleanup := initOrdererTestEnv(t)
		viper.Set("orderer.client.connTimeout", 10*time.Millisecond)
		defer cleanup()
		oClient, err := common.NewOrdererClientFromEnv()
		if err != nil {
			t.Fatalf("failed to create OrdererClient for test: %v", err)
		}
		_, err = oClient.Deliver()
		require.Contains(t, err.Error(), "orderer client failed to connect")
	})
}
