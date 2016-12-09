package mspmgmt

import (
	"testing"

	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos/common"
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

// TODO: as soon as proper per-chain MSP support is developed, this test will have to be changed
func TestGetMSPManagerFromBlock(t *testing.T) {
	err := LoadLocalMsp("../../../msp/sampleconfig/")
	if err != nil {
		t.Fatalf("LoadLocalMsp failed, err %s", err)
	}

	mgr, err := GetMSPManagerFromBlock(&common.Block{ /* TODO: FILLME! */ })
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
