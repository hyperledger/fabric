/*
Copyright Hitachi America, Ltd.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/handlers/library"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/peer/node/mock"
	"github.com/hyperledger/fabric/protos/common"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestStartCmd(t *testing.T) {
	defer viper.Reset()
	g := NewGomegaWithT(t)

	tempDir, err := ioutil.TempDir("", "startcmd")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	viper.Set("peer.address", "localhost:6051")
	viper.Set("peer.listenAddress", "0.0.0.0:6051")
	viper.Set("peer.chaincodeListenAddress", "0.0.0.0:6052")
	viper.Set("peer.fileSystemPath", tempDir)
	viper.Set("chaincode.executetimeout", "30s")
	viper.Set("chaincode.mode", "dev")

	msptesttools.LoadMSPSetupForTesting()

	go func() {
		cmd := startCmd()
		assert.NoError(t, cmd.Execute(), "expected to successfully start command")
	}()

	grpcProbe := func(addr string) bool {
		c, err := grpc.Dial(addr, grpc.WithBlock(), grpc.WithInsecure())
		if err == nil {
			c.Close()
			return true
		}
		return false
	}
	g.Eventually(grpcProbe("localhost:6051")).Should(BeTrue())
}

func TestAdminHasSeparateListener(t *testing.T) {
	assert.False(t, adminHasSeparateListener("0.0.0.0:7051", ""))

	assert.Panics(t, func() {
		adminHasSeparateListener("foo", "blabla")
	})

	assert.Panics(t, func() {
		adminHasSeparateListener("0.0.0.0:7051", "blabla")
	})

	assert.False(t, adminHasSeparateListener("0.0.0.0:7051", "0.0.0.0:7051"))
	assert.False(t, adminHasSeparateListener("0.0.0.0:7051", "127.0.0.1:7051"))
	assert.True(t, adminHasSeparateListener("0.0.0.0:7051", "0.0.0.0:7055"))
}

func TestHandlerMap(t *testing.T) {
	config1 := `
  peer:
    handlers:
      authFilters:
        -
          name: filter1
          library: /opt/lib/filter1.so
        -
          name: filter2
  `
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewBuffer([]byte(config1)))
	assert.NoError(t, err)

	libConf := library.Config{}
	err = viperutil.EnhancedExactUnmarshalKey("peer.handlers", &libConf)
	assert.NoError(t, err)
	assert.Len(t, libConf.AuthFilters, 2, "expected two filters")
	assert.Equal(t, "/opt/lib/filter1.so", libConf.AuthFilters[0].Library)
	assert.Equal(t, "filter2", libConf.AuthFilters[1].Name)
}

func TestComputeChaincodeEndpoint(t *testing.T) {
	/*** Scenario 1: chaincodeAddress and chaincodeListenAddress are not set ***/
	viper.Set(chaincodeAddrKey, nil)
	viper.Set(chaincodeListenAddrKey, nil)
	// Scenario 1.1: peer address is 0.0.0.0
	// computeChaincodeEndpoint will return error
	peerAddress0 := "0.0.0.0"
	ccEndpoint, err := computeChaincodeEndpoint(peerAddress0)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)
	// Scenario 1.2: peer address is not 0.0.0.0
	// chaincodeEndpoint will be peerAddress:7052
	peerAddress := "127.0.0.1"
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, peerAddress+":7052", ccEndpoint)

	/*** Scenario 2: set up chaincodeListenAddress only ***/
	// Scenario 2.1: chaincodeListenAddress is 0.0.0.0
	chaincodeListenPort := "8052"
	settingChaincodeListenAddress0 := "0.0.0.0:" + chaincodeListenPort
	viper.Set(chaincodeListenAddrKey, settingChaincodeListenAddress0)
	viper.Set(chaincodeAddrKey, nil)
	// Scenario 2.1.1: peer address is 0.0.0.0
	// computeChaincodeEndpoint will return error
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress0)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)
	// Scenario 2.1.2: peer address is not 0.0.0.0
	// chaincodeEndpoint will be peerAddress:chaincodeListenPort
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, peerAddress+":"+chaincodeListenPort, ccEndpoint)
	// Scenario 2.2: chaincodeListenAddress is not 0.0.0.0
	// chaincodeEndpoint will be chaincodeListenAddress
	settingChaincodeListenAddress := "127.0.0.1:" + chaincodeListenPort
	viper.Set(chaincodeListenAddrKey, settingChaincodeListenAddress)
	viper.Set(chaincodeAddrKey, nil)
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, settingChaincodeListenAddress, ccEndpoint)
	// Scenario 2.3: chaincodeListenAddress is invalid
	// computeChaincodeEndpoint will return error
	settingChaincodeListenAddressInvalid := "abc"
	viper.Set(chaincodeListenAddrKey, settingChaincodeListenAddressInvalid)
	viper.Set(chaincodeAddrKey, nil)
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)

	/*** Scenario 3: set up chaincodeAddress only ***/
	// Scenario 3.1: chaincodeAddress is 0.0.0.0
	// computeChaincodeEndpoint will return error
	chaincodeAddressPort := "9052"
	settingChaincodeAddress0 := "0.0.0.0:" + chaincodeAddressPort
	viper.Set(chaincodeListenAddrKey, nil)
	viper.Set(chaincodeAddrKey, settingChaincodeAddress0)
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)
	// Scenario 3.2: chaincodeAddress is not 0.0.0.0
	// chaincodeEndpoint will be chaincodeAddress
	settingChaincodeAddress := "127.0.0.2:" + chaincodeAddressPort
	viper.Set(chaincodeListenAddrKey, nil)
	viper.Set(chaincodeAddrKey, settingChaincodeAddress)
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, settingChaincodeAddress, ccEndpoint)
	// Scenario 3.3: chaincodeAddress is invalid
	// computeChaincodeEndpoint will return error
	settingChaincodeAddressInvalid := "bcd"
	viper.Set(chaincodeListenAddrKey, nil)
	viper.Set(chaincodeAddrKey, settingChaincodeAddressInvalid)
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)

	/*** Scenario 4: set up both chaincodeAddress and chaincodeListenAddress ***/
	// This scenario will be the same to scenarios 3: set up chaincodeAddress only.
}

func TestResetLoop(t *testing.T) {
	peerLedger := &mock.PeerLedger{}
	peerLedger.GetBlockchainInfoReturnsOnCall(
		0,
		&common.BlockchainInfo{
			Height: uint64(1),
		},
		nil,
	)
	peerLedger.GetBlockchainInfoReturnsOnCall(
		1,
		&common.BlockchainInfo{
			Height: uint64(5),
		},
		nil,
	)
	peerLedger.GetBlockchainInfoReturnsOnCall(
		2,
		&common.BlockchainInfo{
			Height: uint64(11),
		},
		nil,
	)
	peerLedger.GetBlockchainInfoReturnsOnCall(
		3,
		&common.BlockchainInfo{
			Height: uint64(11),
		},
		nil,
	)

	getLedger := &mock.GetLedger{}
	getLedger.Returns(peerLedger)

	resetFilter := &reset{
		reject: true,
	}

	heights := map[string]uint64{
		"testchannel":  uint64(10),
		"testchannel2": uint64(10),
	}

	resetLoop(resetFilter, heights, getLedger.Spy, 1*time.Second)
	assert.False(t, resetFilter.reject)
	assert.Equal(t, 4, peerLedger.GetBlockchainInfoCallCount())
}
