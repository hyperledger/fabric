/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"fmt"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

type mockACLProvider struct {
	retErr error
}

func (m *mockACLProvider) CheckACL(resName string, channelID string, idinfo interface{}) error {
	return m.retErr
}

func (e *mockACLProvider) GenerateSimulationResults(txEnvelop *common.Envelope, simulator ledger.TxSimulator) error {
	return nil
}

//treat each test as an independent isolated one
func reinit() {
	aclProvider = nil
	once = sync.Once{}
}

func TestACLProcessor(t *testing.T) {
	reinit()
	assert.NotNil(t, GetConfigTxProcessor().GenerateSimulationResults(nil, nil), "Expected non-nil error")
	RegisterACLProvider(nil)
	assert.Nil(t, GetConfigTxProcessor().GenerateSimulationResults(nil, nil), "Expected nil error")
}

func TestPanicOnUnregistered(t *testing.T) {
	reinit()
	assert.Panics(t, func() {
		GetACLProvider()
	}, "Should have paniced on unregistered call")
}

func TestRegisterNilProvider(t *testing.T) {
	reinit()
	RegisterACLProvider(nil)
	assert.NotNil(t, GetACLProvider(), "Expected non-nil retval")
}

func TestBadID(t *testing.T) {
	reinit()
	RegisterACLProvider(nil)
	err := GetACLProvider().CheckACL(PROPOSE, "somechain", "badidtype")
	assert.Error(t, err, "Expected error")
}

func TestBadResource(t *testing.T) {
	reinit()
	RegisterACLProvider(nil)
	err := GetACLProvider().CheckACL("unknownresource", "somechain", &pb.SignedProposal{})
	assert.Error(t, err, "Expected error")
}

func TestOverride(t *testing.T) {
	reinit()
	RegisterACLProvider(nil)
	GetACLProvider().(*aclMgmtImpl).aclOverrides[PROPOSE] = func(res, c string, idinfo interface{}) error {
		return nil
	}
	err := GetACLProvider().CheckACL(PROPOSE, "somechain", &pb.SignedProposal{})
	assert.NoError(t, err)
	delete(GetACLProvider().(*aclMgmtImpl).aclOverrides, PROPOSE)
}

func TestWithProvider(t *testing.T) {
	reinit()
	RegisterACLProvider(&mockACLProvider{})
	err := GetACLProvider().CheckACL(PROPOSE, "somechain", &pb.SignedProposal{})
	assert.NoError(t, err)
}

func TestBadACL(t *testing.T) {
	reinit()
	RegisterACLProvider(&mockACLProvider{retErr: fmt.Errorf("badacl")})
	err := GetACLProvider().CheckACL(PROPOSE, "somechain", &pb.SignedProposal{})
	assert.Error(t, err, "Expected error")
}
