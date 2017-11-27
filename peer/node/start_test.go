/*
Copyright 2017 Hitachi America, Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package node

import (
	"bytes"
	"io/ioutil"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/handlers/library"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestStartCmd(t *testing.T) {
	viper.Set("peer.address", "0.0.0.0:6051")
	viper.Set("peer.listenAddress", "0.0.0.0:6051")
	viper.Set("peer.chaincodeListenAddress", "0.0.0.0:6052")
	viper.Set("peer.fileSystemPath", "/tmp/hyperledger/test")
	viper.Set("chaincode.executetimeout", "30s")
	viper.Set("chaincode.mode", "dev")
	overrideLogModules := []string{"msp", "gossip", "ledger", "cauthdsl", "policies", "grpc"}
	for _, module := range overrideLogModules {
		viper.Set("logging."+module, "INFO")
	}

	msptesttools.LoadMSPSetupForTesting()

	go func() {
		cmd := startCmd()
		assert.NoError(t, cmd.Execute(), "expected to successfully start command")
	}()

	timer := time.NewTimer(time.Second * 3)
	defer timer.Stop()

	// waiting for pid file will be created
loop:
	for {
		select {
		case <-timer.C:
			t.Errorf("timeout waiting for start command")
		default:
			_, err := os.Stat("/tmp/hyperledger/test/peer.pid")
			if err != nil {
				time.Sleep(200 * time.Millisecond)
			} else {
				break loop
			}
		}
	}

	pidFile, err := ioutil.ReadFile("/tmp/hyperledger/test/peer.pid")
	if err != nil {
		t.Fail()
		t.Errorf("can't delete pid file")
	}
	pid, err := strconv.Atoi(string(pidFile))
	killerr := syscall.Kill(pid, syscall.SIGTERM)
	if killerr != nil {
		t.Errorf("Error trying to kill -15 pid %d: %s", pid, killerr)
	}

	os.RemoveAll("/tmp/hyperledger/test")
}

func TestWritePid(t *testing.T) {
	var tests = []struct {
		name     string
		fileName string
		pid      int
		expected bool
	}{
		{
			name:     "readPid success",
			fileName: "/tmp/hyperledger/test/peer.pid",
			pid:      os.Getpid(),
			expected: true,
		},
		{
			name:     "readPid error",
			fileName: "",
			pid:      os.Getpid(),
			expected: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Logf("Running test %s", test.name)
			if test.expected {
				err := writePid(test.fileName, test.pid)
				os.Remove(test.fileName)
				assert.NoError(t, err, "expected to successfully write pid file")
			} else {
				err := writePid(test.fileName, test.pid)
				assert.Error(t, err, "addition of empty pid filename should fail")
			}
		})
	}
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
