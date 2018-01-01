/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

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

func initOrdererTestEnv(t *testing.T) {
	t.Helper()
	cfgPath := "./testdata"
	os.Setenv("FABRIC_CFG_PATH", cfgPath)
	viper.Reset()
	_ = common.InitConfig("test")
	caFile := filepath.Join("certs", "ca.crt")
	viper.Set("orderer.tls.rootcert.file", caFile)
	keyFile := filepath.Join("certs", "client.key")
	viper.Set("orderer.tls.clientKey.file", keyFile)
	certFile := filepath.Join("certs", "client.crt")
	viper.Set("orderer.tls.clientCert.file", certFile)
}

func TestNewOrdererClientFromEnv(t *testing.T) {
	initOrdererTestEnv(t)

	oClient, err := common.NewOrdererClientFromEnv()
	assert.NoError(t, err)
	assert.NotNil(t, oClient)

	viper.Set("orderer.tls.enabled", true)
	oClient, err = common.NewOrdererClientFromEnv()
	assert.NoError(t, err)
	assert.NotNil(t, oClient)

	viper.Set("orderer.tls.enabled", true)
	viper.Set("orderer.tls.clientAuthRequired", true)
	oClient, err = common.NewOrdererClientFromEnv()
	assert.NoError(t, err)
	assert.NotNil(t, oClient)

	// bad key file
	badKeyFile := filepath.Join("certs", "bad.key")
	viper.Set("orderer.tls.clientKey.file", badKeyFile)
	oClient, err = common.NewOrdererClientFromEnv()
	assert.Contains(t, err.Error(), "failed to create OrdererClient from config")
	assert.Nil(t, oClient)

	// bad cert file path
	viper.Set("orderer.tls.clientCert.file", "./nocert.crt")
	oClient, err = common.NewOrdererClientFromEnv()
	assert.Contains(t, err.Error(), "unable to load orderer.tls.clientCert.file")
	assert.Contains(t, err.Error(), "failed to load config for OrdererClient")
	assert.Nil(t, oClient)

	// bad key file path
	viper.Set("orderer.tls.clientKey.file", "./nokey.key")
	oClient, err = common.NewOrdererClientFromEnv()
	assert.Contains(t, err.Error(), "unable to load orderer.tls.clientKey.file")
	assert.Nil(t, oClient)

	// bad ca path
	viper.Set("orderer.tls.rootcert.file", "noroot.crt")
	oClient, err = common.NewOrdererClientFromEnv()
	assert.Contains(t, err.Error(), "unable to load orderer.tls.rootcert.file")
	assert.Nil(t, oClient)

	viper.Reset()
	os.Unsetenv("FABRIC_CFG_PATH")

}

func TestOrdererClient(t *testing.T) {
	initOrdererTestEnv(t)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("error creating server for test: %v", err)
	}
	defer lis.Close()
	viper.Set("orderer.address", lis.Addr().String())
	oClient, err := common.NewOrdererClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create OrdererClient for test: %v", err)
	}
	bc, err := oClient.Broadcast()
	assert.NoError(t, err)
	assert.NotNil(t, bc)
	dc, err := oClient.Deliver()
	assert.NoError(t, err)
	assert.NotNil(t, dc)

	viper.Set("orderer.address", "")

	t.Run("OrdererClient.Broadcast() timeout", func(t *testing.T) {
		t.Parallel()
		oClient2, err2 := common.NewOrdererClientFromEnv()
		if err2 != nil {
			t.Fatalf("failed to create OrdererClient for test: %v", err2)
		}
		_, err2 = oClient2.Broadcast()
		assert.Contains(t, err2.Error(), "orderer client failed to connect")
	})

	t.Run("OrdererClient.Deliver() timeout", func(t *testing.T) {
		t.Parallel()
		oClient3, err3 := common.NewOrdererClientFromEnv()
		if err3 != nil {
			t.Fatalf("failed to create OrdererClient for test: %v", err3)
		}
		_, err3 = oClient3.Deliver()
		assert.Contains(t, err3.Error(), "orderer client failed to connect")
	})

	viper.Reset()
	os.Unsetenv("FABRIC_CFG_PATH")

}
