/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package common_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func initPeerTestEnv(t *testing.T) {
	t.Helper()
	cfgPath := "./testdata"
	os.Setenv("FABRIC_CFG_PATH", cfgPath)
	viper.Reset()
	_ = common.InitConfig("test")
	caFile := filepath.Join("certs", "ca.crt")
	viper.Set("peer.tls.rootcert.file", caFile)
	keyFile := filepath.Join("certs", "client.key")
	viper.Set("peer.tls.clientKey.file", keyFile)
	certFile := filepath.Join("certs", "client.crt")
	viper.Set("peer.tls.clientCert.file", certFile)
}

func TestNewPeerClientFromEnv(t *testing.T) {
	initPeerTestEnv(t)

	pClient, err := common.NewPeerClientFromEnv()
	assert.NoError(t, err)
	assert.NotNil(t, pClient)

	viper.Set("peer.tls.enabled", true)
	pClient, err = common.NewPeerClientFromEnv()
	assert.NoError(t, err)
	assert.NotNil(t, pClient)

	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.clientAuthRequired", true)
	pClient, err = common.NewPeerClientFromEnv()
	assert.NoError(t, err)
	assert.NotNil(t, pClient)

	// bad key file
	badKeyFile := filepath.Join("certs", "bad.key")
	viper.Set("peer.tls.clientKey.file", badKeyFile)
	pClient, err = common.NewPeerClientFromEnv()
	assert.Contains(t, err.Error(), "failed to create PeerClient from config")
	assert.Nil(t, pClient)

	// bad cert file path
	viper.Set("peer.tls.clientCert.file", "./nocert.crt")
	pClient, err = common.NewPeerClientFromEnv()
	assert.Contains(t, err.Error(), "unable to load peer.tls.clientCert.file")
	assert.Contains(t, err.Error(), "failed to load config for PeerClient")
	assert.Nil(t, pClient)

	// bad key file path
	viper.Set("peer.tls.clientKey.file", "./nokey.key")
	pClient, err = common.NewPeerClientFromEnv()
	assert.Contains(t, err.Error(), "unable to load peer.tls.clientKey.file")
	assert.Nil(t, pClient)

	// bad ca path
	viper.Set("peer.tls.rootcert.file", "noroot.crt")
	pClient, err = common.NewPeerClientFromEnv()
	assert.Contains(t, err.Error(), "unable to load peer.tls.rootcert.file")
	assert.Nil(t, pClient)

	viper.Reset()
	os.Unsetenv("FABRIC_CFG_PATH")

}

func TestPeerClient(t *testing.T) {
	initPeerTestEnv(t)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("error creating server for test: %v", err)
	}
	defer lis.Close()
	viper.Set("peer.address", lis.Addr().String())
	pClient1, err := common.NewPeerClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create PeerClient for test: %v", err)
	}
	eClient, err := pClient1.Endorser()
	assert.NoError(t, err)
	assert.NotNil(t, eClient)
	eClient, err = common.GetEndorserClient("", "")
	assert.NoError(t, err)
	assert.NotNil(t, eClient)

	aClient, err := pClient1.Admin()
	assert.NoError(t, err)
	assert.NotNil(t, aClient)
	aClient, err = common.GetAdminClient()
	assert.NoError(t, err)
	assert.NotNil(t, aClient)

	viper.Set("peer.address", "")
	t.Run("PeerClient.GetEndorser() timeout", func(t *testing.T) {
		t.Parallel()
		pClient2, err2 := common.NewPeerClientFromEnv()
		if err != nil {
			t.Fatalf("failed to create PeerClient for test: %v", err)
		}
		_, err2 = pClient2.Endorser()
		assert.Contains(t, err2.Error(), "endorser client failed to connect")
	})
	t.Run("GetEndorserClient() timeout", func(t *testing.T) {
		t.Parallel()
		_, err3 := common.GetEndorserClient("", "")
		assert.Contains(t, err3.Error(), "endorser client failed to connect")
	})
	t.Run("PeerClient.GetAdmin() timeout", func(t *testing.T) {
		t.Parallel()
		pClient3, err4 := common.NewPeerClientFromEnv()
		if err != nil {
			t.Fatalf("failed to create PeerClient for test: %v", err)
		}
		_, err4 = pClient3.Admin()
		assert.Contains(t, err4.Error(), "admin client failed to connect")
	})
	t.Run("GetAdminClient() timeout", func(t *testing.T) {
		t.Parallel()
		_, err5 := common.GetAdminClient()
		assert.Contains(t, err5.Error(), "admin client failed to connect")
	})

	viper.Reset()
	os.Unsetenv("FABRIC_CFG_PATH")
}

func TestNewPeerClientForAddress(t *testing.T) {
	initPeerTestEnv(t)

	// TLS disabled
	viper.Set("peer.tls.enabled", false)

	// success case
	pClient, err := common.NewPeerClientForAddress("testPeer", "")
	assert.NoError(t, err)
	assert.NotNil(t, pClient)

	// failure - no peer address supplied
	pClient, err = common.NewPeerClientForAddress("", "")
	assert.Contains(t, err.Error(), "peer address must be set")
	assert.Nil(t, pClient)

	// TLS enabled
	viper.Set("peer.tls.enabled", true)

	// success case
	pClient, err = common.NewPeerClientForAddress("tlsPeer", "./testdata/certs/ca.crt")
	assert.NoError(t, err)
	assert.NotNil(t, pClient)

	// failure - bad tls root cert file
	pClient, err = common.NewPeerClientForAddress("badPeer", "bad.crt")
	assert.Contains(t, err.Error(), "unable to load TLS root cert file from bad.crt")
	assert.Nil(t, pClient)

	// failure - empty tls root cert file
	pClient, err = common.NewPeerClientForAddress("badPeer", "")
	assert.Contains(t, err.Error(), "tls root cert file must be set")
	assert.Nil(t, pClient)

	viper.Reset()
	os.Unsetenv("FABRIC_CFG_PATH")
}

func TestGetEndorserClientForAddressError(t *testing.T) {
	initPeerTestEnv(t)
	viper.Set("peer.tls.enabled", true)

	// failure
	eClient, err := common.GetEndorserClient("peer0", "")
	assert.Contains(t, err.Error(), "tls root cert file must be set")
	assert.Nil(t, eClient)

	viper.Reset()
	os.Unsetenv("FABRIC_CFG_PATH")
}
