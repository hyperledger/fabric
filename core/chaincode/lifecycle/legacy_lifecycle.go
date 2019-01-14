/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"
)

// ChaincodeDefinition returns the details for a chaincode by name
func (l *Lifecycle) ChaincodeDefinition(chaincodeName string, qe ledger.SimpleQueryExecutor) (ccprovider.ChaincodeDefinition, error) {
	return l.LegacyImpl.ChaincodeDefinition(chaincodeName, qe)
}

// ChaincodeContainerInfo returns the information necessary to launch a chaincode
func (l *Lifecycle) ChaincodeContainerInfo(chaincodeName string, qe ledger.SimpleQueryExecutor) (*ccprovider.ChaincodeContainerInfo, error) {
	return l.LegacyImpl.ChaincodeContainerInfo(chaincodeName, qe)
}
