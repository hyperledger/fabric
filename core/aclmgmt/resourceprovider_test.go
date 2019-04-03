/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
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

func (pe *mockPolicyEvaluatorImpl) Evaluate(polName string, sd []*protoutil.SignedData) error {
	err, ok := pe.peval[polName]
	if !ok {
		return PolicyNotFound(polName)
	}

	//this could be non nil or some error
	return err
}

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

//go:generate counterfeiter -o mocks/defaultaclprovider.go --fake-name DefaultACLProvider . defaultACLProvider

func TestPolicyBase(t *testing.T) {
	peval := &mockPolicyEvaluatorImpl{pmap: map[string]string{"res": "pol"}, peval: map[string]error{"pol": nil}}
	pprov := newPolicyProvider(peval)
	sProp, _ := protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	err := pprov.CheckACL("pol", sProp)
	assert.NoError(t, err)

	signer := &mocks.SignerSerializer{}
	env, err := protoutil.CreateSignedEnvelope(common.HeaderType_CONFIG, "myc", signer, &common.ConfigEnvelope{}, 0, 0)
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

	sProp, _ := protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	err = pprov.CheckACL("badpolicy", sProp)
	assert.Error(t, err)

	sProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	sProp.ProposalBytes = []byte("bad proposal bytes")
	err = pprov.CheckACL("res", sProp)
	assert.Error(t, err)

	sProp, _ = protoutil.MockSignedEndorserProposalOrPanic("A", &peer.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	prop := &peer.Proposal{}
	if proto.Unmarshal(sProp.ProposalBytes, prop) != nil {
		t.FailNow()
	}
	prop.Header = []byte("bad hdr")
	sProp.ProposalBytes = protoutil.MarshalOrPanic(prop)
	err = pprov.CheckACL("res", sProp)
	assert.Error(t, err)
}

// test to ensure ptypes are processed by default provider
func TestForceDefaultsForPType(t *testing.T) {
	defAclProvider := &mocks.DefaultACLProvider{}
	defAclProvider.CheckACLReturns(nil)
	defAclProvider.IsPtypePolicyReturns(true)
	rp := &resourceProvider{defaultProvider: defAclProvider}
	err := rp.CheckACL("aptype", "somechannel", struct{}{})
	assert.NoError(t, err)
}

func init() {
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not load msp config, err %s", err)
		os.Exit(-1)
		return
	}
}
