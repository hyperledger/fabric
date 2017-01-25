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
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/msp/testutils"
	"github.com/hyperledger/fabric/protos/utils"
)

func TestLocalMSP(t *testing.T) {
	testMSPConfigPath := utils.GetTESTMSPConfigPath()
	err := LoadLocalMsp(testMSPConfigPath)
	if err != nil {
		t.Fatalf("LoadLocalMsp failed, err %s", err)
	}

	_, err = GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetDefaultSigningIdentity failed, err %s", err)
	}
}

// TODO: as soon as proper per-chain MSP support is developed, this test will no longer be required
func TestFakeSetup(t *testing.T) {
	testMSPConfigPath := utils.GetTESTMSPConfigPath()
	err := LoadFakeSetupWithLocalMspAndTestChainMsp(testMSPConfigPath)
	if err != nil {
		t.Fatalf("LoadLocalMsp failed, err %s", err)
	}

	_, err = GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetDefaultSigningIdentity failed, err %s", err)
	}

	msps, err := GetManagerForChain(util.GetTestChainID()).GetMSPs()
	if err != nil {
		t.Fatalf("EnlistedMSPs failed, err %s", err)
	}

	if msps == nil || len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager for chain %s", util.GetTestChainID())
	}
}

func TestGetMSPManagerFromBlock(t *testing.T) {
	testMSPConfigPath := utils.GetTESTMSPConfigPath()
	conf, err := msp.GetLocalMspConfig(testMSPConfigPath)
	if err != nil {
		t.Fatalf("GetLocalMspConfig failed, err %s", err)
	}

	block, err := msptestutils.GetTestBlockFromMspConfig(conf)
	if err != nil {
		t.Fatalf("getTestBlockFromMspConfig failed, err %s", err)
	}

	mgr, err := GetMSPManagerFromBlock("testchainid", block)
	if err != nil {
		t.Fatalf("GetMSPManagerFromBlock failed, err %s", err)
	} else if mgr == nil {
		t.Fatalf("Returned nil manager")
	}

	msps, err := mgr.GetMSPs()
	if err != nil {
		t.Fatalf("EnlistedMSPs failed, err %s", err)
	}

	if msps == nil || len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager for chain %s", util.GetTestChainID())
	}
}
