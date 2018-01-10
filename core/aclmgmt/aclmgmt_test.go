/*

Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"sync"
	"testing"

	"github.com/hyperledger/fabric/core/aclmgmt/mocks"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"

	"github.com/pkg/errors"
)

//treat each test as an independent isolated one
func reinit() {
	aclProvider = nil
	once = sync.Once{}
}

func registerACLProvider() *mocks.MockACLProvider {
	aclProv := &mocks.MockACLProvider{}
	aclProv.Reset()

	RegisterACLProvider(aclProv)

	return aclProv
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
	err := GetACLProvider().CheckACL(resources.PROPOSE, "somechain", "badidtype")
	assert.Error(t, err, "Expected error")
}

func TestBadResource(t *testing.T) {
	reinit()
	RegisterACLProvider(nil)
	err := GetACLProvider().CheckACL("unknownresource", "somechain", &pb.SignedProposal{})
	assert.Error(t, err, "Expected error")
}

func TestWithProvider(t *testing.T) {
	reinit()
	aclprov := registerACLProvider()
	prop := &pb.SignedProposal{}
	aclprov.On("CheckACL", resources.PROPOSE, "somechain", prop).Return(nil)
	err := GetACLProvider().CheckACL(resources.PROPOSE, "somechain", prop)
	assert.NoError(t, err)
}

func TestBadACL(t *testing.T) {
	reinit()
	aclprov := registerACLProvider()
	prop := &pb.SignedProposal{}
	aclprov.On("CheckACL", resources.PROPOSE, "somechain", prop).Return(errors.New("badacl"))
	err := GetACLProvider().CheckACL(resources.PROPOSE, "somechain", prop)
	assert.Error(t, err, "Expected error")
}
