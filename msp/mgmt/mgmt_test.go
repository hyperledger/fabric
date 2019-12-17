/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mgmt

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/sw"
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
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	XXXSetMSPManager("baz", msp.NewMSPManager())
	ids := GetIdentityDeserializer("baz", cryptoProvider)
	assert.NotNil(t, ids)
	ids = GetIdentityDeserializer("", cryptoProvider)
	assert.NotNil(t, ids)
}

func TestGetLocalSigningIdentityOrPanic(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	sid := GetLocalSigningIdentityOrPanic(cryptoProvider)
	assert.NotNil(t, sid)
}

func TestUpdateLocalMspCache(t *testing.T) {
	// reset localMsp to force it to be initialized on the first call
	localMsp = nil

	cryptoProvider := factory.GetDefault()

	// first call should initialize local MSP and returned the cached version
	firstMsp := GetLocalMSP(cryptoProvider)
	// second call should return the same
	secondMsp := GetLocalMSP(cryptoProvider)
	// third call should return the same
	thirdMsp := GetLocalMSP(cryptoProvider)

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
	cryptoProvider, err := LoadMSPSetupForTesting()
	assert.Nil(t, err)

	// test for nonexistent channel
	mspMgmtMgr := GetManagerForChain("fake")

	id := GetLocalSigningIdentityOrPanic(cryptoProvider)
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
	mspMgmtMgr = GetManagerForChain("testchannelid")

	id = GetLocalSigningIdentityOrPanic(cryptoProvider)
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

func LoadMSPSetupForTesting() (bccsp.BCCSP, error) {
	dir := configtest.GetDevMspDir()
	conf, err := msp.GetLocalMspConfig(dir, nil, "SampleOrg")
	if err != nil {
		return nil, err
	}

	cryptoProvider := factory.GetDefault()

	err = GetLocalMSP(cryptoProvider).Setup(conf)
	if err != nil {
		return nil, err
	}

	err = GetManagerForChain("testchannelid").Setup([]msp.MSP{GetLocalMSP(cryptoProvider)})
	if err != nil {
		return nil, err
	}

	return cryptoProvider, nil
}
