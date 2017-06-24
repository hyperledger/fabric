/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package state

import (
	"testing"

	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/assert"
)

func init() {
	util.SetupTestLogging()
}

func TestNewNodeMetastate(t *testing.T) {
	metastate := NewNodeMetastate(0)
	assert.Equal(t, metastate.Height(), uint64(0))
}

func TestNodeMetastateImpl_Update(t *testing.T) {
	metastate := NewNodeMetastate(0)
	assert.Equal(t, metastate.Height(), uint64(0))
	metastate.Update(10)
	assert.Equal(t, metastate.Height(), uint64(10))
}

// Test node metastate encoding
func TestNodeMetastateImpl_Bytes(t *testing.T) {
	metastate := NewNodeMetastate(0)
	// Encode state into bytes and check there is no errors
	_, err := metastate.Bytes()
	assert.NoError(t, err)
}

// Check the deserialization of the meta stats structure
func TestNodeMetastate_FromBytes(t *testing.T) {
	metastate := NewNodeMetastate(0)
	// Serialize into bytes array
	bytes, err := metastate.Bytes()
	assert.NoError(t, err)
	if bytes == nil {
		t.Fatal("Was not able to serialize meta state into byte array.")
	}

	// Deserialize back and check, that state still have same
	// height value
	state, err := FromBytes(bytes)
	assert.NoError(t, err)

	assert.Equal(t, state.Height(), uint64(0))

	// Update state to the new height and serialize it again
	state.Update(17)
	bytes, err = state.Bytes()
	assert.NoError(t, err)
	if bytes == nil {
		t.Fatal("Was not able to serialize meta state into byte array.")
	}

	// Restore state from byte array and validate
	// that stored height is still the same
	updatedState, err := FromBytes(bytes)
	assert.NoError(t, err)
	assert.Equal(t, updatedState.Height(), uint64(17))
}
