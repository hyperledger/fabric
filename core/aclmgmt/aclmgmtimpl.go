/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

var aclMgmtLogger = flogging.MustGetLogger("aclmgmt")

type aclMethod func(resName string, channelID string, idinfo interface{}) error

//implementation of aclMgmt
type aclMgmtImpl struct {
	//by default resources are managed by RSCC. However users may set aclMethod for
	//a resource and bypass RSCC if necessary
	aclOverrides map[string]aclMethod
}

var rscc ACLProvider

//CheckACL checks the ACL for the resource for the channel using the
//idinfo. idinfo is an object such as SignedProposal from which an
//id can be extracted for testing against a policy
func (am *aclMgmtImpl) CheckACL(resName string, channelID string, idinfo interface{}) error {
	aclMeth := am.aclOverrides[resName]
	if aclMeth != nil {
		return aclMeth(resName, channelID, idinfo)
	}

	if rscc == nil {
		panic("-----RegisterACLProvider not called ----")
	}

	return rscc.CheckACL(resName, channelID, idinfo)
}

func newACLMgmt(r ACLProvider) ACLProvider {
	rscc = r
	if rscc == nil {
		rscc = newDefaultACLProvider()
	}

	//by default overrides are not set and all acl checks are referred to rscc
	return &aclMgmtImpl{aclOverrides: make(map[string]aclMethod)}
}

func (am *aclMgmtImpl) GenerateSimulationResults(txEnvelop *common.Envelope, simulator ledger.TxSimulator, initializingLedger bool) error {
	if rscc == nil {
		panic("-----RegisterACLProvider not called ----")
	}

	return rscc.GenerateSimulationResults(txEnvelop, simulator, initializingLedger)
}
