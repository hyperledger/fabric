/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/shim"
)

var sysccLogger = flogging.MustGetLogger("sccapi")

// SystemChaincode defines the metadata needed to initialize system chaincode
// when the fabric comes up. SystemChaincodes are installed by adding an
// entry in importsysccs.go
type SystemChaincode struct {
	//Unique name of the system chaincode
	Name string

	// Chaincode holds the actual chaincode instance
	Chaincode shim.Chaincode
}

type SysCCWrapper struct {
	SCC *SystemChaincode
}

func (sccw *SysCCWrapper) Name() string              { return sccw.SCC.Name }
func (sccw *SysCCWrapper) Chaincode() shim.Chaincode { return sccw.SCC.Chaincode }

type SelfDescribingSysCC interface {
	//Unique name of the system chaincode
	Name() string

	// Chaincode returns the underlying chaincode
	Chaincode() shim.Chaincode
}
