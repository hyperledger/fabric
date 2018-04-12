/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mgmt

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
)

func TestNewDeserializersManager(t *testing.T) {
	assert.NotNil(t, NewDeserializersManager())
}

func TestMspDeserializersManager_Deserialize(t *testing.T) {
	m := NewDeserializersManager()

	i, err := GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err)
	raw, err := i.Serialize()
	assert.NoError(t, err)

	i2, err := m.Deserialize(raw)
	assert.NoError(t, err)
	assert.NotNil(t, i2)
	assert.NotNil(t, i2.IdBytes)
	assert.Equal(t, m.GetLocalMSPIdentifier(), i2.Mspid)
}

func TestMspDeserializersManager_GetChannelDeserializers(t *testing.T) {
	m := NewDeserializersManager()

	deserializers := m.GetChannelDeserializers()
	assert.NotNil(t, deserializers)
}

func TestMspDeserializersManager_GetLocalDeserializer(t *testing.T) {
	m := NewDeserializersManager()

	i, err := GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err)
	raw, err := i.Serialize()
	assert.NoError(t, err)

	i2, err := m.GetLocalDeserializer().DeserializeIdentity(raw)
	assert.NoError(t, err)
	assert.NotNil(t, i2)
	assert.Equal(t, m.GetLocalMSPIdentifier(), i2.GetMSPIdentifier())
}

func TestMain(m *testing.M) {

	mspDir, err := configtest.GetDevMspDir()
	if err != nil {
		fmt.Printf("Error getting DevMspDir: %s", err)
		os.Exit(-1)
	}

	testConf, err := msp.GetLocalMspConfig(mspDir, nil, "SampleOrg")
	if err != nil {
		fmt.Printf("Setup should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	err = GetLocalMSP().Setup(testConf)
	if err != nil {
		fmt.Printf("Setup for msp should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	XXXSetMSPManager("foo", msp.NewMSPManager())
	retVal := m.Run()
	os.Exit(retVal)
}
