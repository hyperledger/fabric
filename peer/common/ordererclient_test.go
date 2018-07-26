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
	"time"

	"github.com/hyperledger/fabric/peer/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func initOrdererTestEnv(t *testing.T) (cleanup func()) {
	t.Helper()
	cfgPath := "./testdata"
	os.Setenv("FABRIC_CFG_PATH", cfgPath)
	viper.Reset()
	_ = common.InitConfig("test")

	return func() {
		err := os.Unsetenv("FABRIC_CFG_PATH")
		assert.NoError(t, err)
		viper.Reset()
	}
}

func TestNewOrdererClientFromEnv(t *testing.T) {
	cleanup := initOrdererTestEnv(t)
	defer cleanup()

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
}

func TestOrdererClient(t *testing.T) {
	cleanup := initOrdererTestEnv(t)
	defer cleanup()

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
		assert.Contains(t, err.Error(), "orderer client failed to connect")
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
		assert.Contains(t, err.Error(), "orderer client failed to connect")
	})
}
