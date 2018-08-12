/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
)

// ChaincodeDefinition captures the info about chaincode
type ChaincodeDefinition struct {
	Name              string
	Hash              []byte
	Version           string
	CollectionConfigs *common.CollectionConfigPackage
}

func (cdef *ChaincodeDefinition) String() string {
	return fmt.Sprintf("Name=%s, Version=%s, Hash=%#v", cdef.Name, cdef.Version, cdef.Hash)
}

// ChaincodeLifecycleEventListener interface enables ledger components (mainly, intended for statedb)
// to be able to listen to chaincode lifecycle events. 'dbArtifactsTar' represents db specific artifacts
// (such as index specs) packaged in a tar
type ChaincodeLifecycleEventListener interface {
	// HandleChaincodeDeploy is invoked when chaincode installed + defined becomes true.
	// The expected usage are to creates all the necessary statedb structures (such as indexes) and update
	// service discovery info. This function is invoked immediately before the committing the state changes
	// that contain chaincode definition or when a chaincode install happens
	HandleChaincodeDeploy(chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) error
	// ChaincodeDeployDone is invoked after the chaincode deployment is finished - `succeeded` indicates
	// whether the deploy finished successfully
	ChaincodeDeployDone(succeeded bool)
}

// ChaincodeInfoProvider interface enables event mgr to retrieve chaincode info for a given chaincode
type ChaincodeInfoProvider interface {
	// GetDeployedChaincodeInfo retrieves the details about the deployed chaincode.
	// This function is expected to return nil, if the chaincode with the given name is not deployed
	// or the version or hash of the deployed chaincode does not match with the given version and hash
	GetDeployedChaincodeInfo(chainid string, chaincodeDefinition *ChaincodeDefinition) (*ledger.DeployedChaincodeInfo, error)
	// RetrieveChaincodeArtifacts checks if the given chaincode is installed on the peer and if yes,
	// it extracts the state db specific artifacts from the chaincode package tarball
	RetrieveChaincodeArtifacts(chaincodeDefinition *ChaincodeDefinition) (installed bool, dbArtifactsTar []byte, err error)
}
