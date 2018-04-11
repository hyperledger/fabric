/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mgmt

import (
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
)

func TestGetManagerForChains(t *testing.T) {
	// MSPManager for channel does not exist prior to this call
	mspMgr1 := GetManagerForChain("test")
	// ensure MSPManager is set
	if mspMgr1 == nil {
		t.Fatal("mspMgr1 fail")
	}

	// MSPManager for channel now exists
	mspMgr2 := GetManagerForChain("test")
	// ensure MSPManager returned matches the first result
	if mspMgr2 != mspMgr1 {
		t.Fatal("mspMgr2 != mspMgr1 fail")
	}
}

func TestGetManagerForChains_usingMSPConfigHandlers(t *testing.T) {
	XXXSetMSPManager("foo", msp.NewMSPManager())
	msp2 := GetManagerForChain("foo")
	// return value should be set because the MSPManager was initialized
	if msp2 == nil {
		t.FailNow()
	}
}

func TestGetIdentityDeserializer(t *testing.T) {
	XXXSetMSPManager("baz", msp.NewMSPManager())
	ids := GetIdentityDeserializer("baz")
	assert.NotNil(t, ids)
	ids = GetIdentityDeserializer("")
	assert.NotNil(t, ids)
}

func TestGetLocalSigningIdentityOrPanic(t *testing.T) {
	sid := GetLocalSigningIdentityOrPanic()
	assert.NotNil(t, sid)
}

func TestUpdateLocalMspCache(t *testing.T) {
	// reset localMsp to force it to be initialized on the first call
	localMsp = nil

	// first call should initialize local MSP and returned the cached version
	firstMsp := GetLocalMSP()
	// second call should return the same
	secondMsp := GetLocalMSP()
	// third call should return the same
	thirdMsp := GetLocalMSP()

	// the same (non-cached if not patched) instance
	if thirdMsp != secondMsp {
		t.Fatalf("thirdMsp != secondMsp")
	}
	// first (cached) and second (non-cached) different unless patched
	if firstMsp != secondMsp {
		t.Fatalf("firstMsp != secondMsp")
	}
}

func TestNewMSPMgmtMgr(t *testing.T) {
	err := LoadMSPSetupForTesting()
	assert.Nil(t, err)

	// test for nonexistent channel
	mspMgmtMgr := GetManagerForChain("fake")

	id := GetLocalSigningIdentityOrPanic()
	assert.NotNil(t, id)

	serializedID, err := id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded, got err %s", err)
		return
	}

	idBack, err := mspMgmtMgr.DeserializeIdentity(serializedID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel doesn't exist")
	assert.Nil(t, idBack, "deserialized identity should have been nil")

	// test for existing channel
	mspMgmtMgr = GetManagerForChain(util.GetTestChainID())

	id = GetLocalSigningIdentityOrPanic()
	assert.NotNil(t, id)

	serializedID, err = id.Serialize()
	if err != nil {
		t.Fatalf("Serialize should have succeeded, got err %s", err)
		return
	}

	idBack, err = mspMgmtMgr.DeserializeIdentity(serializedID)
	assert.NoError(t, err)
	assert.NotNil(t, idBack, "deserialized identity should not have been nil")
}

func LoadMSPSetupForTesting() error {
	dir, err := configtest.GetDevMspDir()
	if err != nil {
		return err
	}
	conf, err := msp.GetLocalMspConfig(dir, nil, "SampleOrg")
	if err != nil {
		return err
	}

	err = GetLocalMSP().Setup(conf)
	if err != nil {
		return err
	}

	err = GetManagerForChain(util.GetTestChainID()).Setup([]msp.MSP{GetLocalMSP()})
	if err != nil {
		return err
	}

	return nil
}
