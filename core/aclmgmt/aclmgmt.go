/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/common"
)

var aclLogger = flogging.MustGetLogger("aclmgmt")

type ACLProvider interface {
	//the provider also provides config processor to build state from
	//config
	customtx.Processor

	//CheckACL checks the ACL for the resource for the channel using the
	//idinfo. idinfo is an object such as SignedProposal from which an
	//id can be extracted for testing against a policy
	CheckACL(resName string, channelID string, idinfo interface{}) error
}

//---------- custom tx processor initialized once by peer -------
var configtxLock sync.RWMutex

type AclMgmtConfigTxProcessor struct {
}

var aclMgmtCfgTxProcessor = &AclMgmtConfigTxProcessor{}

//GenerateSimulationResults this is just a proxy to delegate registered aclProvider.
//Need this as aclmgmt is initialized with ledger initialization as required by ledger
func (*AclMgmtConfigTxProcessor) GenerateSimulationResults(txEnvelop *common.Envelope, simulator ledger.TxSimulator, initializingLedger bool) error {
	configtxLock.RLock()
	defer configtxLock.RUnlock()

	//this should not be nil (aclProvider is initialized at the outset to either
	//rscc or default)
	if aclProvider != nil {
		return aclProvider.GenerateSimulationResults(txEnvelop, simulator, initializingLedger)
	}

	return fmt.Errorf("warning! call to handle config tx before setting ACL provider")
}

//GetConfigTxProcessor initialized at peer startup with ledgermgmt to receive config blocks
//for channels
func GetConfigTxProcessor() customtx.Processor {
	aclLogger.Info("Initializing CONFIG processor")
	return aclMgmtCfgTxProcessor
}

//---------- ACLProvider intialized once SCCs are brought up by peer ---------
var aclProvider ACLProvider

var once sync.Once

//RegisterACLProvider will be called to register actual SCC if RSCC (an ACLProvider) is enabled
func RegisterACLProvider(r ACLProvider) {
	once.Do(func() {
		configtxLock.Lock()
		defer configtxLock.Unlock()
		aclProvider = newACLMgmt(r)
	})
}

//GetACLProvider returns ACLProvider
func GetACLProvider() ACLProvider {
	if aclProvider == nil {
		panic("-----RegisterACLProvider not called -----")
	}
	return aclProvider
}

//NewDefaultACLProvider constructs a new default provider for other systems
//such as RSCC to use
func NewDefaultACLProvider() ACLProvider {
	return newDefaultACLProvider()
}
