/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"bytes"
	"fmt"
	"sync"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("lifecycle")

const (
	// NamespacesName is the prefix (or namespace) of the DB which will be used to store
	// the information about other namespaces (for things like chaincodes) in the DB.
	// We want a sub-namespaces within lifecycle in case other information needs to be stored here
	// in the future.
	NamespacesName = "namespaces"

	// ChaincodeSourcesName is the namespace reserved for storing the information about where
	// to find the chaincode (such as as a package on the local filesystem, or in the future,
	// at some network resource). This namespace is only populated in the org implicit collection.
	ChaincodeSourcesName = "chaincode-sources"

	// ChaincodeLocalPackageType is the name of the type of chaincode-sources which may be serialized
	// into the org's private data collection
	ChaincodeLocalPackageType = "ChaincodeLocalPackage"

	// ChaincodeParametersType is the name of the type used to store the parts of the chaincode definition
	// which are serialized as values in the statedb
	ChaincodeParametersType = "ChaincodeParameters"

	// ChaincodeDefinitionType is the name of the type used to store defined chaincodes
	ChaincodeDefinitionType = "ChaincodeDefinition"

	// FriendlyChaincodeDefinitionType is the name exposed to the outside world for the chaincode namespace
	FriendlyChaincodeDefinitionType = "Chaincode"

	// DefaultEndorsementPolicyRef is the name of the default endorsement policy for this channel
	DefaultEndorsementPolicyRef = "/Channel/Application/Endorsement"
)

var DefaultEndorsementPolicyBytes = protoutil.MarshalOrPanic(&pb.ApplicationPolicy{
	Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
		ChannelConfigPolicyReference: DefaultEndorsementPolicyRef,
	},
})

// Sequences are the underpinning of the definition framework for lifecycle.
// All definitions must have a Sequence field in the public state.  This
// sequence is incremented by exactly 1 with each redefinition of the
// namespace. The private state org approvals also have a Sequence number
// embedded into the key which matches them either to the vote for the commit,
// or registers approval for an already committed definition.
//
// Public/World DB layout looks like the following:
// namespaces/metadata/<namespace> -> namespace metadata, including namespace type
// namespaces/fields/<namespace>/Sequence -> sequence for this namespace
// namespaces/fields/<namespace>/<field> -> field of namespace type
//
// So, for instance, a db might look like:
//
// namespaces/metadata/mycc:                   "ChaincodeDefinition"
// namespaces/fields/mycc/Sequence             1 (The current sequence)
// namespaces/fields/mycc/EndorsementInfo:     {Version: "1.3", EndorsementPlugin: "builtin", InitRequired: true}
// namespaces/fields/mycc/ValidationInfo:      {ValidationPlugin: "builtin", ValidationParameter: <application-policy>}
// namespaces/fields/mycc/Collections          {<collection info>}
//
// Private/Org Scope Implcit Collection layout looks like the following
// namespaces/metadata/<namespace>#<sequence_number> -> namespace metadata, including type
// namespaces/fields/<namespace>#<sequence_number>/<field>  -> field of namespace type
//
// namespaces/metadata/mycc#1:                   "ChaincodeParameters"
// namespaces/fields/mycc#1/EndorsementInfo:     {Version: "1.3", EndorsementPlugin: "builtin", InitRequired: true}
// namespaces/fields/mycc#1/ValidationInfo:      {ValidationPlugin: "builtin", ValidationParameter: <application-policy>}
// namespaces/fields/mycc#1/Collections          {<collection info>}
// namespaces/metadata/mycc#2:                   "ChaincodeParameters"
// namespaces/fields/mycc#2/EndorsementInfo:     {Version: "1.4", EndorsementPlugin: "builtin", InitRequired: true}
// namespaces/fields/mycc#2/ValidationInfo:      {ValidationPlugin: "builtin", ValidationParameter: <application-policy>}
// namespaces/fields/mycc#2/Collections          {<collection info>}
//
// chaincode-sources/metadata/mycc#1              "ChaincodeLocalPackage"
// chaincode-sources/fields/mycc#1/PackageID      "hash1"

// ChaincodeLocalPackage is a type of chaincode-sources which may be serialized
// into the org's private data collection.
// WARNING: This structure is serialized/deserialized from the DB, re-ordering
// or adding fields will cause opaque checks to fail.
type ChaincodeLocalPackage struct {
	PackageID string
}

// ChaincodeParameters are the parts of the chaincode definition which are serialized
// as values in the statedb.  It is expected that any instance will have no nil fields once initialized.
// WARNING: This structure is serialized/deserialized from the DB, re-ordering or adding fields
// will cause opaque checks to fail.
type ChaincodeParameters struct {
	EndorsementInfo *lb.ChaincodeEndorsementInfo
	ValidationInfo  *lb.ChaincodeValidationInfo
	Collections     *pb.CollectionConfigPackage
}

func (cp *ChaincodeParameters) Equal(ocp *ChaincodeParameters) error {
	switch {
	case cp.EndorsementInfo.Version != ocp.EndorsementInfo.Version:
		return errors.Errorf("expected Version '%s' does not match passed Version '%s'", cp.EndorsementInfo.Version, ocp.EndorsementInfo.Version)
	case cp.EndorsementInfo.EndorsementPlugin != ocp.EndorsementInfo.EndorsementPlugin:
		return errors.Errorf("expected EndorsementPlugin '%s' does not match passed EndorsementPlugin '%s'", cp.EndorsementInfo.EndorsementPlugin, ocp.EndorsementInfo.EndorsementPlugin)
	case cp.EndorsementInfo.InitRequired != ocp.EndorsementInfo.InitRequired:
		return errors.Errorf("expected InitRequired '%t' does not match passed InitRequired '%t'", cp.EndorsementInfo.InitRequired, ocp.EndorsementInfo.InitRequired)
	case cp.ValidationInfo.ValidationPlugin != ocp.ValidationInfo.ValidationPlugin:
		return errors.Errorf("expected ValidationPlugin '%s' does not match passed ValidationPlugin '%s'", cp.ValidationInfo.ValidationPlugin, ocp.ValidationInfo.ValidationPlugin)
	case !bytes.Equal(cp.ValidationInfo.ValidationParameter, ocp.ValidationInfo.ValidationParameter):
		return errors.Errorf("expected ValidationParameter '%x' does not match passed ValidationParameter '%x'", cp.ValidationInfo.ValidationParameter, ocp.ValidationInfo.ValidationParameter)
	case !proto.Equal(cp.Collections, ocp.Collections):
		return errors.Errorf("Collections do not match")
	default:
	}
	return nil
}

// ChaincodeDefinition contains the chaincode parameters, as well as the sequence number of the definition.
// Note, it does not embed ChaincodeParameters so as not to complicate the serialization.  It is expected
// that any instance will have no nil fields once initialized.
// WARNING: This structure is serialized/deserialized from the DB, re-ordering or adding fields
// will cause opaque checks to fail.
type ChaincodeDefinition struct {
	Sequence        int64
	EndorsementInfo *lb.ChaincodeEndorsementInfo
	ValidationInfo  *lb.ChaincodeValidationInfo
	Collections     *pb.CollectionConfigPackage
}

type ApprovedChaincodeDefinition struct {
	Sequence        int64
	EndorsementInfo *lb.ChaincodeEndorsementInfo
	ValidationInfo  *lb.ChaincodeValidationInfo
	Collections     *pb.CollectionConfigPackage
	Source          *lb.ChaincodeSource
}

// Parameters returns the non-sequence info of the chaincode definition
func (cd *ChaincodeDefinition) Parameters() *ChaincodeParameters {
	return &ChaincodeParameters{
		EndorsementInfo: cd.EndorsementInfo,
		ValidationInfo:  cd.ValidationInfo,
		Collections:     cd.Collections,
	}
}

func (cd *ChaincodeDefinition) String() string {
	endorsementInfo := "endorsement info: <EMPTY>"
	if cd.EndorsementInfo != nil {
		endorsementInfo = fmt.Sprintf("endorsement info: (version: '%s', plugin: '%s', init required: %t)",
			cd.EndorsementInfo.Version,
			cd.EndorsementInfo.EndorsementPlugin,
			cd.EndorsementInfo.InitRequired,
		)
	}

	validationInfo := "validation info: <EMPTY>"
	if cd.ValidationInfo != nil {
		validationInfo = fmt.Sprintf("validation info: (plugin: '%s', policy: '%x')",
			cd.ValidationInfo.ValidationPlugin,
			cd.ValidationInfo.ValidationParameter,
		)
	}

	return fmt.Sprintf("sequence: %d, %s, %s, collections: (%+v)",
		cd.Sequence,
		endorsementInfo,
		validationInfo,
		cd.Collections,
	)
}

//go:generate counterfeiter -o mock/chaincode_builder.go --fake-name ChaincodeBuilder . ChaincodeBuilder

type ChaincodeBuilder interface {
	Build(ccid string) error
}

// ChaincodeStore provides a way to persist chaincodes
type ChaincodeStore interface {
	Save(label string, ccInstallPkg []byte) (string, error)
	ListInstalledChaincodes() ([]chaincode.InstalledChaincode, error)
	Load(packageID string) (ccInstallPkg []byte, err error)
	Delete(packageID string) error
}

type PackageParser interface {
	Parse(data []byte) (*persistence.ChaincodePackage, error)
}

//go:generate counterfeiter -o mock/install_listener.go --fake-name InstallListener . InstallListener
type InstallListener interface {
	HandleChaincodeInstalled(md *persistence.ChaincodePackageMetadata, packageID string)
}

//go:generate counterfeiter -o mock/installed_chaincodes_lister.go --fake-name InstalledChaincodesLister . InstalledChaincodesLister
type InstalledChaincodesLister interface {
	ListInstalledChaincodes() []*chaincode.InstalledChaincode
	GetInstalledChaincode(packageID string) (*chaincode.InstalledChaincode, error)
}

// Resources stores the common functions needed by all components of the lifecycle
// by the SCC as well as internally.  It also has some utility methods attached to it
// for querying the lifecycle definitions.
type Resources struct {
	ChannelConfigSource ChannelConfigSource
	ChaincodeStore      ChaincodeStore
	PackageParser       PackageParser
	Serializer          *Serializer
}

// ChaincodeDefinitionIfDefined returns whether the chaincode name is defined in the new lifecycle, a shim around
// the SimpleQueryExecutor to work with the serializer, or an error.  If the namespace is defined, but it is
// not a chaincode, this is considered an error.
func (r *Resources) ChaincodeDefinitionIfDefined(chaincodeName string, state ReadableState) (bool, *ChaincodeDefinition, error) {
	if chaincodeName == LifecycleNamespace {
		return true, &ChaincodeDefinition{
			EndorsementInfo: &lb.ChaincodeEndorsementInfo{
				InitRequired: false,
			},
			ValidationInfo: &lb.ChaincodeValidationInfo{},
		}, nil
	}

	metadata, ok, err := r.Serializer.DeserializeMetadata(NamespacesName, chaincodeName, state)
	if err != nil {
		return false, nil, errors.WithMessagef(err, "could not deserialize metadata for chaincode %s", chaincodeName)
	}

	if !ok {
		return false, nil, nil
	}

	if metadata.Datatype != ChaincodeDefinitionType {
		return false, nil, errors.Errorf("not a chaincode type: %s", metadata.Datatype)
	}

	definedChaincode := &ChaincodeDefinition{}
	err = r.Serializer.Deserialize(NamespacesName, chaincodeName, metadata, definedChaincode, state)
	if err != nil {
		return false, nil, errors.WithMessagef(err, "could not deserialize chaincode definition for chaincode %s", chaincodeName)
	}

	return true, definedChaincode, nil
}

func (r *Resources) LifecycleEndorsementPolicyAsBytes(channelID string) ([]byte, error) {
	channelConfig := r.ChannelConfigSource.GetStableChannelConfig(channelID)
	if channelConfig == nil {
		return nil, errors.Errorf("could not get channel config for channel '%s'", channelID)
	}

	if _, ok := channelConfig.PolicyManager().GetPolicy(LifecycleEndorsementPolicyRef); ok {
		return LifecycleDefaultEndorsementPolicyBytes, nil
	}

	// This was a channel which was upgraded or did not define a lifecycle endorsement policy, use a default
	// of "a majority of orgs must have a member sign".
	ac, ok := channelConfig.ApplicationConfig()
	if !ok {
		return nil, errors.Errorf("could not get application config for channel '%s'", channelID)
	}
	orgs := ac.Organizations()
	mspids := make([]string, 0, len(orgs))
	for _, org := range orgs {
		mspids = append(mspids, org.MSPID())
	}

	return protoutil.MarshalOrPanic(&cb.ApplicationPolicy{
		Type: &cb.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: policydsl.SignedByNOutOfGivenRole(int32(len(mspids)/2+1), msp.MSPRole_MEMBER, mspids),
		},
	}), nil
}

// ExternalFunctions is intended primarily to support the SCC functions.
// In general, its methods signatures produce writes (which must be commmitted
// as part of an endorsement flow), or return human readable errors (for
// instance indicating a chaincode is not found) rather than sentinels.
// Instead, use the utility functions attached to the lifecycle Resources
// when needed.
type ExternalFunctions struct {
	Resources                 *Resources
	InstallListener           InstallListener
	InstalledChaincodesLister InstalledChaincodesLister
	ChaincodeBuilder          ChaincodeBuilder
	BuildRegistry             *container.BuildRegistry
	mutex                     sync.Mutex
	BuildLocks                map[string]*sync.Mutex
	concurrentInstalls        uint32
}

// CheckCommitReadiness takes a chaincode definition, checks that
// its sequence number is the next allowable sequence number and checks which
// organizations have approved the definition.
func (ef *ExternalFunctions) CheckCommitReadiness(chname, ccname string, cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) (map[string]bool, error) {
	currentSequence, err := ef.Resources.Serializer.DeserializeFieldAsInt64(NamespacesName, ccname, "Sequence", publicState)
	if err != nil {
		return nil, errors.WithMessage(err, "could not get current sequence")
	}

	if cd.Sequence != currentSequence+1 {
		return nil, errors.Errorf("requested sequence is %d, but new definition must be sequence %d", cd.Sequence, currentSequence+1)
	}

	if err := ef.SetChaincodeDefinitionDefaults(chname, cd); err != nil {
		return nil, errors.WithMessagef(err, "could not set defaults for chaincode definition in channel %s", chname)
	}

	var approvals map[string]bool
	if approvals, err = ef.QueryOrgApprovals(ccname, cd, orgStates); err != nil {
		return nil, err
	}

	logger.Infof("Successfully checked commit readiness of chaincode name '%s' on channel '%s' with definition {%s}", ccname, chname, cd)

	return approvals, nil
}

// CommitChaincodeDefinition takes a chaincode definition, checks that its
// sequence number is the next allowable sequence number, checks which
// organizations have approved the definition, and applies the definition to
// the public world state. It is the responsibility of the caller to check
// the approvals to determine if the result is valid (typically, this means
// checking that the peer's own org has approved the definition).
func (ef *ExternalFunctions) CommitChaincodeDefinition(chname, ccname string, cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) (map[string]bool, error) {
	approvals, err := ef.CheckCommitReadiness(chname, ccname, cd, publicState, orgStates)
	if err != nil {
		return nil, err
	}

	if err = ef.Resources.Serializer.Serialize(NamespacesName, ccname, cd, publicState); err != nil {
		return nil, errors.WithMessage(err, "could not serialize chaincode definition")
	}

	return approvals, nil
}

// DefaultEndorsementPolicyAsBytes returns a marshalled version
// of the default chaincode endorsement policy in the supplied channel
func (ef *ExternalFunctions) DefaultEndorsementPolicyAsBytes(channelID string) ([]byte, error) {
	channelConfig := ef.Resources.ChannelConfigSource.GetStableChannelConfig(channelID)
	if channelConfig == nil {
		return nil, errors.Errorf("could not get channel config for channel '%s'", channelID)
	}

	// see if the channel defines a default
	if _, ok := channelConfig.PolicyManager().GetPolicy(DefaultEndorsementPolicyRef); ok {
		return DefaultEndorsementPolicyBytes, nil
	}

	return nil, errors.Errorf(
		"policy '%s' must be defined for channel '%s' before chaincode operations can be attempted",
		DefaultEndorsementPolicyRef,
		channelID,
	)
}

// SetChaincodeDefinitionDefaults fills any empty fields in the
// supplied ChaincodeDefinition with the supplied channel's defaults
func (ef *ExternalFunctions) SetChaincodeDefinitionDefaults(chname string, cd *ChaincodeDefinition) error {
	if cd.EndorsementInfo.EndorsementPlugin == "" {
		// TODO:
		// 1) rename to "default" or "builtin"
		// 2) retrieve from channel config
		cd.EndorsementInfo.EndorsementPlugin = "escc"
	}

	if cd.ValidationInfo.ValidationPlugin == "" {
		// TODO:
		// 1) rename to "default" or "builtin"
		// 2) retrieve from channel config
		cd.ValidationInfo.ValidationPlugin = "vscc"
	}

	if len(cd.ValidationInfo.ValidationParameter) == 0 {
		policyBytes, err := ef.DefaultEndorsementPolicyAsBytes(chname)
		if err != nil {
			return err
		}

		cd.ValidationInfo.ValidationParameter = policyBytes
	}

	return nil
}

// ApproveChaincodeDefinitionForOrg adds a chaincode definition entry into the passed in Org state.  The definition must be
// for either the currently defined sequence number or the next sequence number.  If the definition is
// for the current sequence number, then it must match exactly the current definition or it will be rejected.
func (ef *ExternalFunctions) ApproveChaincodeDefinitionForOrg(chname, ccname string, cd *ChaincodeDefinition, packageID string, publicState ReadableState, orgState ReadWritableState) error {
	// Get the current sequence from the public state
	currentSequence, err := ef.Resources.Serializer.DeserializeFieldAsInt64(NamespacesName, ccname, "Sequence", publicState)
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

	if err := ef.SetChaincodeDefinitionDefaults(chname, cd); err != nil {
		return errors.WithMessagef(err, "could not set defaults for chaincode definition in channel %s", chname)
	}

	privateName := fmt.Sprintf("%s#%d", ccname, requestedSequence)

	if requestedSequence == currentSequence {
		metadata, ok, err := ef.Resources.Serializer.DeserializeMetadata(NamespacesName, ccname, publicState)
		if err != nil {
			return errors.WithMessage(err, "could not fetch metadata for current definition")
		}
		if !ok {
			return errors.Errorf("missing metadata for currently committed sequence number (%d)", currentSequence)
		}

		definedChaincode := &ChaincodeDefinition{}
		if err := ef.Resources.Serializer.Deserialize(NamespacesName, ccname, metadata, definedChaincode, publicState); err != nil {
			return errors.WithMessagef(err, "could not deserialize namespace %s as chaincode", ccname)
		}

		if err := definedChaincode.Parameters().Equal(cd.Parameters()); err != nil {
			return errors.WithMessagef(err, "attempted to redefine the current committed sequence (%d) for namespace %s with different parameters", currentSequence, ccname)
		}
		// it might be the case that some organization just installed
		// the chaincode and now would like to approve, therefore we
		// need to check to distiguish the case. hence need to read from
		// the orgState metadata and see whenever this is the case
		redefine, err := ef.isAttemptToRedefine(privateName, packageID, requestedSequence, cd, orgState)
		if err != nil {
			return err
		}
		if redefine {
			return errors.Errorf("attempted to redefine the current committed sequence (%d) for namespace %s", currentSequence, ccname)
		}
	}

	// if requested sequence is not committed, and attempt is made to update its content,
	// we need to check whether new definition actually contains updated content, to avoid
	// empty write set.
	if requestedSequence == currentSequence+1 {
		redefine, err := ef.isAttemptToRedefine(privateName, packageID, requestedSequence, cd, orgState)
		if err != nil {
			return err
		}
		if redefine {
			return errors.Errorf("attempted to redefine uncommitted sequence (%d) for namespace %s with unchanged content", requestedSequence, ccname)
		}
	}

	if err := ef.Resources.Serializer.Serialize(NamespacesName, privateName, cd.Parameters(), orgState); err != nil {
		return errors.WithMessage(err, "could not serialize chaincode parameters to state")
	}

	// set the package id - whether empty or not. Setting
	// an empty package ID means that the chaincode won't
	// be invocable. The package might be set empty after
	// the definition commits as a way of instructing the
	// peers of an org no longer to endorse invocations
	// for this chaincode
	if err := ef.Resources.Serializer.Serialize(ChaincodeSourcesName, privateName, &ChaincodeLocalPackage{
		PackageID: packageID,
	}, orgState); err != nil {
		return errors.WithMessage(err, "could not serialize chaincode package info to state")
	}

	logger.Infof("Successfully endorsed chaincode approval with name '%s', package ID '%s', on channel '%s' with definition {%s}", ccname, packageID, chname, cd)

	return nil
}

func (ef *ExternalFunctions) isAttemptToRedefine(privateName, packageID string, requestedSequence int64, cd *ChaincodeDefinition, orgState ReadWritableState) (bool, error) {
	metadata, ok, err := ef.Resources.Serializer.DeserializeMetadata(NamespacesName, privateName, orgState)
	if err != nil {
		return false, errors.WithMessagef(err, "could not fetch chaincode definition, chaincode %s, requested at sequence %d", privateName, requestedSequence)
	}

	if ok {
		logger.Debugf("Attempting to redefine chaincode definition, chaincode %s at sequence %d", privateName, requestedSequence)

		chaincodeParameters := &ChaincodeParameters{}
		if err := ef.Resources.Serializer.Deserialize(NamespacesName, privateName, metadata, chaincodeParameters, orgState); err != nil {
			return false, errors.WithMessagef(err, "could not deserialize namespace %s as chaincode", privateName)
		}

		if err := chaincodeParameters.Equal(cd.Parameters()); err == nil {
			// also check package ID updates
			metadata, ok, err := ef.Resources.Serializer.DeserializeMetadata(ChaincodeSourcesName, privateName, orgState)
			if err != nil {
				return false, errors.WithMessagef(err, "could not deserialize chaincode-source metadata for %s", privateName)
			}
			if ok {
				ccLocalPackage := &ChaincodeLocalPackage{}
				if err := ef.Resources.Serializer.Deserialize(ChaincodeSourcesName, privateName, metadata, ccLocalPackage, orgState); err != nil {
					return false, errors.WithMessagef(err, "could not deserialize chaincode package for %s", privateName)
				}

				if ccLocalPackage.PackageID == packageID {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// QueryApprovedChaincodeDefinition returns the approved chaincode definition in Org state by using the given parameters.
// If the parameter of sequence is not provided, this function returns the latest approved chaincode definition
// (latest: new one of the currently defined sequence number and the next sequence number).
func (ef *ExternalFunctions) QueryApprovedChaincodeDefinition(chname, ccname string, sequence int64, publicState ReadableState, orgState ReadableState) (*ApprovedChaincodeDefinition, error) {
	requestedSequence := sequence

	// If requested sequence is not provided,
	// set the latest sequence number (either the currently defined sequence number or the next sequence number)
	if requestedSequence == 0 {
		currentSequence, err := ef.Resources.Serializer.DeserializeFieldAsInt64(NamespacesName, ccname, "Sequence", publicState)
		if err != nil {
			return nil, errors.WithMessage(err, "could not get current sequence")
		}
		requestedSequence = currentSequence

		nextSequence := currentSequence + 1
		privateName := fmt.Sprintf("%s#%d", ccname, nextSequence)
		_, ok, err := ef.Resources.Serializer.DeserializeMetadata(NamespacesName, privateName, orgState)
		if err != nil {
			return nil, errors.WithMessagef(err, "could not deserialize namespace metadata for next sequence %d", nextSequence)
		}
		if ok {
			requestedSequence = nextSequence
		}
	}

	logger.Infof("Attempting to fetch approved definition (name: '%s', sequence: '%d') on channel '%s'", ccname, requestedSequence, chname)
	privateName := fmt.Sprintf("%s#%d", ccname, requestedSequence)
	metadata, ok, err := ef.Resources.Serializer.DeserializeMetadata(NamespacesName, privateName, orgState)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not deserialize namespace metadata for %s", privateName)
	}

	if !ok {
		return nil, errors.Errorf("could not fetch approved chaincode definition (name: '%s', sequence: '%d') on channel '%s'", ccname, sequence, chname)
	}

	if metadata.Datatype != ChaincodeParametersType {
		return nil, errors.Errorf("not a chaincode parameters type: %s", metadata.Datatype)
	}

	// Get chaincode parameters for the request sequence
	ccParameters := &ChaincodeParameters{}
	if err := ef.Resources.Serializer.Deserialize(NamespacesName, privateName, metadata, ccParameters, orgState); err != nil {
		return nil, errors.WithMessagef(err, "could not deserialize chaincode parameters for %s", privateName)
	}

	// Get package ID for the requested sequence
	metadata, ok, err = ef.Resources.Serializer.DeserializeMetadata(ChaincodeSourcesName, privateName, orgState)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not deserialize chaincode-source metadata for %s", privateName)
	}
	if !ok {
		return nil, errors.Errorf("could not fetch approved chaincode definition (name: '%s', sequence: '%d') on channel '%s'", ccname, sequence, chname)
	}
	if metadata.Datatype != ChaincodeLocalPackageType {
		return nil, errors.Errorf("not a chaincode local package type: %s", metadata.Datatype)
	}

	ccLocalPackage := &ChaincodeLocalPackage{}
	if err := ef.Resources.Serializer.Deserialize(ChaincodeSourcesName, privateName, metadata, ccLocalPackage, orgState); err != nil {
		return nil, errors.WithMessagef(err, "could not deserialize chaincode package for %s", privateName)
	}

	// Convert to lb.ChaincodeSource
	// An empty package ID means that the chaincode won't be invocable
	var ccsrc *lb.ChaincodeSource
	if ccLocalPackage.PackageID != "" {
		ccsrc = &lb.ChaincodeSource{
			Type: &lb.ChaincodeSource_LocalPackage{
				LocalPackage: &lb.ChaincodeSource_Local{
					PackageId: ccLocalPackage.PackageID,
				},
			},
		}
	} else {
		ccsrc = &lb.ChaincodeSource{
			Type: &lb.ChaincodeSource_Unavailable_{
				Unavailable: &lb.ChaincodeSource_Unavailable{},
			},
		}
	}

	return &ApprovedChaincodeDefinition{
		Sequence:        requestedSequence,
		EndorsementInfo: ccParameters.EndorsementInfo,
		ValidationInfo:  ccParameters.ValidationInfo,
		Collections:     ccParameters.Collections,
		Source:          ccsrc,
	}, nil
}

// ErrNamespaceNotDefined is the error returned when a namespace
// is not defined. This indicates that the chaincode definition
// has not been committed.
type ErrNamespaceNotDefined struct {
	Namespace string
}

func (e ErrNamespaceNotDefined) Error() string {
	return fmt.Sprintf("namespace %s is not defined", e.Namespace)
}

// QueryChaincodeDefinition returns the defined chaincode by the given name (if it is committed, and a chaincode)
// or otherwise returns an error.
func (ef *ExternalFunctions) QueryChaincodeDefinition(name string, publicState ReadableState) (*ChaincodeDefinition, error) {
	metadata, ok, err := ef.Resources.Serializer.DeserializeMetadata(NamespacesName, name, publicState)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not fetch metadata for namespace %s", name)
	}
	if !ok {
		return nil, ErrNamespaceNotDefined{Namespace: name}
	}

	definedChaincode := &ChaincodeDefinition{}
	if err := ef.Resources.Serializer.Deserialize(NamespacesName, name, metadata, definedChaincode, publicState); err != nil {
		return nil, errors.WithMessagef(err, "could not deserialize namespace %s as chaincode", name)
	}

	logger.Infof("Successfully queried chaincode name '%s' with definition {%s},", name, definedChaincode)

	return definedChaincode, nil
}

// QueryOrgApprovals returns a map containing the orgs whose orgStates were
// provided and whether or not they have approved a chaincode definition with
// the specified parameters.
func (ef *ExternalFunctions) QueryOrgApprovals(name string, cd *ChaincodeDefinition, orgStates []OpaqueState) (map[string]bool, error) {
	approvals := map[string]bool{}
	privateName := fmt.Sprintf("%s#%d", name, cd.Sequence)
	for _, orgState := range orgStates {
		match, err := ef.Resources.Serializer.IsSerialized(NamespacesName, privateName, cd.Parameters(), orgState)
		if err != nil {
			return nil, errors.WithMessagef(err, "serialization check failed for key %s", privateName)
		}

		_, org := implicitcollection.MspIDIfImplicitCollection(orgState.CollectionName())
		approvals[org] = match
	}

	return approvals, nil
}

// InstallChaincode installs a given chaincode to the peer's chaincode store.
// It returns the hash to reference the chaincode by or an error on failure.
func (ef *ExternalFunctions) InstallChaincode(chaincodeInstallPackage []byte) (*chaincode.InstalledChaincode, error) {
	// Let's validate that the chaincodeInstallPackage is at least well formed before writing it
	pkg, err := ef.Resources.PackageParser.Parse(chaincodeInstallPackage)
	if err != nil {
		return nil, errors.WithMessage(err, "could not parse as a chaincode install package")
	}

	if pkg.Metadata == nil {
		return nil, errors.New("empty metadata for supplied chaincode")
	}

	packageID, err := ef.Resources.ChaincodeStore.Save(pkg.Metadata.Label, chaincodeInstallPackage)
	if err != nil {
		return nil, errors.WithMessage(err, "could not save cc install package")
	}

	buildLock, cleanupBuildLocks := ef.getBuildLock(packageID)
	defer cleanupBuildLocks()

	buildLock.Lock()
	defer buildLock.Unlock()

	buildStatus, ok := ef.BuildRegistry.BuildStatus(packageID)
	if ok {
		// another invocation of lifecycle has concurrently
		// installed a chaincode with this package id
		<-buildStatus.Done()
		if buildStatus.Err() == nil {
			return nil, errors.Errorf("chaincode already successfully installed (package ID '%s')", packageID)
		}
		buildStatus = ef.BuildRegistry.ResetBuildStatus(packageID)
	}
	err = ef.ChaincodeBuilder.Build(packageID)
	buildStatus.Notify(err)
	<-buildStatus.Done()
	if err := buildStatus.Err(); err != nil {
		ef.Resources.ChaincodeStore.Delete(packageID)
		return nil, errors.WithMessage(err, "could not build chaincode")
	}

	if ef.InstallListener != nil {
		ef.InstallListener.HandleChaincodeInstalled(pkg.Metadata, packageID)
	}

	logger.Infof("Successfully installed chaincode with package ID '%s'", packageID)

	return &chaincode.InstalledChaincode{
		PackageID: packageID,
		Label:     pkg.Metadata.Label,
	}, nil
}

func (ef *ExternalFunctions) getBuildLock(packageID string) (*sync.Mutex, func()) {
	ef.mutex.Lock()
	defer ef.mutex.Unlock()

	ef.concurrentInstalls++

	cleanup := func() {
		ef.mutex.Lock()
		defer ef.mutex.Unlock()

		ef.concurrentInstalls--
		// If there are no other concurrent installs happening in parallel,
		// cleanup the build lock mapping to release memory
		if ef.concurrentInstalls == 0 {
			ef.BuildLocks = nil
		}
	}

	if ef.BuildLocks == nil {
		ef.BuildLocks = map[string]*sync.Mutex{}
	}

	_, ok := ef.BuildLocks[packageID]
	if !ok {
		ef.BuildLocks[packageID] = &sync.Mutex{}
	}

	return ef.BuildLocks[packageID], cleanup
}

// GetInstalledChaincodePackage retrieves the installed chaincode with the given package ID
// from the peer's chaincode store.
func (ef *ExternalFunctions) GetInstalledChaincodePackage(packageID string) ([]byte, error) {
	pkgBytes, err := ef.Resources.ChaincodeStore.Load(packageID)
	if err != nil {
		return nil, errors.WithMessage(err, "could not load cc install package")
	}

	return pkgBytes, nil
}

// QueryNamespaceDefinitions lists the publicly defined namespaces in a channel.  Today it should only ever
// find Datatype encodings of 'ChaincodeDefinition'.
func (ef *ExternalFunctions) QueryNamespaceDefinitions(publicState RangeableState) (map[string]string, error) {
	metadatas, err := ef.Resources.Serializer.DeserializeAllMetadata(NamespacesName, publicState)
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

// QueryInstalledChaincode returns metadata for the chaincode with the supplied package ID.
func (ef *ExternalFunctions) QueryInstalledChaincode(packageID string) (*chaincode.InstalledChaincode, error) {
	return ef.InstalledChaincodesLister.GetInstalledChaincode(packageID)
}

// QueryInstalledChaincodes returns a list of installed chaincodes
func (ef *ExternalFunctions) QueryInstalledChaincodes() []*chaincode.InstalledChaincode {
	return ef.InstalledChaincodesLister.ListInstalledChaincodes()
}
