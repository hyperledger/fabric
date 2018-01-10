/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

//fabric resources used for ACL checks. Note that some of the checks
//such as LSCC_INSTALL are "peer wide" (current access checks in peer are
//based on local MSP). These are not currently covered by resource or default
//ACLProviders
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
	CSCC_JoinChain                = "CSCC.JoinChain"
	CSCC_GetConfigBlock           = "CSCC.GetConfigBlock"
	CSCC_GetChannels              = "CSCC.GetChannels"
	CSCC_GetConfigTree            = "CSCC.GetConfigTree"
	CSCC_SimulateConfigTreeUpdate = "CSCC.SimulateConfigTreeUpdate"

	//Chaincode-to-Chaincode call
	CC2CC = "CC2CC"

	//Events
	BLOCKEVENT         = "BLOCKEVENT"
	FILTEREDBLOCKEVENT = "FILTEREDBLOCKEVENT"
)
