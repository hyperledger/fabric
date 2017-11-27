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

func TestCreateChaincodeServerForChaincodeAddress(t *testing.T) {
	var peerAddress, ccListenAddress, ccAddress string
	peerAddress = "127.0.0.1"
	peerAddress0 := "0.0.0.0"
	defaultAddress := "127.0.0.1:7052"
	defaultAddress0 := "0.0.0.0:7052"
	defaultEqualAddress0 := "[::]:7052"

	/*** Scenario 1: do not set chaincodeAddress or chaincodeListenAddress ***/
	// Scenario 1.1: peer address is not 0.0.0.0
	// ccListenAddress will be the default one (peerAddress:7052)
	// ccAddress will be the same to ccListenAddress
	viper.Set(chaincodeListenAddrKey, nil)
	viper.Set(chaincodeAddrKey, nil)
	ccListenAddress, ccAddress, err := createChaincodeServerReturnAddress(peerAddress)
	assert.Equal(t, defaultAddress, ccListenAddress)
	assert.Equal(t, defaultAddress, ccAddress)
	assert.NoError(t, err)
	// Scenario 1.1: peer address is 0.0.0.0
	// ccListenAddress will be the default one (peerAddress:7052)
	// ccAddress will not be set and get an error
	viper.Set(chaincodeListenAddrKey, nil)
	viper.Set(chaincodeAddrKey, nil)
	ccListenAddress, ccAddress, err = createChaincodeServerReturnAddress(peerAddress0)
	assert.True(t, true,
		defaultAddress0 == ccListenAddress ||
			defaultEqualAddress0 == ccListenAddress)
	assert.Error(t, err)
	assert.Equal(t, "", ccAddress)

	/*** Scenario 2: set up chaincodeListenAddress only ***/
	// Scenario 2.1: chaincodeListenAddress is not 0.0.0.0 nor "::"
	// ccListenAddress and ccAddress will be chaincodeListenAddress
	settingChaincodeListenAddress := "127.0.0.1:8052"
	viper.Set(chaincodeListenAddrKey, settingChaincodeListenAddress)
	viper.Set(chaincodeAddrKey, nil)
	ccListenAddress, ccAddress, err = createChaincodeServerReturnAddress(peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, settingChaincodeListenAddress, ccListenAddress)
	assert.Equal(t, settingChaincodeListenAddress, ccAddress)
	// Scenario 2.2: chaincodeListenAddress is 0.0.0.0 and peerAddress is not 0.0.0.0
	// ccListenAddress will be chaincodeListenAddress
	// ccAddress will be peerAddress:8052
	// Tips: 0.0.0.0:8052 is the equal to [::]:8052
	settingChaincodeListenAddress = "0.0.0.0:8052"
	settingEqualChaincodeListenAddress := "[::]:8052"
	viper.Set(chaincodeListenAddrKey, settingChaincodeListenAddress)
	viper.Set(chaincodeAddrKey, nil)
	ccListenAddress, ccAddress, err = createChaincodeServerReturnAddress(peerAddress)
	assert.NoError(t, err)
	assert.True(t, true,
		ccListenAddress == settingChaincodeListenAddress ||
			ccListenAddress == settingEqualChaincodeListenAddress)
	assert.Equal(t, peerAddress+":8052", ccAddress)
	// Scenario 2.3: both chaincodeListenAddress and peerAddress are 0.0.0.0
	// ccListenAddress will be chaincodeListenAddress
	// ccAddress will not be set and get an error
	viper.Set(chaincodeListenAddrKey, settingChaincodeListenAddress)
	viper.Set(chaincodeAddrKey, nil)
	ccListenAddress, ccAddress, err = createChaincodeServerReturnAddress(peerAddress0)
	assert.Error(t, err)
	assert.True(t, true,
		ccListenAddress == settingChaincodeListenAddress ||
			ccListenAddress == settingEqualChaincodeListenAddress)
	assert.Equal(t, "", ccAddress)

	/*** Scenario 3: set up chaincodeAddress only ***/
	// Scenario 3.1: chaincodeAddress is valid
	// ccListenAddress will be the default one (peerAddress:7052)
	// ccAddress will be set
	settingChaincodeAddress := "127.0.0.2:8052"
	viper.Set(chaincodeAddrKey, settingChaincodeAddress)
	viper.Set(chaincodeListenAddrKey, nil)
	ccListenAddress, ccAddress, err = createChaincodeServerReturnAddress(peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, defaultAddress, ccListenAddress)
	assert.Equal(t, settingChaincodeAddress, ccAddress)
	// Scenario 3.2: chaincodeAddress is invalid
	// ccListenAddress will be the default one (peerAddress:7052)
	// ccAddress will not be set and get an error
	viper.Set(chaincodeAddrKey, "abc")
	viper.Set(chaincodeListenAddrKey, nil)
	ccListenAddress, ccAddress, err = createChaincodeServerReturnAddress(peerAddress)
	assert.Error(t, err)
	assert.Equal(t, defaultAddress, ccListenAddress)
	assert.Equal(t, "", ccAddress)

	/*** Scenario 4: set up both chaincodeListenAddress and chaincodeAddress ***/
	// ccListenAddress and ccAddress will be the corresponding values
	settingChaincodeListenAddress = "127.0.0.1:8052"
	viper.Set(chaincodeListenAddrKey, settingChaincodeListenAddress)
	settingChaincodeAddress = "127.0.0.2:8052"
	viper.Set(chaincodeAddrKey, settingChaincodeAddress)
	ccListenAddress, ccAddress, err = createChaincodeServerReturnAddress(peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, settingChaincodeListenAddress, ccListenAddress)
	assert.Equal(t, settingChaincodeAddress, ccAddress)
}

// TestComputeChaincodeEndpointForInvalidCCListenAddr will test those codes
// which are not covered by TestCreateChaincodeServerForChaincodeAddress
func TestComputeChaincodeEndpointForInvalidCCListenAddr(t *testing.T) {
	// Scenario 1: chaincodeAddress and chaincodeListenAddress are not set
	// Scenario 1.1: peer address is 0.0.0.0
	// computeChaincodeEndpoint will return error
	viper.Set(chaincodeAddrKey, nil)
	viper.Set(chaincodeListenAddrKey, nil)
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

	// Scenario 2: chaincodeListenAddress is invalid and chaincodeAddress is not set
	viper.Set(chaincodeAddrKey, nil)
	viper.Set(chaincodeListenAddrKey, "abc")
	ccEndpoint, err = computeChaincodeEndpoint(peerAddress)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)
}

func createChaincodeServerReturnAddress(peerHostname string) (ccListenAddress, ccAddress string, ccEpFuncErr error) {
	ccSrv, ccEpFunc := createChaincodeServer(nil, peerHostname)
	ccListenAddress = ccSrv.Address()
	// release listener
	ccSrv.Listener().Close()
	endPoint, ccEpFuncErr := ccEpFunc()
	if ccEpFuncErr != nil {
		return ccListenAddress, "", ccEpFuncErr
	}
	ccAddress = endPoint.Address
	return ccListenAddress, ccAddress, ccEpFuncErr
}
