/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric/core/common/ccprovider"
)

// ChaincodeDefinition captures the info about chaincode
type ChaincodeDefinition struct {
	Name    string
	Hash    []byte
	Version string
}

func (cdef *ChaincodeDefinition) String() string {
	return fmt.Sprintf("Name=%s, Version=%s, Hash=%#v", cdef.Name, cdef.Version, cdef.Hash)
}

// ChaincodeLifecycleEventListener interface enables ledger components (mainly, intended for statedb)
// to be able to listen to chaincode lifecycle events. 'dbArtifactsTar' represents db specific artifacts
// (such as index specs) packaged in a tar
type ChaincodeLifecycleEventListener interface {
	// HandleChaincodeDeploy is expected to creates all the necessary statedb structures (such as indexes)
	HandleChaincodeDeploy(chaincodeDefinition *ChaincodeDefinition, dbArtifactsTar []byte) error
}

// ChaincodeInfoProvider interface enables event mgr to retrieve chaincode info for a given chaincode
type ChaincodeInfoProvider interface {
	// IsChaincodeDeployed returns true if the given chaincode is deployed on the given channel
	IsChaincodeDeployed(chainid string, chaincodeDefinition *ChaincodeDefinition) (bool, error)
	// RetrieveChaincodeArtifacts checks if the given chaincode is installed on the peer and if yes,
	// it extracts the state db specific artifacts from the chaincode package tarball
	RetrieveChaincodeArtifacts(chaincodeDefinition *ChaincodeDefinition) (installed bool, dbArtifactsTar []byte, err error)
}

type chaincodeInfoProviderImpl struct {
}

// IsChaincodeDeployed implements function in the interface ChaincodeInfoProvider
func (p *chaincodeInfoProviderImpl) IsChaincodeDeployed(chainid string, chaincodeDefinition *ChaincodeDefinition) (bool, error) {
	return ccprovider.IsChaincodeDeployed(chainid, chaincodeDefinition.Name, chaincodeDefinition.Version, chaincodeDefinition.Hash)
}

// RetrieveChaincodeArtifacts implements function in the interface ChaincodeInfoProvider
func (p *chaincodeInfoProviderImpl) RetrieveChaincodeArtifacts(chaincodeDefinition *ChaincodeDefinition) (installed bool, dbArtifactsTar []byte, err error) {
	return ccprovider.ExtractStatedbArtifactsForChaincode(chaincodeDefinition.Name, chaincodeDefinition.Version)
}
