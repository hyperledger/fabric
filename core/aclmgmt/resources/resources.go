/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package roesources contains resource names used in fabric for ACL checks.
// Note that some of the checks such as Lscc_INSTALL are "peer wide" (current
// access checks in peer are based on local MSP). These are not currently
// covered by resource or default ACLProviders
package resources

const (
	// _lifecycle resources
	Lifecycle_InstallChaincode                   = "_lifecycle/InstallChaincode"
	Lifecycle_QueryInstalledChaincode            = "_lifecycle/QueryInstalledChaincode"
	Lifecycle_GetInstalledChaincodePackage       = "_lifecycle/GetInstalledChaincodePackage"
	Lifecycle_QueryInstalledChaincodes           = "_lifecycle/QueryInstalledChaincodes"
	Lifecycle_ApproveChaincodeDefinitionForMyOrg = "_lifecycle/ApproveChaincodeDefinitionForMyOrg"
	Lifecycle_QueryApprovedChaincodeDefinition   = "_lifecycle/QueryApprovedChaincodeDefinition"
	Lifecycle_CommitChaincodeDefinition          = "_lifecycle/CommitChaincodeDefinition"
	Lifecycle_QueryChaincodeDefinition           = "_lifecycle/QueryChaincodeDefinition"
	Lifecycle_QueryChaincodeDefinitions          = "_lifecycle/QueryChaincodeDefinitions"
	Lifecycle_CheckCommitReadiness               = "_lifecycle/CheckCommitReadiness"

	// snapshot resources
	Snapshot_submitrequest = "snapshot/submitrequest"
	Snapshot_cancelrequest = "snapshot/cancelrequest"
	Snapshot_listpending   = "snapshot/listpending"

	// Lscc resources
	Lscc_Install                   = "lscc/Install"
	Lscc_Deploy                    = "lscc/Deploy"
	Lscc_Upgrade                   = "lscc/Upgrade"
	Lscc_ChaincodeExists           = "lscc/ChaincodeExists"
	Lscc_GetDeploymentSpec         = "lscc/GetDeploymentSpec"
	Lscc_GetChaincodeData          = "lscc/GetChaincodeData"
	Lscc_GetInstantiatedChaincodes = "lscc/GetInstantiatedChaincodes"
	Lscc_GetInstalledChaincodes    = "lscc/GetInstalledChaincodes"
	Lscc_GetCollectionsConfig      = "lscc/GetCollectionsConfig"

	// Qscc resources
	Qscc_GetChainInfo       = "qscc/GetChainInfo"
	Qscc_GetBlockByNumber   = "qscc/GetBlockByNumber"
	Qscc_GetBlockByHash     = "qscc/GetBlockByHash"
	Qscc_GetTransactionByID = "qscc/GetTransactionByID"
	Qscc_GetBlockByTxID     = "qscc/GetBlockByTxID"

	// Cscc resources
	Cscc_JoinChain            = "cscc/JoinChain"
	Cscc_JoinChainBySnapshot  = "cscc/JoinChainBySnapshot"
	Cscc_JoinBySnapshotStatus = "cscc/JoinBySnapshotStatus"
	Cscc_GetConfigBlock       = "cscc/GetConfigBlock"
	Cscc_GetChannelConfig     = "cscc/GetChannelConfig"
	Cscc_GetChannels          = "cscc/GetChannels"

	// Peer resources
	Peer_Propose              = "peer/Propose"
	Peer_ChaincodeToChaincode = "peer/ChaincodeToChaincode"

	// Events
	Event_Block         = "event/Block"
	Event_FilteredBlock = "event/FilteredBlock"

	// Gateway resources
	Gateway_CommitStatus    = "gateway/CommitStatus"
	Gateway_ChaincodeEvents = "gateway/ChaincodeEvents"
)
