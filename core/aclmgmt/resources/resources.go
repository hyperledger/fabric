/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

//fabric resources used for ACL checks. Note that some of the checks
//such as Lscc_INSTALL are "peer wide" (current access checks in peer are
//based on local MSP). These are not currently covered by resource or default
//ACLProviders
const (
	//Lscc resources
	Lscc_Install                   = "lscc/Install"
	Lscc_Deploy                    = "lscc/Deploy"
	Lscc_Upgrade                   = "lscc/Upgrade"
	Lscc_ChaincodeExists           = "lscc/ChaincodeExists"
	Lscc_GetDeploymentSpec         = "lscc/GetDeploymentSpec"
	Lscc_GetChaincodeData          = "lscc/GetChaincodeData"
	Lscc_GetInstantiatedChaincodes = "lscc/GetInstantiatedChaincodes"
	Lscc_GetInstalledChaincodes    = "lscc/GetInstalledChaincodes"
	Lscc_GetCollectionsConfig      = "lscc/GetCollectionsConfig"

	//Qscc resources
	Qscc_GetChainInfo       = "qscc/GetChainInfo"
	Qscc_GetBlockByNumber   = "qscc/GetBlockByNumber"
	Qscc_GetBlockByHash     = "qscc/GetBlockByHash"
	Qscc_GetTransactionByID = "qscc/GetTransactionByID"
	Qscc_GetBlockByTxID     = "qscc/GetBlockByTxID"

	//Cscc resources
	Cscc_JoinChain                = "cscc/JoinChain"
	Cscc_GetConfigBlock           = "cscc/GetConfigBlock"
	Cscc_GetChannels              = "cscc/GetChannels"
	Cscc_GetConfigTree            = "cscc/GetConfigTree"
	Cscc_SimulateConfigTreeUpdate = "cscc/SimulateConfigTreeUpdate"

	//Peer resources
	Peer_Propose              = "peer/Propose"
	Peer_ChaincodeToChaincode = "peer/ChaincodeToChaincode"

	//Events
	Event_Block         = "event/Block"
	Event_FilteredBlock = "event/FilteredBlock"
)
