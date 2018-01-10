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
package aclmgmt

import (
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func newPolicyProvider(pEvaluator policyEvaluator) aclmgmtPolicyProvider {
	return &aclmgmtPolicyProviderImpl{pEvaluator}
}

// ------- mocks ---------

//mockPolicyEvaluatorImpl implements policyEvaluator
type mockPolicyEvaluatorImpl struct {
	pmap  map[string]string
	peval map[string]error
}

func (pe *mockPolicyEvaluatorImpl) PolicyRefForAPI(resName string) string {
	return pe.pmap[resName]
}

func (pe *mockPolicyEvaluatorImpl) Evaluate(polName string, sd []*common.SignedData) error {
	err, ok := pe.peval[polName]
	if !ok {
		return PolicyNotFound(polName)
	}

	//this could be non nil or some error
	return err
}

func TestPolicyBase(t *testing.T) {
	peval := &mockPolicyEvaluatorImpl{pmap: map[string]string{"res": "pol"}, peval: map[string]error{"pol": nil}}
	pprov := newPolicyProvider(peval)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	err := pprov.CheckACL("pol", sProp)
	assert.NoError(t, err)

	env, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG, "myc", localmsp.NewSigner(), &common.ConfigEnvelope{}, 0, 0)
	assert.NoError(t, err)
	err = pprov.CheckACL("pol", env)
	assert.NoError(t, err)
}

func TestPolicyBad(t *testing.T) {
	peval := &mockPolicyEvaluatorImpl{pmap: map[string]string{"res": "pol"}, peval: map[string]error{"pol": nil}}
	pprov := newPolicyProvider(peval)

	//bad policy
	err := pprov.CheckACL("pol", []byte("not a signed proposal"))
	assert.Error(t, err, InvalidIdInfo("pol").Error())

	sProp, _ := utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	err = pprov.CheckACL("badpolicy", sProp)
	assert.Error(t, err)

	sProp, _ = utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.ProposalBytes = []byte("bad proposal bytes")
	err = pprov.CheckACL("res", sProp)
	assert.Error(t, err)

	sProp, _ = utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	prop := &peer.Proposal{}
	if proto.Unmarshal(sProp.ProposalBytes, prop) != nil {
		t.FailNow()
	}
	prop.Header = []byte("bad hdr")
	sProp.ProposalBytes = utils.MarshalOrPanic(prop)
	err = pprov.CheckACL("res", sProp)
	assert.Error(t, err)
}

func init() {
	var err error
	// setup the MSP manager so that we can sign/verify
	err = msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not load msp config, err %s", err)
		os.Exit(-1)
		return
	}
}
