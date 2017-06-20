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
	"io/ioutil"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

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
