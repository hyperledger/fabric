/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cceventmgmt

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
)

// ChaincodeDefinition captures the info about chaincode
type ChaincodeDefinition struct {
	Name              string
	Hash              []byte
	Version           string
	CollectionConfigs []byte
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
	// IsChaincodeDeployed returns true if the given chaincode is deployed on the given channel.
	// TODO, the syscc provider should probably be part of the impl struct, but because of the
	// current complexities of our initialization, it is much easier to pass as a parameter.
	IsChaincodeDeployed(chainid string, chaincodeDefinition *ChaincodeDefinition, sccp sysccprovider.SystemChaincodeProvider) (bool, error)
	// RetrieveChaincodeArtifacts checks if the given chaincode is installed on the peer and if yes,
	// it extracts the state db specific artifacts from the chaincode package tarball
	RetrieveChaincodeArtifacts(chaincodeDefinition *ChaincodeDefinition) (installed bool, dbArtifactsTar []byte, err error)
}

type chaincodeInfoProviderImpl struct {
	PlatformRegistry *platforms.Registry
}

// IsChaincodeDeployed implements function in the interface ChaincodeInfoProvider
func (p *chaincodeInfoProviderImpl) IsChaincodeDeployed(chainid string, chaincodeDefinition *ChaincodeDefinition, sccp sysccprovider.SystemChaincodeProvider) (bool, error) {
	return ccprovider.IsChaincodeDeployed(chainid, chaincodeDefinition.Name, chaincodeDefinition.Version, chaincodeDefinition.Hash, sccp)
}

// RetrieveChaincodeArtifacts implements function in the interface ChaincodeInfoProvider
func (p *chaincodeInfoProviderImpl) RetrieveChaincodeArtifacts(chaincodeDefinition *ChaincodeDefinition) (installed bool, dbArtifactsTar []byte, err error) {
	return ccprovider.ExtractStatedbArtifactsForChaincode(chaincodeDefinition.Name, chaincodeDefinition.Version, p.PlatformRegistry)
}
