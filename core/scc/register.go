// +build !pluginsenabled !cgo darwin,!go1.10 linux,!go1.9 linux,ppc64le,!go1.10

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/common/ccprovider"
)

// CreateSysCCs creates all of the system chaincodes which are compiled into fabric
func CreateSysCCs(ccp ccprovider.ChaincodeProvider, p *Provider, aclProvider aclmgmt.ACLProvider) []*SystemChaincode {
	return builtInSystemChaincodes(ccp, p, aclProvider)
}
