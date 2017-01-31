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

package msp

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestMSPConfigManager(t *testing.T) {
	conf, err := msp.GetLocalMspConfig("../../../../msp/sampleconfig/", "DEFAULT")
	assert.NoError(t, err)

	confBytes, err := proto.Marshal(conf)
	assert.NoError(t, err)

	ci := &common.ConfigValue{Value: confBytes}

	// test success:

	// begin/propose/commit
	key := "DEFAULT"

	mspCH := &MSPConfigHandler{}
	mspCH.BeginConfig()
	err = mspCH.ProposeConfig(key, ci)
	assert.NoError(t, err)
	mspCH.CommitConfig()

	msps, err := mspCH.GetMSPs()
	assert.NoError(t, err)

	if msps == nil || len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager")
	}

	// test failure
	// begin/propose/commit
	mspCH.BeginConfig()
	err = mspCH.ProposeConfig(key, ci)
	assert.NoError(t, err)
	err = mspCH.ProposeConfig(key, &common.ConfigValue{Value: []byte("BARF!")})
	assert.Error(t, err)
}
