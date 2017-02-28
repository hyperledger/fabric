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

	"github.com/hyperledger/fabric/msp"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	"github.com/stretchr/testify/assert"
)

func TestMSPConfigManager(t *testing.T) {
	conf, err := msp.GetLocalMspConfig("../../../msp/sampleconfig/", nil, "DEFAULT")
	assert.NoError(t, err)

	// test success:

	// begin/propose/commit
	mspCH := NewMSPConfigHandler()
	mspCH.BeginConfig(t)
	_, err = mspCH.ProposeMSP(t, conf)
	assert.NoError(t, err)
	mspCH.PreCommit(t)
	mspCH.CommitProposals(t)

	msps, err := mspCH.GetMSPs()
	assert.NoError(t, err)

	if msps == nil || len(msps) == 0 {
		t.Fatalf("There are no MSPS in the manager")
	}

	// test failure
	// begin/propose/commit
	mspCH.BeginConfig(t)
	_, err = mspCH.ProposeMSP(t, conf)
	assert.NoError(t, err)
	_, err = mspCH.ProposeMSP(t, &mspprotos.MSPConfig{Config: []byte("BARF!")})
	assert.Error(t, err)
}
