/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aclmgmt

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
)

var aclLogger = flogging.MustGetLogger("aclmgmt")

//fabric resources used for ACL checks. Note that some of the checks
//such as LSCC_INSTALL are "peer wide" (current access checks in peer are
//based on local MSP). These are not currently covered by RSCC or defaultProvider
const (
	PROPOSE = "PROPOSE"

	//LSCC resources
	LSCC_INSTALL                = "LSCC_INSTALL"
	LSCC_DEPLOY                 = "LSCC_DEPLOY"
	LSCC_UPGRADE                = "LSCC_UPGRADE"
	LSCC_GETCCINFO              = "LSCC_GETCCINFO"
	LSCC_GETDEPSPEC             = "LSCC_GETDEPSPEC"
	LSCC_GETCCDATA              = "LSCC_GETCCDATA"
	LSCC_GETCHAINCODES          = "LSCC_GETCHAINCODES"
	LSCC_GETINSTALLEDCHAINCODES = "LSCC_GETINSTALLEDCHAINCODES"

	//QSCC resources
	QSCC_GetChainInfo       = "QSCC_GetChainInfo"
	QSCC_GetBlockByNumber   = "QSCC_GetBlockByNumber"
	QSCC_GetBlockByHash     = "QSCC_GetBlockByHash"
	QSCC_GetTransactionByID = "QSCC_GetTransactionByID"
	QSCC_GetBlockByTxID     = "QSCC_GetBlockByTxID"

	//CSCC resources
	CSCC_JoinChain      = "CSCC_JoinChain"
	CSCC_GetConfigBlock = "CSCC_GetConfigBlock"
	CSCC_GetChannels    = "CSCC_GetChannels"

	//Chaincode-to-Chaincode call
	CC2CC = "CC2CC"

	//Events
	BLOCKEVENT         = "BLOCKEVENT"
	FILTEREDBLOCKEVENT = "FILTEREDBLOCKEVENT"
)

type ACLProvider interface {
	//CheckACL checks the ACL for the resource for the channel using the
	//idinfo. idinfo is an object such as SignedProposal from which an
	//id can be extracted for testing against a policy
	CheckACL(resName string, channelID string, idinfo interface{}) error
}

var aclProvider ACLProvider

var once sync.Once

//RegisterACLProvider will be called to register actual SCC if RSCC (an ACLProvider) is enabled
func RegisterACLProvider(r ACLProvider) {
	once.Do(func() {
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
