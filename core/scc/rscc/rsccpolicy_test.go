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
package rscc

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

// ------- mocks ---------

//mockPolicyEvaluatorImpl implements policyEvaluator
type mockPolicyEvaluatorImpl struct {
	pmap  map[string]string
	peval map[string]error
}

func (pe *mockPolicyEvaluatorImpl) PolicyRefForResource(resName string) string {
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

func TestRsccPolicyBase(t *testing.T) {
	peval := &mockPolicyEvaluatorImpl{pmap: map[string]string{"res": "pol"}, peval: map[string]error{"pol": nil}}
	pprov := newRsccPolicyProvider("myc", peval)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	err := pprov.CheckACL("pol", sProp)
	assert.NoError(t, err)
}

func TestRsccPolicyBad(t *testing.T) {
	peval := &mockPolicyEvaluatorImpl{pmap: map[string]string{"res": "pol"}, peval: map[string]error{"pol": nil}}
	pprov := newRsccPolicyProvider("myc", peval)

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
