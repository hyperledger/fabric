/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"strings"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger"

	"github.com/pkg/errors"
)

// LegacyDefinition is an implmentor of ccprovider.ChaincodeDefinition.
// It is a different data-type to allow differentiation at cast-time from
// chaincode definitions which require validaiton of instantiation policy.
type LegacyDefinition struct {
	Name                string
	Version             string
	HashField           []byte
	EndorsementPlugin   string
	ValidationPlugin    string
	ValidationParameter []byte
}

// CCName returns the chaincode name
func (ld *LegacyDefinition) CCName() string {
	return ld.Name
}

// Hash returns the hash of the chaincode.
func (ld *LegacyDefinition) Hash() []byte {
	return ld.HashField
}

// CCVersion returns the version of the chaincode.
func (ld *LegacyDefinition) CCVersion() string {
	return ld.Version
}

// Validation returns how to validate transactions for this chaincode.
// The string returned is the name of the validation method (usually 'vscc')
// and the bytes returned are the argument to the validation (in the case of
// 'vscc', this is a marshaled pb.VSCCArgs message).
func (ld *LegacyDefinition) Validation() (string, []byte) {
	return ld.ValidationPlugin, ld.ValidationParameter
}

// Endorsement returns how to endorse proposals for this chaincode.
// The string returns is the name of the endorsement method (usually 'escc').
func (ld *LegacyDefinition) Endorsement() string {
	return ld.EndorsementPlugin
}

// ChaincodeDefinition returns the details for a chaincode by name
func (l *Lifecycle) ChaincodeDefinition(chaincodeName string, qe ledger.SimpleQueryExecutor) (ccprovider.ChaincodeDefinition, error) {
	state := &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	}
	metadata, ok, err := l.Serializer.DeserializeMetadata(NamespacesName, chaincodeName, state)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize metadata for chaincode %s", chaincodeName))
	}

	if !ok {
		return l.LegacyImpl.ChaincodeDefinition(chaincodeName, qe)
	}

	if metadata.Datatype != ChaincodeDefinitionType {
		return nil, errors.Errorf("not a chaincode type: %s", metadata.Datatype)
	}

	definedChaincode := &ChaincodeDefinition{}
	// Note, this is generally overkill, there's no reason to read keys for the whole definition, but that's how
	// the old lifecycle does it, so to avoid contention, we'll reproduce that logic.  This interface should really be broken
	// into retrieving different bits of chaincode data, like the hash, the endorsement plugin, etc. and only called as needed.
	err = l.Serializer.Deserialize(NamespacesName, chaincodeName, definedChaincode, state)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize chaincode definition for chaincode %s", chaincodeName))
	}

	return &LegacyDefinition{
		Name:                chaincodeName,
		Version:             definedChaincode.Version,
		HashField:           definedChaincode.Hash,
		EndorsementPlugin:   definedChaincode.EndorsementPlugin,
		ValidationPlugin:    definedChaincode.ValidationPlugin,
		ValidationParameter: definedChaincode.ValidationParameter,
	}, nil

}

// ChaincodeContainerInfo returns the information necessary to launch a chaincode
func (l *Lifecycle) ChaincodeContainerInfo(chaincodeName string, qe ledger.SimpleQueryExecutor) (*ccprovider.ChaincodeContainerInfo, error) {
	state := &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	}
	metadata, ok, err := l.Serializer.DeserializeMetadata(NamespacesName, chaincodeName, state)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize metadata for chaincode %s", chaincodeName))
	}

	if !ok {
		return l.LegacyImpl.ChaincodeContainerInfo(chaincodeName, qe)
	}

	if metadata.Datatype != ChaincodeDefinitionType {
		return nil, errors.Errorf("not a chaincode type: %s", metadata.Datatype)
	}

	definedChaincode := &ChaincodeDefinition{}
	err = l.Serializer.Deserialize(NamespacesName, chaincodeName, definedChaincode, state)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize chaincode definition for chaincode %s", chaincodeName))
	}

	// XXX Note, everything below is effectively throw-away.  We need to build and maintain
	// a cache of current chaincode container info for our peer based ont he state of our
	// org's implicit collection.  We cannot query it here because it would introduce an
	// unwanted read dependency.  Also note, this unconditionally reads the chaincode bytes
	// every time, which will be quite slow.  There is purposefully no optimization here
	// as it is throwaway code.

	ccPackageBytes, _, _, err := l.ChaincodeStore.Load(definedChaincode.Hash)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not load chaincode from chaincode store for %s:%s (%x)", chaincodeName, definedChaincode.Version, definedChaincode.Hash))
	}

	ccPackage, err := l.PackageParser.Parse(ccPackageBytes)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not parse chaincode package for %s:%s (%x)", chaincodeName, definedChaincode.Version, definedChaincode.Hash))
	}

	return &ccprovider.ChaincodeContainerInfo{
		Name:          chaincodeName,
		Version:       definedChaincode.Version,
		Path:          ccPackage.Metadata.Path,
		Type:          strings.ToUpper(ccPackage.Metadata.Type),
		ContainerType: "DOCKER",
	}, nil
}
