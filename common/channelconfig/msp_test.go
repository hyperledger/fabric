/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	mspprotos "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/require"
)

func TestMSPConfigManager(t *testing.T) {
	mspDir := configtest.GetDevMspDir()
	conf, err := msp.GetLocalMspConfig(mspDir, nil, "SampleOrg")
	require.NoError(t, err)

	// test success:

	mspVers := []msp.MSPVersion{msp.MSPv1_0, msp.MSPv1_1}

	for _, ver := range mspVers {
		mspCH := NewMSPConfigHandler(ver, factory.GetDefault())

		_, err = mspCH.ProposeMSP(conf)
		require.NoError(t, err)

		mgr, err := mspCH.CreateMSPManager()
		require.NoError(t, err)
		require.NotNil(t, mgr)

		msps, err := mgr.GetMSPs()
		require.NoError(t, err)

		if len(msps) == 0 {
			t.Fatalf("There are no MSPS in the manager")
		}

		for _, mspInst := range msps {
			require.Equal(t, mspInst.GetVersion(), msp.MSPVersion(ver))
		}
	}
}

func TestMSPConfigFailure(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	mspCH := NewMSPConfigHandler(msp.MSPv1_0, cryptoProvider)

	// begin/propose/commit
	t.Run("Bad proto", func(t *testing.T) {
		_, err := mspCH.ProposeMSP(&mspprotos.MSPConfig{Config: []byte("BARF!")})
		require.Error(t, err)
	})

	t.Run("Bad MSP Type", func(t *testing.T) {
		_, err := mspCH.ProposeMSP(&mspprotos.MSPConfig{Type: int32(10)})
		require.Error(t, err)
	})
}
