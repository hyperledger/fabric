/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rscc

import (
	"github.com/hyperledger/fabric/protos/common"
)

//TODO - PLACE HOLDER FOR RESOURCE POLICY INTEGRATION.

//rsccPolicyProvider is the basic policy provider for RSCC. It is an ACLProvider
type rsccPolicyProvider interface {
	GetPolicyName(resName string) string
	CheckACL(resName string, idinfo interface{}) error
}

//rsccPolicyProviderImpl holds the bytes from state of the ledger
type rsccPolicyProviderImpl struct {
}

//GetPolicyName returns the policy name given the resource string
func (rp *rsccPolicyProviderImpl) GetPolicyName(resName string) string {
	return ""
}

func newRsccPolicyProvider(cg *common.ConfigGroup) (*rsccPolicyProviderImpl, error) {
	return &rsccPolicyProviderImpl{}, nil
}

//CheckACL rscc implements AClProvider's CheckACL interface so it can be registered
//as a provider with aclmgmt
func (rp *rsccPolicyProviderImpl) CheckACL(polName string, idinfo interface{}) error {
	rsccLogger.Debugf("acl check(%s)", polName)
	return nil
}
