/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"bytes"
	"fmt"

	"github.com/hyperledger/fabric/common/chaincode"
	corechaincode "github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/ledger"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	// NamespacesName is the prefix (or namespace) of the DB which will be used to store
	// the information about other namespaces (for things like chaincodes) in the DB.
	// We want a sub-namespaces within lifecycle in case other information needs to be stored here
	// in the future.
	NamespacesName = "namespaces"

	// ChaincodeDefinitionType is the name of the type used to store defined chaincodes
	ChaincodeDefinitionType = "ChaincodeDefinition"

	// FriendlyChaincodeDefinitionType is the name exposed to the outside world for the chaincode namespace
	FriendlyChaincodeDefinitionType = "Chaincode"
)

// Public/World DB layout looks like the following:
// namespaces/metadata/<namespace> -> namespace metadata, including namespace type
// namespaces/fields/<namespace>/Sequence -> sequence for this namespace
// namespaces/fields/<namespace>/<field> -> field of namespace type
//
// So, for instance, a db might look like:
//
// namespaces/metadata/mycc:                   "ChaincodeDefinition"
// namespaces/fields/mycc/Sequence             1 (The current sequence)
// namespaces/fields/mycc/Version:             "1.3"
// namespaces/fields/mycc/Hash:                []byte("some-hash")
// namespaces/fields/mycc/EndorsementPlugin:   "builtin"
// namespaces/fields/mycc/ValidationPlugin:    "builtin"
// namespaces/fields/mycc/ValidationParameter: []byte("some-marshaled-signature-policy")
//
// Private/Org Scope Implcit Collection layout looks like the following
// namespaces/metadata/<namespace>#<sequence_number> -> namespace metadata, including type
// namespaces/fields/<namespace>#<sequence_number>/<field>  -> field of namespace type
//
// namespaces/metadata/mycc#0:                   "ChaincodeParameters"
// namespaces/fields/mycc#0/Version:             "1.2"
// namespaces/fields/mycc#0/Hash:                []byte("some-hash-for-v1.2")
// namespaces/fields/mycc#0/EndorsementPlugin:   "builtin"
// namespaces/fields/mycc#0/ValidationPlugin:    "builtin"
// namespaces/fields/mycc#0/ValidationParameter: []byte("some-marshaled-signature-policy")
// namespaces/metadata/mycc#1:                   "ChaincodeParameters"
// namespaces/fields/mycc#1/Version:             "1.3"
// namespaces/fields/mycc#1/Hash:                []byte("some-hash-for-v1.3")
// namespaces/fields/mycc#1/EndorsementPlugin:   "builtin"
// namespaces/fields/mycc#1/ValidationPlugin:    "builtin"
// namespaces/fields/mycc#1/ValidationParameter: []byte("some-marshaled-signature-policy")

// ChaincodeParameters are the parts of the chaincode definition which are serialized
// as values in the statedb.
// WARNING: This structure is serialized/deserialized from the DB, re-ordering or adding fields
// will cause opaque checks to fail.
type ChaincodeParameters struct {
	Version             string
	Hash                []byte
	EndorsementPlugin   string
	ValidationPlugin    string
	ValidationParameter []byte
	Collections         *cb.CollectionConfigPackage
}

// ChaincodeDefinition contains the chaincode parameters, as well as the sequence number of the definition.
// Note, it does not embed ChaincodeParameters so as not to complicate the serialization.
// WARNING: This structure is serialized/deserialized from the DB, re-ordering or adding fields
// will cause opaque checks to fail.
type ChaincodeDefinition struct {
	Sequence            int64
	Version             string
	Hash                []byte
	EndorsementPlugin   string
	ValidationPlugin    string
	ValidationParameter []byte
	Collections         *cb.CollectionConfigPackage
}

// Parameters returns the non-sequence info of the chaincode definition
func (cd *ChaincodeDefinition) Parameters() *ChaincodeParameters {
	return &ChaincodeParameters{
		Version:             cd.Version,
		Hash:                cd.Hash,
		EndorsementPlugin:   cd.EndorsementPlugin,
		ValidationPlugin:    cd.ValidationPlugin,
		ValidationParameter: cd.ValidationParameter,
		Collections:         cd.Collections,
	}
}

// ChaincodeStore provides a way to persist chaincodes
type ChaincodeStore interface {
	Save(name, version string, ccInstallPkg []byte) (hash []byte, err error)
	RetrieveHash(name, version string) (hash []byte, err error)
	ListInstalledChaincodes() ([]chaincode.InstalledChaincode, error)
	Load(hash []byte) (ccInstallPkg []byte, metadata []*persistence.ChaincodeMetadata, err error)
}

type PackageParser interface {
	Parse(data []byte) (*persistence.ChaincodePackage, error)
}

//go:generate counterfeiter -o mock/legacy_lifecycle.go --fake-name LegacyLifecycle . LegacyLifecycle
type LegacyLifecycle interface {
	corechaincode.Lifecycle
}

//go:generate counterfeiter -o mock/legacy_ccinfo.go --fake-name LegacyDeployedCCInfoProvider . LegacyDeployedCCInfoProvider
type LegacyDeployedCCInfoProvider interface {
	ledger.DeployedChaincodeInfoProvider
}

// Lifecycle implements the lifecycle operations which are invoked
// by the SCC as well as internally
type Lifecycle struct {
	ChannelConfigSource          ChannelConfigSource
	ChaincodeStore               ChaincodeStore
	PackageParser                PackageParser
	Serializer                   *Serializer
	LegacyImpl                   LegacyLifecycle
	LegacyDeployedCCInfoProvider LegacyDeployedCCInfoProvider
}

// CommitChaincodeDefinition takes a chaincode definition, checks that its sequence number is the next allowable sequence number,
// checks which organizations agree with the definition, and applies the definition to the public world state.
// It is the responsibility of the caller to check the agreement to determine if the result is valid (typically
// this means checking that the peer's own org is in agreement.)
func (l *Lifecycle) CommitChaincodeDefinition(name string, cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) ([]bool, error) {
	currentSequence, err := l.Serializer.DeserializeFieldAsInt64(NamespacesName, name, "Sequence", publicState)
	if err != nil {
		return nil, errors.WithMessage(err, "could not get current sequence")
	}

	if cd.Sequence != currentSequence+1 {
		return nil, errors.Errorf("requested sequence is %d, but new definition must be sequence %d", cd.Sequence, currentSequence+1)
	}

	agreement := make([]bool, len(orgStates))
	privateName := fmt.Sprintf("%s#%d", name, cd.Sequence)
	for i, orgState := range orgStates {
		match, err := l.Serializer.IsSerialized(NamespacesName, privateName, cd.Parameters(), orgState)
		agreement[i] = (err == nil && match)
	}

	if err = l.Serializer.Serialize(NamespacesName, name, cd, publicState); err != nil {
		return nil, errors.WithMessage(err, "could not serialize chaincode definition")
	}

	return agreement, nil
}

// ApproveChaincodeDefinitionForOrg adds a chaincode definition entry into the passed in Org state.  The definition must be
// for either the currently defined sequence number or the next sequence number.  If the definition is
// for the current sequence number, then it must match exactly the current definition or it will be rejected.
func (l *Lifecycle) ApproveChaincodeDefinitionForOrg(name string, cd *ChaincodeDefinition, publicState ReadableState, orgState ReadWritableState) error {
	// Get the current sequence from the public state
	currentSequence, err := l.Serializer.DeserializeFieldAsInt64(NamespacesName, name, "Sequence", publicState)
	if err != nil {
		return errors.WithMessage(err, "could not get current sequence")
	}

	requestedSequence := cd.Sequence

	if currentSequence == requestedSequence && requestedSequence == 0 {
		return errors.Errorf("requested sequence is 0, but first definable sequence number is 1")
	}

	if requestedSequence < currentSequence {
		return errors.Errorf("currently defined sequence %d is larger than requested sequence %d", currentSequence, requestedSequence)
	}

	if requestedSequence > currentSequence+1 {
		return errors.Errorf("requested sequence %d is larger than the next available sequence number %d", requestedSequence, currentSequence+1)
	}

	if requestedSequence == currentSequence {
		definedChaincode := &ChaincodeDefinition{}
		if err := l.Serializer.Deserialize(NamespacesName, name, definedChaincode, publicState); err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not deserialize namespace %s as chaincode", name))
		}

		switch {
		case definedChaincode.Version != cd.Version:
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but Version '%s' != '%s'", currentSequence, name, definedChaincode.Version, cd.Version)
		case definedChaincode.EndorsementPlugin != cd.EndorsementPlugin:
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but EndorsementPlugin '%s' != '%s'", currentSequence, name, definedChaincode.EndorsementPlugin, cd.EndorsementPlugin)
		case definedChaincode.ValidationPlugin != cd.ValidationPlugin:
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but ValidationPlugin '%s' != '%s'", currentSequence, name, definedChaincode.ValidationPlugin, cd.ValidationPlugin)
		case !bytes.Equal(definedChaincode.ValidationParameter, cd.ValidationParameter):
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but ValidationParameter '%x' != '%x'", currentSequence, name, definedChaincode.ValidationParameter, cd.ValidationParameter)
		case !bytes.Equal(definedChaincode.Hash, cd.Hash):
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but Hash '%x' != '%x'", currentSequence, name, definedChaincode.Hash, cd.Hash)
		case !proto.Equal(definedChaincode.Collections, cd.Collections):
			if proto.Equal(definedChaincode.Collections, &cb.CollectionConfigPackage{}) && cd.Collections == nil {
				break
			}
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but Collections do not match", currentSequence, name)
		default:
		}
	}

	privateName := fmt.Sprintf("%s#%d", name, requestedSequence)
	if err := l.Serializer.Serialize(NamespacesName, privateName, cd.Parameters(), orgState); err != nil {
		return errors.WithMessage(err, "could not serialize chaincode parameters to state")
	}

	return nil
}

// QueryChaincodeDefinition returns the defined chaincode by the given name (if it is defined, and a chaincode)
// or otherwise returns an error.
func (l *Lifecycle) QueryChaincodeDefinition(name string, publicState ReadableState) (*ChaincodeDefinition, error) {
	definedChaincode := &ChaincodeDefinition{}
	if err := l.Serializer.Deserialize(NamespacesName, name, definedChaincode, publicState); err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize namespace %s as chaincode", name))
	}

	return definedChaincode, nil
}

// InstallChaincode installs a given chaincode to the peer's chaincode store.
// It returns the hash to reference the chaincode by or an error on failure.
func (l *Lifecycle) InstallChaincode(name, version string, chaincodeInstallPackage []byte) ([]byte, error) {
	// Let's validate that the chaincodeInstallPackage is at least well formed before writing it
	_, err := l.PackageParser.Parse(chaincodeInstallPackage)
	if err != nil {
		return nil, errors.WithMessage(err, "could not parse as a chaincode install package")
	}

	hash, err := l.ChaincodeStore.Save(name, version, chaincodeInstallPackage)
	if err != nil {
		return nil, errors.WithMessage(err, "could not save cc install package")
	}

	return hash, nil
}

// QueryNamespaceDefinitions lists the publicly defined namespaces in a channel.  Today it should only ever
// find Datatype encodings of 'ChaincodeDefinition'.  In the future as we support encodings like 'TokenManagementSystem'
// or similar, additional statements will be added to the switch.
func (l *Lifecycle) QueryNamespaceDefinitions(publicState RangeableState) (map[string]string, error) {
	metadatas, err := l.Serializer.DeserializeAllMetadata(NamespacesName, publicState)
	if err != nil {
		return nil, errors.WithMessage(err, "could not query namespace metadata")
	}

	result := map[string]string{}
	for key, value := range metadatas {
		switch value.Datatype {
		case ChaincodeDefinitionType:
			result[key] = FriendlyChaincodeDefinitionType
		default:
			// This should never execute, but seems preferable to returning an error
			result[key] = value.Datatype
		}
	}
	return result, nil
}

// QueryInstalledChaincode returns the hash of an installed chaincode of a given name and version.
func (l *Lifecycle) QueryInstalledChaincode(name, version string) ([]byte, error) {
	hash, err := l.ChaincodeStore.RetrieveHash(name, version)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not retrieve hash for chaincode '%s:%s'", name, version))
	}

	return hash, nil
}

// QueryInstalledChaincodes returns a list of installed chaincodes
func (l *Lifecycle) QueryInstalledChaincodes() ([]chaincode.InstalledChaincode, error) {
	return l.ChaincodeStore.ListInstalledChaincodes()
}
