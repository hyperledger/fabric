/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package dockercontroller

import (
	"fmt"
	"os"
	"testing"

	"github.com/fsouza/go-dockerclient"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestHostConfig(t *testing.T) {
	config.SetupTestConfig("./../../../peer")
	var hostConfig = new(docker.HostConfig)
	err := viper.UnmarshalKey("vm.docker.hostConfig", hostConfig)
	if err != nil {
		t.Fatalf("Load docker HostConfig wrong, error: %s", err.Error())
	}
	testutil.AssertNotEquals(t, hostConfig.LogConfig, nil)
	testutil.AssertEquals(t, hostConfig.LogConfig.Type, "json-file")
	testutil.AssertEquals(t, hostConfig.LogConfig.Config["max-size"], "50m")
	testutil.AssertEquals(t, hostConfig.LogConfig.Config["max-file"], "5")
}

func TestGetDockerHostConfig(t *testing.T) {
	os.Setenv("HYPERLEDGER_VM_DOCKER_HOSTCONFIG_NETWORKMODE", "overlay")
	os.Setenv("HYPERLEDGER_VM_DOCKER_HOSTCONFIG_CPUSHARES", fmt.Sprint(1024*1024*1024*2))
	config.SetupTestConfig("./../../../peer")
	hostConfig := getDockerHostConfig()
	testutil.AssertNotNil(t, hostConfig)
	testutil.AssertEquals(t, hostConfig.NetworkMode, "overlay")
	testutil.AssertEquals(t, hostConfig.LogConfig.Type, "json-file")
	testutil.AssertEquals(t, hostConfig.LogConfig.Config["max-size"], "50m")
	testutil.AssertEquals(t, hostConfig.LogConfig.Config["max-file"], "5")
	testutil.AssertEquals(t, hostConfig.Memory, int64(1024*1024*1024*2))
	testutil.AssertEquals(t, hostConfig.CPUShares, int64(1024*1024*1024*2))
}
