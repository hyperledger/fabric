package mspmgmt

import (
	"testing"

	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/msp/testutils"
)

func TestLocalMSP(t *testing.T) {
	err := LoadLocalMsp("../../../msp/sampleconfig/")
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
	err := LoadFakeSetupWithLocalMspAndTestChainMsp("../../../msp/sampleconfig/")
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
	conf, err := msp.GetLocalMspConfig("../../../msp/sampleconfig/")
	if err != nil {
		t.Fatalf("GetLocalMspConfig failed, err %s", err)
	}

	block, err := msptestutils.GetTestBlockFromMspConfig(conf)
	if err != nil {
		t.Fatalf("getTestBlockFromMspConfig failed, err %s", err)
	}

	mgr, err := GetMSPManagerFromBlock(block)
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
