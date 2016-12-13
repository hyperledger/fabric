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

package sa

import (
	"testing"

	"fmt"
	"os"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	// Setup the MSP manager so that we can sign/verify
	mspMgrConfigDir := "./../../../msp/sampleconfig/"
	err := mgmt.LoadFakeSetupWithLocalMspAndTestChainMsp(mspMgrConfigDir)
	if err != nil {
		fmt.Printf("Failed LoadFakeSetupWithLocalMspAndTestChainMsp [%s]", err)
		os.Exit(-1)
	}
	os.Exit(m.Run())
}

func TestMspSecurityAdvisor_OrgByPeerIdentity(t *testing.T) {
	id, err := mgmt.GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Failed getting local default signing identity")
	identityRaw, err := id.Serialize()
	assert.NoError(t, err, "Failed serializing local default signing identity")

	advisor := NewSecurityAdvisor()
	orgIdentity := advisor.OrgByPeerIdentity(api.PeerIdentityType(identityRaw))
	assert.NotNil(t, orgIdentity, "Organization for identity must be different from nil")
}
