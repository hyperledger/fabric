/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msptesttools

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/stretchr/testify/require"
)

func TestNewTestMSP(t *testing.T) {
	mgr, msp, err := NewTestMSP()
	require.NoError(t, err)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	localMSP := mgmt.GetLocalMSP(cryptoProvider)
	require.Same(t, msp, localMSP)

	msps, err := mgr.GetMSPs()
	require.NoError(t, err)
	require.Len(t, msps, 1)
	require.Same(t, msp, msps["SampleOrg"])
}
