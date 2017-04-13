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
	"fmt"
	"os"
	"testing"

	msp2 "github.com/hyperledger/fabric/common/config/msp"
	"github.com/hyperledger/fabric/core/config"
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

	mspDir, err := config.GetDevMspDir()
	if err != nil {
		fmt.Printf("Error getting DevMspDir: %s", err)
		os.Exit(-1)
	}

	testConf, err := msp.GetLocalMspConfig(mspDir, nil, "DEFAULT")
	if err != nil {
		fmt.Printf("Setup should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	err = GetLocalMSP().Setup(testConf)
	if err != nil {
		fmt.Printf("Setup for msp should have succeeded, got err %s instead", err)
		os.Exit(-1)
	}

	XXXSetMSPManager("foo", &msp2.MSPConfigHandler{MSPManager: msp.NewMSPManager()})
	retVal := m.Run()
	os.Exit(retVal)
}
