/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"github.com/hyperledger/fabric/core/common/ccprovider"
)

//DeploySysCCs is the hook for system chaincodes where system chaincodes are registered with the fabric
//note the chaincode must still be deployed and launched like a user chaincode will be
func (p *Provider) DeploySysCCs(chainID string, ccp ccprovider.ChaincodeProvider) {
	for _, sysCC := range p.SysCCs {
		deploySysCC(chainID, ccp, sysCC)
	}
}

//DeDeploySysCCs is used in unit tests to stop and remove the system chaincodes before
//restarting them in the same process. This allows clean start of the system
//in the same process
func (p *Provider) DeDeploySysCCs(chainID string, ccp ccprovider.ChaincodeProvider) {
	for _, sysCC := range p.SysCCs {
		deDeploySysCC(chainID, ccp, sysCC)
	}
}
