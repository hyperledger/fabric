/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"bytes"
	"fmt"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/pkg/errors"
)

const (
	// NamespacesName is the prefix (or namespace) of the DB which will be used to store
	// the information about other namespaces (for things like chaincodes) in the DB.
	// We want a sub-namespaces within lifecycle in case other information needs to be stored here
	// in the future.
	NamespacesName = "namespaces"

	// DefinedChaincodeType is the name of the type used to store defined chaincodes
	DefinedChaincodeType = "DefinedChaincode"

	// FriendlyDefinedChaincodeType is the name exposed to the outside world for the chaincode namespace
	FriendlyDefinedChaincodeType = "Chaincode"
)

// Public/World DB layout looks like the following:
// namespaces/metadata/<namespace> -> namespace metadata, including namespace type
// namespaces/fields/<namespace>/Sequence -> sequence for this namespace
// namespaces/fields/<namespace>/<field> -> field of namespace type
//
// So, for instance, a db might look like:
//
// namespaces/metadata/mycc:                   "DefinedChaincode"
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

// ChaincodeDefinition contains all of the information required to execute
// and validate a chaincode.
type ChaincodeDefinition struct {
	Sequence   int64
	Name       string
	Parameters *ChaincodeParameters
}

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
}

// DefinedChaincode contains the chaincode parameters, as well as the sequence number of the definition.
// Note, it does not embed ChaincodeParameters so as not to complicate the serialization.
// WARNING: This structure is serialized/deserialized from the DB, re-ordering or adding fields
// will cause opaque checks to fail.
type DefinedChaincode struct {
	Sequence            int64
	Version             string
	Hash                []byte
	EndorsementPlugin   string
	ValidationPlugin    string
	ValidationParameter []byte
}

// ChaincodeStore provides a way to persist chaincodes
type ChaincodeStore interface {
	Save(name, version string, ccInstallPkg []byte) (hash []byte, err error)
	RetrieveHash(name, version string) (hash []byte, err error)
	ListInstalledChaincodes() ([]chaincode.InstalledChaincode, error)
}

type PackageParser interface {
	Parse(data []byte) (*persistence.ChaincodePackage, error)
}

// Lifecycle implements the lifecycle operations which are invoked
// by the SCC as well as internally
type Lifecycle struct {
	ChaincodeStore ChaincodeStore
	PackageParser  PackageParser
	Serializer     *Serializer
}

// DefineChaincode takes a chaincode definition, checks that its sequence number is the next allowable sequence number,
// checks which organizations agree with the definition, and applies the definition to the public world state.
// It is the responsibility of the caller to check the agreement to determine if the result is valid (typically
// this means checking that the peer's own org is in agreement.)
func (l *Lifecycle) DefineChaincode(cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) ([]bool, error) {
	currentSequence, err := l.Serializer.DeserializeFieldAsInt64(NamespacesName, cd.Name, "Sequence", publicState)
	if err != nil {
		return nil, errors.WithMessage(err, "could not get current sequence")
	}

	if cd.Sequence != currentSequence+1 {
		return nil, errors.Errorf("requested sequence is %d, but new definition must be sequence %d", cd.Sequence, currentSequence+1)
	}

	agreement := make([]bool, len(orgStates))
	privateName := fmt.Sprintf("%s#%d", cd.Name, cd.Sequence)
	for i, orgState := range orgStates {
		match, err := l.Serializer.IsSerialized(NamespacesName, privateName, cd.Parameters, orgState)
		agreement[i] = (err == nil && match)
	}

	if err = l.Serializer.Serialize(NamespacesName, cd.Name, &DefinedChaincode{
		Sequence:            cd.Sequence,
		Version:             cd.Parameters.Version,
		Hash:                cd.Parameters.Hash,
		EndorsementPlugin:   cd.Parameters.EndorsementPlugin,
		ValidationPlugin:    cd.Parameters.ValidationPlugin,
		ValidationParameter: cd.Parameters.ValidationParameter,
	}, publicState); err != nil {
		return nil, errors.WithMessage(err, "could not serialize chaincode definition")
	}

	return agreement, nil
}

// DefineChaincodeForOrg adds a chaincode definition entry into the passed in Org state.  The definition must be
// for either the currently defined sequence number or the next sequence number.  If the definition is
// for the current sequence number, then it must match exactly the current definition or it will be rejected.
func (l *Lifecycle) DefineChaincodeForOrg(cd *ChaincodeDefinition, publicState ReadableState, orgState ReadWritableState) error {
	// Get the current sequence from the public state
	currentSequence, err := l.Serializer.DeserializeFieldAsInt64(NamespacesName, cd.Name, "Sequence", publicState)
	if err != nil {
		return errors.WithMessage(err, "could not get current sequence")
	}

	requestedSequence := cd.Sequence

	if requestedSequence < currentSequence {
		return errors.Errorf("currently defined sequence %d is larger than requested sequence %d", currentSequence, requestedSequence)
	}

	if requestedSequence > currentSequence+1 {
		return errors.Errorf("requested sequence %d is larger than the next available sequence number %d", requestedSequence, currentSequence+1)
	}

	if requestedSequence == currentSequence {
		definedChaincode := &DefinedChaincode{}
		if err := l.Serializer.Deserialize(NamespacesName, cd.Name, definedChaincode, publicState); err != nil {
			return errors.WithMessage(err, fmt.Sprintf("could not deserialize namespace %s as chaincode", cd.Name))
		}

		switch {
		case definedChaincode.Version != cd.Parameters.Version:
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but Version '%s' != '%s'", currentSequence, cd.Name, definedChaincode.Version, cd.Parameters.Version)
		case definedChaincode.EndorsementPlugin != cd.Parameters.EndorsementPlugin:
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but EndorsementPlugin '%s' != '%s'", currentSequence, cd.Name, definedChaincode.EndorsementPlugin, cd.Parameters.EndorsementPlugin)
		case definedChaincode.ValidationPlugin != cd.Parameters.ValidationPlugin:
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but ValidationPlugin '%s' != '%s'", currentSequence, cd.Name, definedChaincode.ValidationPlugin, cd.Parameters.ValidationPlugin)
		case !bytes.Equal(definedChaincode.ValidationParameter, cd.Parameters.ValidationParameter):
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but ValidationParameter '%x' != '%x'", currentSequence, cd.Name, definedChaincode.ValidationParameter, cd.Parameters.ValidationParameter)
		case !bytes.Equal(definedChaincode.Hash, cd.Parameters.Hash):
			return errors.Errorf("attempted to define the current sequence (%d) for namespace %s, but Hash '%x' != '%x'", currentSequence, cd.Name, definedChaincode.Hash, cd.Parameters.Hash)
		default:
		}
	}

	privateName := fmt.Sprintf("%s#%d", cd.Name, requestedSequence)
	if err := l.Serializer.Serialize(NamespacesName, privateName, cd.Parameters, orgState); err != nil {
		return errors.WithMessage(err, "could not serialize chaincode parameters to state")
	}

	return nil
}

// QueryDefinedChaincode returns the defined chaincode by the given name (if it is defined, and a chaincode)
// or otherwise returns an error.
func (l *Lifecycle) QueryDefinedChaincode(name string, publicState ReadableState) (*DefinedChaincode, error) {
	definedChaincode := &DefinedChaincode{}
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

// QueryDefinedNamespaces lists the publicly defined namespaces in a channel.  Today it should only ever
// find Datatype encodings of 'DefinedChaincode'.  In the future as we support encodings like 'TokenManagementSystem'
// or similar, additional statements will be added to the switch.
func (l *Lifecycle) QueryDefinedNamespaces(publicState RangeableState) (map[string]string, error) {
	metadatas, err := l.Serializer.DeserializeAllMetadata(NamespacesName, publicState)
	if err != nil {
		return nil, errors.WithMessage(err, "could not query namespace metadata")
	}

	result := map[string]string{}
	for key, value := range metadatas {
		switch value.Datatype {
		case DefinedChaincodeType:
			result[key] = FriendlyDefinedChaincodeType
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
