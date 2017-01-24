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

package mgmt_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/msp"
	. "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp/utils"
	"github.com/stretchr/testify/assert"
)

func TestMSPConfigManager(t *testing.T) {
	conf, err := msp.GetLocalMspConfig("../sampleconfig/")
	assert.NoError(t, err)

	confBytes, err := proto.Marshal(conf)
	assert.NoError(t, err)

	ci := &common.ConfigurationItem{Key: msputils.MSPKey, Value: confBytes}

	// test success:

	// begin/propose/commit
	var mspCH configtx.Handler
	mspCH = &MSPConfigHandler{}
	mspCH.BeginConfig()
	err = mspCH.ProposeConfig(ci)
	assert.NoError(t, err)
	mspCH.CommitConfig()

	// get the manager we just created
	mgr := mspCH.(*MSPConfigHandler).GetMSPManager()

	msps, err := mgr.GetMSPs()
	assert.NoError(t, err)

	if msps == nil || len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager")
	}

	// test failure
	// begin/propose/commit
	mspCH.BeginConfig()
	err = mspCH.ProposeConfig(ci)
	assert.NoError(t, err)
	err = mspCH.ProposeConfig(&common.ConfigurationItem{Key: msputils.MSPKey, Value: []byte("BARF!")})
	assert.Error(t, err)
}
