/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/stretchr/testify/require"
)

func TestNewDeserializersManager(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	require.NotNil(t, NewDeserializersManager(mgmt.GetLocalMSP(cryptoProvider)))
}

func TestMspDeserializersManager_Deserialize(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	localMSP := mgmt.GetLocalMSP(cryptoProvider)
	m := NewDeserializersManager(localMSP)

	i, err := localMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	raw, err := i.Serialize()
	require.NoError(t, err)

	i2, err := m.Deserialize(raw)
	require.NoError(t, err)
	require.NotNil(t, i2)
	require.NotNil(t, i2.IdBytes)
	require.Equal(t, m.GetLocalMSPIdentifier(), i2.Mspid)
}

func TestMspDeserializersManager_GetChannelDeserializers(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	m := NewDeserializersManager(mgmt.GetLocalMSP(cryptoProvider))

	deserializers := m.GetChannelDeserializers()
	require.NotNil(t, deserializers)
}

func TestMspDeserializersManager_GetLocalDeserializer(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	localMSP := mgmt.GetLocalMSP(cryptoProvider)
	m := NewDeserializersManager(localMSP)

	i, err := localMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	raw, err := i.Serialize()
	require.NoError(t, err)

	i2, err := m.GetLocalDeserializer().DeserializeIdentity(raw)
	require.NoError(t, err)
	require.NotNil(t, i2)
	require.Equal(t, m.GetLocalMSPIdentifier(), i2.GetMSPIdentifier())
}

func TestMain(m *testing.M) {
	mspDir := configtest.GetDevMspDir()

	testConf, err := msp.GetLocalMspConfig(mspDir, nil, "SampleOrg")
	if err != nil {
		fmt.Printf("Setup should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	cryptoProvider := factory.GetDefault()

	err = mgmt.GetLocalMSP(cryptoProvider).Setup(testConf)
	if err != nil {
		fmt.Printf("Setup for msp should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	mgmt.XXXSetMSPManager("foo", msp.NewMSPManager())
	retVal := m.Run()
	os.Exit(retVal)
}
