/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mgmt

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/require"
)

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

func TestLocalMSP(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	mspDir := configtest.GetDevMspDir()
	conf, err := msp.GetLocalMspConfig(mspDir, nil, "SampleOrg")
	require.NoError(t, err, "failed to get local MSP config")
	err = GetLocalMSP(cryptoProvider).Setup(conf)
	require.NoError(t, err, "failed to setup local MSP")

	_, err = GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	require.NoError(t, err, "failed to get default signing identity")
}

func TestMain(m *testing.M) {
	mspDir := configtest.GetDevMspDir()

	testConf, err := msp.GetLocalMspConfig(mspDir, nil, "SampleOrg")
	if err != nil {
		fmt.Printf("Setup should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	cryptoProvider := factory.GetDefault()

	err = GetLocalMSP(cryptoProvider).Setup(testConf)
	if err != nil {
		fmt.Printf("Setup for msp should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	retVal := m.Run()
	os.Exit(retVal)
}
