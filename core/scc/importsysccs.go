/*
Copyright IBM Corp. 2016, 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scc

import (
	//import system chain codes here
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/core/scc/escc"
	"github.com/hyperledger/fabric/core/scc/lccc"
	"github.com/hyperledger/fabric/core/scc/qscc"
	"github.com/hyperledger/fabric/core/scc/vscc"
)

//see systemchaincode_test.go for an example using "sample_syscc"
var systemChaincodes = []*SystemChaincode{
	{
		ChainlessCC: true,
		Enabled:     true,
		Name:        "cscc",
		Path:        "github.com/hyperledger/fabric/core/scc/cscc",
		InitArgs:    [][]byte{[]byte("")},
		Chaincode:   &cscc.PeerConfiger{},
	},
	{
		ChainlessCC: false,
		Enabled:     true,
		Name:        "lccc",
		Path:        "github.com/hyperledger/fabric/core/scc/lccc",
		InitArgs:    [][]byte{[]byte("")},
		Chaincode:   &lccc.LifeCycleSysCC{},
	},
	{
		ChainlessCC: false,
		Enabled:     true,
		Name:        "escc",
		Path:        "github.com/hyperledger/fabric/core/scc/escc",
		InitArgs:    [][]byte{[]byte("")},
		Chaincode:   &escc.EndorserOneValidSignature{},
	},
	{
		ChainlessCC: false,
		Enabled:     true,
		Name:        "vscc",
		Path:        "github.com/hyperledger/fabric/core/scc/vscc",
		InitArgs:    [][]byte{[]byte("")},
		Chaincode:   &vscc.ValidatorOneValidSignature{},
	},
	{
		ChainlessCC: true,
		Enabled:     true,
		Name:        "qscc",
		Path:        "github.com/hyperledger/fabric/core/chaincode/qscc",
		InitArgs:    [][]byte{[]byte("")},
		Chaincode:   &qscc.LedgerQuerier{},
	},
}

//RegisterSysCCs is the hook for system chaincodes where system chaincodes are registered with the fabric
//note the chaincode must still be deployed and launched like a user chaincode will be
func RegisterSysCCs() {
	for _, sysCC := range systemChaincodes {
		RegisterSysCC(sysCC)
	}
}

//DeploySysCCs is the hook for system chaincodes where system chaincodes are registered with the fabric
//note the chaincode must still be deployed and launched like a user chaincode will be
func DeploySysCCs(chainID string) {
	for _, sysCC := range systemChaincodes {
		if !sysCC.ChainlessCC {
			deploySysCC(chainID, sysCC)
		}
	}
}

//DeployChainlessSysCCs is the hook for deploying chainless system chaincodes
//these chaincodes cannot make any ledger calls
func DeployChainlessSysCCs() {
	for _, sysCC := range systemChaincodes {
		if sysCC.ChainlessCC {
			deploySysCC("", sysCC)
		}
	}
}

//DeDeploySysCCs is used in unit tests to stop and remove the system chaincodes before
//restarting them in the same process. This allows clean start of the system
//in the same process
func DeDeploySysCCs(chainID string) {
	for _, sysCC := range systemChaincodes {
		if !sysCC.ChainlessCC {
			DeDeploySysCC(chainID, sysCC)
		}
	}
}

//IsSysCC returns true if the name matches a system chaincode's
//system chaincode names are system, chain wide
func IsSysCC(name string) bool {
	for _, sysCC := range systemChaincodes {
		if sysCC.Name == name {
			return true
		}
	}
	return false
}

//IsChainlessSysCC returns true if the name matches a chainless system chaincode's
//system chaincode names are system, chain wide
func IsChainlessSysCC(name string) bool {
	for _, sysCC := range systemChaincodes {
		if sysCC.Name == name && sysCC.ChainlessCC {
			return true
		}
	}
	return false
}

// MockRegisterSysCCs is used only for testing
// This is needed to break import cycle
func MockRegisterSysCCs(mockSysCCs []*SystemChaincode) []*SystemChaincode {
	orig := systemChaincodes
	systemChaincodes = mockSysCCs
	RegisterSysCCs()
	return orig
}

// MockResetSysCCs restore orig system ccs - is used only for testing
func MockResetSysCCs(mockSysCCs []*SystemChaincode) {
	systemChaincodes = mockSysCCs
}
