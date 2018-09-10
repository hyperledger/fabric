/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/cauthdsl"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/statebased"
	"github.com/stretchr/testify/assert"
)

func TestAddOrg(t *testing.T) {
	// add an org
	ep, err := statebased.NewStateEP(nil)
	assert.NoError(t, err)
	err = ep.AddOrgs(statebased.RoleTypePeer, "Org1")
	assert.NoError(t, err)

	// bad role type
	err = ep.AddOrgs("unknown", "Org1")
	assert.Equal(t, &statebased.RoleTypeDoesNotExistError{RoleType: statebased.RoleType("unknown")}, err)
	assert.EqualError(t, err, "role type unknown does not exist")

	epBytes, err := ep.Policy()
	assert.NoError(t, err)
	expectedEP := cauthdsl.SignedByMspPeer("Org1")
	expectedEPBytes, err := proto.Marshal(expectedEP)
	assert.NoError(t, err)
	assert.Equal(t, expectedEPBytes, epBytes)
}

func TestListOrgs(t *testing.T) {
	expectedEP := cauthdsl.SignedByMspPeer("Org1")
	expectedEPBytes, err := proto.Marshal(expectedEP)
	assert.NoError(t, err)

	// retrieve the orgs
	ep, err := statebased.NewStateEP(expectedEPBytes)
	orgs := ep.ListOrgs()
	assert.Equal(t, []string{"Org1"}, orgs)
}

func TestDelAddOrg(t *testing.T) {
	expectedEP := cauthdsl.SignedByMspPeer("Org1")
	expectedEPBytes, err := proto.Marshal(expectedEP)
	assert.NoError(t, err)
	ep, err := statebased.NewStateEP(expectedEPBytes)

	// retrieve the orgs
	orgs := ep.ListOrgs()
	assert.Equal(t, []string{"Org1"}, orgs)

	// mod the endorsement policy
	ep.AddOrgs(statebased.RoleTypePeer, "Org2")
	ep.DelOrgs("Org1")

	// check whether what is stored is correct
	epBytes, err := ep.Policy()
	assert.NoError(t, err)
	expectedEP = cauthdsl.SignedByMspPeer("Org2")
	expectedEPBytes, err = proto.Marshal(expectedEP)
	assert.NoError(t, err)
	assert.Equal(t, expectedEPBytes, epBytes)
}
