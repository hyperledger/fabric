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

	//Path to the system chaincode; currently not used
	Path string

	//InitArgs initialization arguments to startup the system chaincode
	InitArgs [][]byte

	// Chaincode holds the actual chaincode instance
	Chaincode shim.Chaincode

	// InvokableExternal keeps track of whether
	// this system chaincode can be invoked
	// through a proposal sent to this peer
	InvokableExternal bool

	// InvokableCC2CC keeps track of whether
	// this system chaincode can be invoked
	// by way of a chaincode-to-chaincode
	// invocation
	InvokableCC2CC bool

	// Enabled a convenient switch to enable/disable system chaincode without
	// having to remove entry from importsysccs.go
	Enabled bool
}

type SysCCWrapper struct {
	SCC *SystemChaincode
}

func (sccw *SysCCWrapper) Name() string              { return sccw.SCC.Name }
func (sccw *SysCCWrapper) Path() string              { return sccw.SCC.Path }
func (sccw *SysCCWrapper) InitArgs() [][]byte        { return sccw.SCC.InitArgs }
func (sccw *SysCCWrapper) Chaincode() shim.Chaincode { return sccw.SCC.Chaincode }
func (sccw *SysCCWrapper) InvokableExternal() bool   { return sccw.SCC.InvokableExternal }
func (sccw *SysCCWrapper) InvokableCC2CC() bool      { return sccw.SCC.InvokableCC2CC }
func (sccw *SysCCWrapper) Enabled() bool             { return sccw.SCC.Enabled }

type SelfDescribingSysCC interface {
	//Unique name of the system chaincode
	Name() string

	//Path to the system chaincode; currently not used
	Path() string

	//InitArgs initialization arguments to startup the system chaincode
	InitArgs() [][]byte

	// Chaincode returns the underlying chaincode
	Chaincode() shim.Chaincode

	// InvokableExternal keeps track of whether
	// this system chaincode can be invoked
	// through a proposal sent to this peer
	InvokableExternal() bool

	// InvokableCC2CC keeps track of whether
	// this system chaincode can be invoked
	// by way of a chaincode-to-chaincode
	// invocation
	InvokableCC2CC() bool

	// Enabled a convenient switch to enable/disable system chaincode without
	// having to remove entry from importsysccs.go
	Enabled() bool
}
