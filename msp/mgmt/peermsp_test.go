/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package mgmt

import (
	"io/ioutil"
	"os"
	"testing"
)

// getTestMSPConfigPath returns the path to sampleconfig for unit tests
func getTestMSPConfigPath() string {
	cfgPath := os.Getenv("PEER_CFG_PATH") + "/msp/sampleconfig/"
	if _, err := ioutil.ReadDir(cfgPath); err != nil {
		cfgPath = os.Getenv("GOPATH") + "/src/github.com/hyperledger/fabric/msp/sampleconfig/"
	}
	return cfgPath
}

func TestLocalMSP(t *testing.T) {
	testMSPConfigPath := getTestMSPConfigPath()
	err := LoadLocalMsp(testMSPConfigPath, nil, "DEFAULT")

	if err != nil {
		t.Fatalf("LoadLocalMsp failed, err %s", err)
	}

	_, err = GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetDefaultSigningIdentity failed, err %s", err)
	}
}
