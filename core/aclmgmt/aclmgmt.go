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

//fabric resources used for ACL checks. Note that some of the checks
//such as LSCC_INSTALL are "peer wide" (current access checks in peer are
//based on local MSP). These are not currently covered by RSCC or defaultProvider
const (
	PROPOSE = "PROPOSE"

	//LSCC resources
	LSCC_INSTALL                = "LSCC.INSTALL"
	LSCC_DEPLOY                 = "LSCC.DEPLOY"
	LSCC_UPGRADE                = "LSCC.UPGRADE"
	LSCC_GETCCINFO              = "LSCC.GETCCINFO"
	LSCC_GETDEPSPEC             = "LSCC.GETDEPSPEC"
	LSCC_GETCCDATA              = "LSCC.GETCCDATA"
	LSCC_GETCHAINCODES          = "LSCC.GETCHAINCODES"
	LSCC_GETINSTALLEDCHAINCODES = "LSCC.GETINSTALLEDCHAINCODES"

	//QSCC resources
	QSCC_GetChainInfo       = "QSCC.GetChainInfo"
	QSCC_GetBlockByNumber   = "QSCC.GetBlockByNumber"
	QSCC_GetBlockByHash     = "QSCC.GetBlockByHash"
	QSCC_GetTransactionByID = "QSCC.GetTransactionByID"
	QSCC_GetBlockByTxID     = "QSCC.GetBlockByTxID"

	//CSCC resources
	CSCC_JoinChain      = "CSCC.JoinChain"
	CSCC_GetConfigBlock = "CSCC.GetConfigBlock"
	CSCC_GetChannels    = "CSCC.GetChannels"

	//Chaincode-to-Chaincode call
	CC2CC = "CC2CC"

	//Events
	BLOCKEVENT         = "BLOCKEVENT"
	FILTEREDBLOCKEVENT = "FILTEREDBLOCKEVENT"
)

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
