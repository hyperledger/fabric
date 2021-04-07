/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"regexp"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/common"
	mspprotos "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/dispatcher"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	// LifecycleNamespace is the namespace in the statedb where lifecycle
	// information is stored
	LifecycleNamespace = "_lifecycle"

	// InstallChaincodeFuncName is the chaincode function name used to install
	// a chaincode
	InstallChaincodeFuncName = "InstallChaincode"

	// QueryInstalledChaincodeFuncName is the chaincode function name used to
	// query an installed chaincode
	QueryInstalledChaincodeFuncName = "QueryInstalledChaincode"

	// QueryInstalledChaincodesFuncName is the chaincode function name used to
	// query all installed chaincodes
	QueryInstalledChaincodesFuncName = "QueryInstalledChaincodes"

	// ApproveChaincodeDefinitionForMyOrgFuncName is the chaincode function name
	// used to approve a chaincode definition for execution by the user's own org
	ApproveChaincodeDefinitionForMyOrgFuncName = "ApproveChaincodeDefinitionForMyOrg"

	// QueryApprovedChaincodeDefinitionFuncName is the chaincode function name used to
	// query a approved chaincode definition for the user's own org
	QueryApprovedChaincodeDefinitionFuncName = "QueryApprovedChaincodeDefinition"

	// CheckCommitReadinessFuncName is the chaincode function name used to check
	// a specified chaincode definition is ready to be committed. It returns the
	// approval status for a given definition over a given set of orgs
	CheckCommitReadinessFuncName = "CheckCommitReadiness"

	// CommitChaincodeDefinitionFuncName is the chaincode function name used to
	// 'commit' (previously 'instantiate') a chaincode in a channel.
	CommitChaincodeDefinitionFuncName = "CommitChaincodeDefinition"

	// QueryChaincodeDefinitionFuncName is the chaincode function name used to
	// query a committed chaincode definition in a channel.
	QueryChaincodeDefinitionFuncName = "QueryChaincodeDefinition"

	// QueryChaincodeDefinitionsFuncName is the chaincode function name used to
	// query the committed chaincode definitions in a channel.
	QueryChaincodeDefinitionsFuncName = "QueryChaincodeDefinitions"
)

// SCCFunctions provides a backing implementation with concrete arguments
// for each of the SCC functions
type SCCFunctions interface {
	// InstallChaincode persists a chaincode definition to disk
	InstallChaincode([]byte) (*chaincode.InstalledChaincode, error)

	// QueryInstalledChaincode returns metadata for the chaincode with the supplied package ID.
	QueryInstalledChaincode(packageID string) (*chaincode.InstalledChaincode, error)

	// GetInstalledChaincodePackage returns the chaincode package
	// installed on the peer as bytes.
	GetInstalledChaincodePackage(packageID string) ([]byte, error)

	// QueryInstalledChaincodes returns the currently installed chaincodes
	QueryInstalledChaincodes() []*chaincode.InstalledChaincode

	// ApproveChaincodeDefinitionForOrg records a chaincode definition into this org's implicit collection.
	ApproveChaincodeDefinitionForOrg(chname, ccname string, cd *ChaincodeDefinition, packageID string, publicState ReadableState, orgState ReadWritableState) error

	// QueryApprovedChaincodeDefinition returns a approved chaincode definition from this org's implicit collection.
	QueryApprovedChaincodeDefinition(chname, ccname string, sequence int64, publicState ReadableState, orgState ReadableState) (*ApprovedChaincodeDefinition, error)

	// CheckCommitReadiness returns a map containing the orgs
	// whose orgStates were supplied and whether or not they have approved
	// the specified definition.
	CheckCommitReadiness(chname, ccname string, cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) (map[string]bool, error)

	// CommitChaincodeDefinition records a new chaincode definition into the
	// public state and returns a map containing the orgs whose orgStates
	// were supplied and whether or not they have approved the definition.
	CommitChaincodeDefinition(chname, ccname string, cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) (map[string]bool, error)

	// QueryChaincodeDefinition returns a chaincode definition from the public
	// state.
	QueryChaincodeDefinition(name string, publicState ReadableState) (*ChaincodeDefinition, error)

	// QueryOrgApprovals returns a map containing the orgs whose orgStates were
	// supplied and whether or not they have approved a chaincode definition with
	// the specified parameters.
	QueryOrgApprovals(name string, cd *ChaincodeDefinition, orgStates []OpaqueState) (map[string]bool, error)

	// QueryNamespaceDefinitions returns all defined namespaces
	QueryNamespaceDefinitions(publicState RangeableState) (map[string]string, error)
}

//go:generate counterfeiter -o mock/channel_config_source.go --fake-name ChannelConfigSource . ChannelConfigSource

// ChannelConfigSource provides a way to retrieve the channel config for a given
// channel ID.
type ChannelConfigSource interface {
	// GetStableChannelConfig returns the channel config for a given channel id.
	// Note, it is a stable bundle, which means it will not be updated, even if
	// the channel is, so it should be discarded after use.
	GetStableChannelConfig(channelID string) channelconfig.Resources
}

//go:generate counterfeiter -o mock/queryexecutor_provider.go --fake-name QueryExecutorProvider . QueryExecutorProvider

// QueryExecutorProvider provides a way to retrieve the query executor assosciated with an invocation
type QueryExecutorProvider interface {
	TxQueryExecutor(channelID, txID string) ledger.SimpleQueryExecutor
}

// SCC implements the required methods to satisfy the chaincode interface.
// It routes the invocation calls to the backing implementations.
type SCC struct {
	OrgMSPID string

	ACLProvider aclmgmt.ACLProvider

	ChannelConfigSource ChannelConfigSource

	DeployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
	QueryExecutorProvider  QueryExecutorProvider

	// Functions provides the backing implementation of lifecycle.
	Functions SCCFunctions

	// Dispatcher handles the rote protobuf boilerplate for unmarshalling/marshaling
	// the inputs and outputs of the SCC functions.
	Dispatcher *dispatcher.Dispatcher
}

// Name returns "_lifecycle"
func (scc *SCC) Name() string {
	return LifecycleNamespace
}

// Chaincode returns a reference to itself
func (scc *SCC) Chaincode() shim.Chaincode {
	return scc
}

// Init is mostly useless for system chaincodes and always returns success
func (scc *SCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Invoke takes chaincode invocation arguments and routes them to the correct
// underlying lifecycle operation.  All functions take a single argument of
// type marshaled lb.<FunctionName>Args and return a marshaled lb.<FunctionName>Result
func (scc *SCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) == 0 {
		return shim.Error("lifecycle scc must be invoked with arguments")
	}

	if len(args) != 2 {
		return shim.Error(fmt.Sprintf("lifecycle scc operations require exactly two arguments but received %d", len(args)))
	}

	var ac channelconfig.Application
	var channelID string
	if channelID = stub.GetChannelID(); channelID != "" {
		channelConfig := scc.ChannelConfigSource.GetStableChannelConfig(channelID)
		if channelConfig == nil {
			return shim.Error(fmt.Sprintf("could not get channelconfig for channel '%s'", channelID))
		}
		var ok bool
		ac, ok = channelConfig.ApplicationConfig()
		if !ok {
			return shim.Error(fmt.Sprintf("could not get application config for channel '%s'", channelID))
		}
		if !ac.Capabilities().LifecycleV20() {
			return shim.Error(fmt.Sprintf("cannot use new lifecycle for channel '%s' as it does not have the required capabilities enabled", channelID))
		}
	}

	// Handle ACL:
	sp, err := stub.GetSignedProposal()
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed getting signed proposal from stub: [%s]", err))
	}

	err = scc.ACLProvider.CheckACL(fmt.Sprintf("%s/%s", LifecycleNamespace, args[0]), stub.GetChannelID(), sp)
	if err != nil {
		return shim.Error(fmt.Sprintf("Failed to authorize invocation due to failed ACL check: %s", err))
	}

	outputBytes, err := scc.Dispatcher.Dispatch(
		args[1],
		string(args[0]),
		&Invocation{
			ChannelID:         channelID,
			ApplicationConfig: ac,
			SCC:               scc,
			Stub:              stub,
		},
	)
	if err != nil {
		switch err.(type) {
		case ErrNamespaceNotDefined, persistence.CodePackageNotFoundErr:
			return pb.Response{
				Status:  404,
				Message: err.Error(),
			}
		default:
			return shim.Error(fmt.Sprintf("failed to invoke backing implementation of '%s': %s", string(args[0]), err.Error()))
		}
	}

	return shim.Success(outputBytes)
}

type Invocation struct {
	ChannelID         string
	ApplicationConfig channelconfig.Application // Note this may be nil
	Stub              shim.ChaincodeStubInterface
	SCC               *SCC
}

// InstallChaincode is a SCC function that may be dispatched to which routes
// to the underlying lifecycle implementation.
func (i *Invocation) InstallChaincode(input *lb.InstallChaincodeArgs) (proto.Message, error) {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		end := 35
		if len(input.ChaincodeInstallPackage) < end {
			end = len(input.ChaincodeInstallPackage)
		}

		// the first tens of bytes contain the (compressed) portion
		// of the package metadata and so they'll be different across
		// different packages, acting as a package fingerprint useful
		// to identify various packages from the content
		packageFingerprint := input.ChaincodeInstallPackage[0:end]
		logger.Debugf("received invocation of InstallChaincode for install package %x...",
			packageFingerprint,
		)
	}

	installedCC, err := i.SCC.Functions.InstallChaincode(input.ChaincodeInstallPackage)
	if err != nil {
		return nil, err
	}

	return &lb.InstallChaincodeResult{
		Label:     installedCC.Label,
		PackageId: installedCC.PackageID,
	}, nil
}

// QueryInstalledChaincode is a SCC function that may be dispatched to which
// routes to the underlying lifecycle implementation.
func (i *Invocation) QueryInstalledChaincode(input *lb.QueryInstalledChaincodeArgs) (proto.Message, error) {
	logger.Debugf("received invocation of QueryInstalledChaincode for install package ID '%s'",
		input.PackageId,
	)

	chaincode, err := i.SCC.Functions.QueryInstalledChaincode(input.PackageId)
	if err != nil {
		return nil, err
	}

	references := map[string]*lb.QueryInstalledChaincodeResult_References{}
	for channel, chaincodeMetadata := range chaincode.References {
		chaincodes := make([]*lb.QueryInstalledChaincodeResult_Chaincode, len(chaincodeMetadata))
		for i, metadata := range chaincodeMetadata {
			chaincodes[i] = &lb.QueryInstalledChaincodeResult_Chaincode{
				Name:    metadata.Name,
				Version: metadata.Version,
			}
		}

		references[channel] = &lb.QueryInstalledChaincodeResult_References{
			Chaincodes: chaincodes,
		}
	}

	return &lb.QueryInstalledChaincodeResult{
		Label:      chaincode.Label,
		PackageId:  chaincode.PackageID,
		References: references,
	}, nil
}

// GetInstalledChaincodePackage is a SCC function that may be dispatched to
// which routes to the underlying lifecycle implementation.
func (i *Invocation) GetInstalledChaincodePackage(input *lb.GetInstalledChaincodePackageArgs) (proto.Message, error) {
	logger.Debugf("received invocation of GetInstalledChaincodePackage")

	pkgBytes, err := i.SCC.Functions.GetInstalledChaincodePackage(input.PackageId)
	if err != nil {
		return nil, err
	}

	return &lb.GetInstalledChaincodePackageResult{
		ChaincodeInstallPackage: pkgBytes,
	}, nil
}

// QueryInstalledChaincodes is a SCC function that may be dispatched to which
// routes to the underlying lifecycle implementation.
func (i *Invocation) QueryInstalledChaincodes(input *lb.QueryInstalledChaincodesArgs) (proto.Message, error) {
	logger.Debugf("received invocation of QueryInstalledChaincodes")

	chaincodes := i.SCC.Functions.QueryInstalledChaincodes()

	result := &lb.QueryInstalledChaincodesResult{}
	for _, chaincode := range chaincodes {
		references := map[string]*lb.QueryInstalledChaincodesResult_References{}
		for channel, chaincodeMetadata := range chaincode.References {
			chaincodes := make([]*lb.QueryInstalledChaincodesResult_Chaincode, len(chaincodeMetadata))
			for i, metadata := range chaincodeMetadata {
				chaincodes[i] = &lb.QueryInstalledChaincodesResult_Chaincode{
					Name:    metadata.Name,
					Version: metadata.Version,
				}
			}

			references[channel] = &lb.QueryInstalledChaincodesResult_References{
				Chaincodes: chaincodes,
			}
		}

		result.InstalledChaincodes = append(result.InstalledChaincodes,
			&lb.QueryInstalledChaincodesResult_InstalledChaincode{
				Label:      chaincode.Label,
				PackageId:  chaincode.PackageID,
				References: references,
			})
	}

	return result, nil
}

// ApproveChaincodeDefinitionForMyOrg is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) ApproveChaincodeDefinitionForMyOrg(input *lb.ApproveChaincodeDefinitionForMyOrgArgs) (proto.Message, error) {
	if err := i.validateInput(input.Name, input.Version, input.Collections); err != nil {
		return nil, errors.WithMessage(err, "error validating chaincode definition")
	}
	collectionName := implicitcollection.NameForOrg(i.SCC.OrgMSPID)
	var collectionConfig []*pb.CollectionConfig
	if input.Collections != nil {
		collectionConfig = input.Collections.Config
	}

	var packageID string
	if input.Source != nil {
		switch source := input.Source.Type.(type) {
		case *lb.ChaincodeSource_LocalPackage:
			packageID = source.LocalPackage.PackageId
		case *lb.ChaincodeSource_Unavailable_:
		default:
		}
	}

	cd := &ChaincodeDefinition{
		Sequence: input.Sequence,
		EndorsementInfo: &lb.ChaincodeEndorsementInfo{
			Version:           input.Version,
			EndorsementPlugin: input.EndorsementPlugin,
			InitRequired:      input.InitRequired,
		},
		ValidationInfo: &lb.ChaincodeValidationInfo{
			ValidationPlugin:    input.ValidationPlugin,
			ValidationParameter: input.ValidationParameter,
		},
		Collections: &pb.CollectionConfigPackage{
			Config: collectionConfig,
		},
	}

	logger.Debugf("received invocation of ApproveChaincodeDefinitionForMyOrg on channel '%s' for definition '%s'",
		i.Stub.GetChannelID(),
		cd,
	)

	if err := i.SCC.Functions.ApproveChaincodeDefinitionForOrg(
		i.Stub.GetChannelID(),
		input.Name,
		cd,
		packageID,
		i.Stub,
		&ChaincodePrivateLedgerShim{
			Collection: collectionName,
			Stub:       i.Stub,
		},
	); err != nil {
		return nil, err
	}
	return &lb.ApproveChaincodeDefinitionForMyOrgResult{}, nil
}

// QueryApprovedChaincodeDefinition is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) QueryApprovedChaincodeDefinition(input *lb.QueryApprovedChaincodeDefinitionArgs) (proto.Message, error) {
	logger.Debugf("received invocation of QueryApprovedChaincodeDefinition on channel '%s' for chaincode '%s'",
		i.Stub.GetChannelID(),
		input.Name,
	)
	collectionName := implicitcollection.NameForOrg(i.SCC.OrgMSPID)

	ca, err := i.SCC.Functions.QueryApprovedChaincodeDefinition(
		i.Stub.GetChannelID(),
		input.Name,
		input.Sequence,
		i.Stub,
		&ChaincodePrivateLedgerShim{
			Collection: collectionName,
			Stub:       i.Stub,
		},
	)
	if err != nil {
		return nil, err
	}

	return &lb.QueryApprovedChaincodeDefinitionResult{
		Sequence:            ca.Sequence,
		Version:             ca.EndorsementInfo.Version,
		EndorsementPlugin:   ca.EndorsementInfo.EndorsementPlugin,
		ValidationPlugin:    ca.ValidationInfo.ValidationPlugin,
		ValidationParameter: ca.ValidationInfo.ValidationParameter,
		InitRequired:        ca.EndorsementInfo.InitRequired,
		Collections:         ca.Collections,
		Source:              ca.Source,
	}, nil
}

// CheckCommitReadiness is a SCC function that may be dispatched
// to the underlying lifecycle implementation.
func (i *Invocation) CheckCommitReadiness(input *lb.CheckCommitReadinessArgs) (proto.Message, error) {
	opaqueStates, err := i.createOpaqueStates()
	if err != nil {
		return nil, err
	}

	cd := &ChaincodeDefinition{
		Sequence: input.Sequence,
		EndorsementInfo: &lb.ChaincodeEndorsementInfo{
			Version:           input.Version,
			EndorsementPlugin: input.EndorsementPlugin,
			InitRequired:      input.InitRequired,
		},
		ValidationInfo: &lb.ChaincodeValidationInfo{
			ValidationPlugin:    input.ValidationPlugin,
			ValidationParameter: input.ValidationParameter,
		},
		Collections: input.Collections,
	}

	logger.Debugf("received invocation of CheckCommitReadiness on channel '%s' for definition '%s'",
		i.Stub.GetChannelID(),
		cd,
	)

	approvals, err := i.SCC.Functions.CheckCommitReadiness(
		i.Stub.GetChannelID(),
		input.Name,
		cd,
		i.Stub,
		opaqueStates,
	)
	if err != nil {
		return nil, err
	}

	return &lb.CheckCommitReadinessResult{
		Approvals: approvals,
	}, nil
}

// CommitChaincodeDefinition is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) CommitChaincodeDefinition(input *lb.CommitChaincodeDefinitionArgs) (proto.Message, error) {
	if err := i.validateInput(input.Name, input.Version, input.Collections); err != nil {
		return nil, errors.WithMessage(err, "error validating chaincode definition")
	}

	if i.ApplicationConfig == nil {
		return nil, errors.Errorf("no application config for channel '%s'", i.Stub.GetChannelID())
	}

	orgs := i.ApplicationConfig.Organizations()
	opaqueStates := make([]OpaqueState, 0, len(orgs))
	var myOrg string
	for _, org := range orgs {
		opaqueStates = append(opaqueStates, &ChaincodePrivateLedgerShim{
			Collection: implicitcollection.NameForOrg(org.MSPID()),
			Stub:       i.Stub,
		})
		if org.MSPID() == i.SCC.OrgMSPID {
			myOrg = i.SCC.OrgMSPID
		}
	}

	if myOrg == "" {
		return nil, errors.Errorf("impossibly, this peer's org is processing requests for a channel it is not a member of")
	}

	cd := &ChaincodeDefinition{
		Sequence: input.Sequence,
		EndorsementInfo: &lb.ChaincodeEndorsementInfo{
			Version:           input.Version,
			EndorsementPlugin: input.EndorsementPlugin,
			InitRequired:      input.InitRequired,
		},
		ValidationInfo: &lb.ChaincodeValidationInfo{
			ValidationPlugin:    input.ValidationPlugin,
			ValidationParameter: input.ValidationParameter,
		},
		Collections: input.Collections,
	}

	logger.Debugf("received invocation of CommitChaincodeDefinition on channel '%s' for definition '%s'",
		i.Stub.GetChannelID(),
		cd,
	)

	approvals, err := i.SCC.Functions.CommitChaincodeDefinition(
		i.Stub.GetChannelID(),
		input.Name,
		cd,
		i.Stub,
		opaqueStates,
	)
	if err != nil {
		return nil, err
	}

	if !approvals[myOrg] {
		return nil, errors.Errorf("chaincode definition not agreed to by this org (%s)", i.SCC.OrgMSPID)
	}

	logger.Infof("Successfully endorsed commit for chaincode name '%s' on channel '%s' with definition {%s}", input.Name, i.Stub.GetChannelID(), cd)

	return &lb.CommitChaincodeDefinitionResult{}, nil
}

// QueryChaincodeDefinition is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) QueryChaincodeDefinition(input *lb.QueryChaincodeDefinitionArgs) (proto.Message, error) {
	logger.Debugf("received invocation of QueryChaincodeDefinition on channel '%s' for chaincode '%s'",
		i.Stub.GetChannelID(),
		input.Name,
	)

	definedChaincode, err := i.SCC.Functions.QueryChaincodeDefinition(input.Name, i.Stub)
	if err != nil {
		return nil, err
	}

	opaqueStates, err := i.createOpaqueStates()
	if err != nil {
		return nil, err
	}

	var approvals map[string]bool
	if approvals, err = i.SCC.Functions.QueryOrgApprovals(input.Name, definedChaincode, opaqueStates); err != nil {
		return nil, err
	}

	return &lb.QueryChaincodeDefinitionResult{
		Sequence:            definedChaincode.Sequence,
		Version:             definedChaincode.EndorsementInfo.Version,
		EndorsementPlugin:   definedChaincode.EndorsementInfo.EndorsementPlugin,
		ValidationPlugin:    definedChaincode.ValidationInfo.ValidationPlugin,
		ValidationParameter: definedChaincode.ValidationInfo.ValidationParameter,
		InitRequired:        definedChaincode.EndorsementInfo.InitRequired,
		Collections:         definedChaincode.Collections,
		Approvals:           approvals,
	}, nil
}

// QueryChaincodeDefinitions is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) QueryChaincodeDefinitions(input *lb.QueryChaincodeDefinitionsArgs) (proto.Message, error) {
	logger.Debugf("received invocation of QueryChaincodeDefinitions on channel '%s'",
		i.Stub.GetChannelID(),
	)

	namespaces, err := i.SCC.Functions.QueryNamespaceDefinitions(&ChaincodePublicLedgerShim{ChaincodeStubInterface: i.Stub})
	if err != nil {
		return nil, err
	}

	chaincodeDefinitions := []*lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition{}
	for namespace, nType := range namespaces {
		if nType == FriendlyChaincodeDefinitionType {
			definedChaincode, err := i.SCC.Functions.QueryChaincodeDefinition(namespace, i.Stub)
			if err != nil {
				return nil, err
			}

			chaincodeDefinitions = append(chaincodeDefinitions, &lb.QueryChaincodeDefinitionsResult_ChaincodeDefinition{
				Name:                namespace,
				Sequence:            definedChaincode.Sequence,
				Version:             definedChaincode.EndorsementInfo.Version,
				EndorsementPlugin:   definedChaincode.EndorsementInfo.EndorsementPlugin,
				ValidationPlugin:    definedChaincode.ValidationInfo.ValidationPlugin,
				ValidationParameter: definedChaincode.ValidationInfo.ValidationParameter,
				InitRequired:        definedChaincode.EndorsementInfo.InitRequired,
				Collections:         definedChaincode.Collections,
			})
		}
	}

	return &lb.QueryChaincodeDefinitionsResult{
		ChaincodeDefinitions: chaincodeDefinitions,
	}, nil
}

var (
	// NOTE the chaincode name/version regular expressions should stay in sync
	// with those defined in core/scc/lscc/lscc.go until LSCC has been removed.
	ChaincodeNameRegExp    = regexp.MustCompile("^[a-zA-Z0-9]+([-_][a-zA-Z0-9]+)*$")
	ChaincodeVersionRegExp = regexp.MustCompile("^[A-Za-z0-9_.+-]+$")

	collectionNameRegExp = regexp.MustCompile("^[A-Za-z0-9-]+([A-Za-z0-9_-]+)*$")

	// currently defined system chaincode names that shouldn't
	// be allowed as user-defined chaincode names
	systemChaincodeNames = map[string]struct{}{
		"cscc": {},
		"escc": {},
		"lscc": {},
		"qscc": {},
		"vscc": {},
	}
)

func (i *Invocation) validateInput(name, version string, collections *pb.CollectionConfigPackage) error {
	if !ChaincodeNameRegExp.MatchString(name) {
		return errors.Errorf("invalid chaincode name '%s'. Names can only consist of alphanumerics, '_', and '-' and can only begin with alphanumerics", name)
	}
	if _, ok := systemChaincodeNames[name]; ok {
		return errors.Errorf("chaincode name '%s' is the name of a system chaincode", name)
	}

	if !ChaincodeVersionRegExp.MatchString(version) {
		return errors.Errorf("invalid chaincode version '%s'. Versions can only consist of alphanumerics, '_', '-', '+', and '.'", version)
	}

	collConfigs, err := extractStaticCollectionConfigs(collections)
	if err != nil {
		return err
	}
	// we extract the channel config to check whether the supplied collection configuration
	// complies to the given msp configuration and performs semantic validation.
	// Channel config may change afterwards (i.e., after endorsement or commit of this transaction).
	// Fabric will deal with the situation where some collection configs are no longer meaningful.
	// Therefore, the use of channel config for verifying during endorsement is more
	// towards catching manual errors in the config as oppose to any attempt of serializability.
	channelConfig := i.SCC.ChannelConfigSource.GetStableChannelConfig(i.ChannelID)
	if channelConfig == nil {
		return errors.Errorf("could not get channelconfig for channel '%s'", i.ChannelID)
	}
	mspMgr := channelConfig.MSPManager()
	if mspMgr == nil {
		return errors.Errorf("could not get MSP manager for channel '%s'", i.ChannelID)
	}

	if err := validateCollectionConfigs(collConfigs, mspMgr); err != nil {
		return err
	}

	// validate against collection configs in the committed definition
	qe := i.SCC.QueryExecutorProvider.TxQueryExecutor(i.Stub.GetChannelID(), i.Stub.GetTxID())
	committedCCDef, err := i.SCC.DeployedCCInfoProvider.ChaincodeInfo(i.ChannelID, name, qe)
	if err != nil {
		return errors.Wrapf(err, "could not retrieve committed definition for chaincode '%s'", name)
	}
	if committedCCDef == nil {
		return nil
	}
	if err := validateCollConfigsAgainstCommittedDef(collConfigs, committedCCDef.ExplicitCollectionConfigPkg); err != nil {
		return err
	}
	return nil
}

func extractStaticCollectionConfigs(collConfigPkg *pb.CollectionConfigPackage) ([]*pb.StaticCollectionConfig, error) {
	if collConfigPkg == nil || len(collConfigPkg.Config) == 0 {
		return nil, nil
	}
	collConfigs := make([]*pb.StaticCollectionConfig, len(collConfigPkg.Config))
	for i, c := range collConfigPkg.Config {
		switch t := c.Payload.(type) {
		case *pb.CollectionConfig_StaticCollectionConfig:
			collConfig := t.StaticCollectionConfig
			if collConfig == nil {
				return nil, errors.Errorf("collection configuration is empty")
			}
			collConfigs[i] = collConfig
		default:
			// this should only occur if a developer has added a new
			// collection config type
			return nil, errors.Errorf("collection config contains unexpected payload type: %T", t)
		}
	}
	return collConfigs, nil
}

func validateCollectionConfigs(collConfigs []*pb.StaticCollectionConfig, mspMgr msp.MSPManager) error {
	if len(collConfigs) == 0 {
		return nil
	}
	collNamesMap := map[string]struct{}{}
	// Process each collection config from a set of collection configs
	for _, c := range collConfigs {
		if !collectionNameRegExp.MatchString(c.Name) {
			return errors.Errorf("invalid collection name '%s'. Names can only consist of alphanumerics, '_', and '-' and cannot begin with '_'",
				c.Name)
		}
		// Ensure that there are no duplicate collection names
		if _, ok := collNamesMap[c.Name]; ok {
			return errors.Errorf("collection-name: %s -- found duplicate in collection configuration",
				c.Name)
		}
		collNamesMap[c.Name] = struct{}{}
		// Validate gossip related parameters present in the collection config
		if c.MaximumPeerCount < c.RequiredPeerCount {
			return errors.Errorf("collection-name: %s -- maximum peer count (%d) cannot be less than the required peer count (%d)",
				c.Name, c.MaximumPeerCount, c.RequiredPeerCount)
		}
		if c.RequiredPeerCount < 0 {
			return errors.Errorf("collection-name: %s -- requiredPeerCount (%d) cannot be less than zero",
				c.Name, c.RequiredPeerCount)
		}
		if err := validateCollectionConfigMemberOrgsPolicy(c, mspMgr); err != nil {
			return err
		}
	}
	return nil
}

// validateCollectionConfigAgainstMsp checks whether the supplied collection configuration
// complies to the given msp configuration
func validateCollectionConfigMemberOrgsPolicy(coll *pb.StaticCollectionConfig, mspMgr msp.MSPManager) error {
	if coll.MemberOrgsPolicy == nil {
		return errors.Errorf("collection member policy is not set for collection '%s'", coll.Name)
	}
	if coll.MemberOrgsPolicy.GetSignaturePolicy() == nil {
		return errors.Errorf("collection member org policy is empty for collection '%s'", coll.Name)
	}

	// calling this constructor ensures extra semantic validation for the policy
	pp := &cauthdsl.EnvelopeBasedPolicyProvider{Deserializer: mspMgr}
	if _, err := pp.NewPolicy(coll.MemberOrgsPolicy.GetSignaturePolicy()); err != nil {
		return errors.WithMessagef(err, "invalid member org policy for collection '%s'", coll.Name)
	}

	// make sure that the signature policy is meaningful (only consists of ORs)
	if err := validateSpOrConcat(coll.MemberOrgsPolicy.GetSignaturePolicy().Rule); err != nil {
		return errors.WithMessagef(err, "collection-name: %s -- error in member org policy", coll.Name)
	}

	msps, err := mspMgr.GetMSPs()
	if err != nil {
		return errors.Wrapf(err, "could not get MSPs")
	}

	// make sure that the orgs listed are actually part of the channel
	// check all principals in the signature policy
	for _, principal := range coll.MemberOrgsPolicy.GetSignaturePolicy().Identities {
		var orgID string
		// the member org policy only supports certain principal types
		switch principal.PrincipalClassification {

		case mspprotos.MSPPrincipal_ROLE:
			msprole := &mspprotos.MSPRole{}
			err := proto.Unmarshal(principal.Principal, msprole)
			if err != nil {
				return errors.Wrapf(err, "collection-name: %s -- cannot unmarshal identity bytes into MSPRole", coll.GetName())
			}
			orgID = msprole.MspIdentifier
			// the msp map is indexed using msp IDs - this behavior is implementation specific, making the following check a bit of a hack
			_, ok := msps[orgID]
			if !ok {
				return errors.Errorf("collection-name: %s -- collection member '%s' is not part of the channel", coll.GetName(), orgID)
			}

		case mspprotos.MSPPrincipal_ORGANIZATION_UNIT:
			mspou := &mspprotos.OrganizationUnit{}
			err := proto.Unmarshal(principal.Principal, mspou)
			if err != nil {
				return errors.Wrapf(err, "collection-name: %s -- cannot unmarshal identity bytes into OrganizationUnit", coll.GetName())
			}
			orgID = mspou.MspIdentifier
			// the msp map is indexed using msp IDs - this behavior is implementation specific, making the following check a bit of a hack
			_, ok := msps[orgID]
			if !ok {
				return errors.Errorf("collection-name: %s -- collection member '%s' is not part of the channel", coll.GetName(), orgID)
			}

		case mspprotos.MSPPrincipal_IDENTITY:
			if _, err := mspMgr.DeserializeIdentity(principal.Principal); err != nil {
				return errors.Errorf("collection-name: %s -- contains an identity that is not part of the channel", coll.GetName())
			}

		default:
			return errors.Errorf("collection-name: %s -- principal type %v is not supported", coll.GetName(), principal.PrincipalClassification)
		}
	}
	return nil
}

// validateSpOrConcat checks if the supplied signature policy is just an OR-concatenation of identities
func validateSpOrConcat(sp *common.SignaturePolicy) error {
	if sp.GetNOutOf() == nil {
		return nil
	}
	// check if N == 1 (OR concatenation)
	if sp.GetNOutOf().N != 1 {
		return errors.Errorf("signature policy is not an OR concatenation, NOutOf %d", sp.GetNOutOf().N)
	}
	// recurse into all sub-rules
	for _, rule := range sp.GetNOutOf().Rules {
		err := validateSpOrConcat(rule)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateCollConfigsAgainstCommittedDef(
	proposedCollConfs []*pb.StaticCollectionConfig,
	committedCollConfPkg *pb.CollectionConfigPackage,
) error {
	if committedCollConfPkg == nil || len(committedCollConfPkg.Config) == 0 {
		return nil
	}

	if len(proposedCollConfs) == 0 {
		return errors.Errorf("the proposed collection config does not contain previously defined collections")
	}

	proposedCollsMap := map[string]*pb.StaticCollectionConfig{}
	for _, c := range proposedCollConfs {
		proposedCollsMap[c.Name] = c
	}

	// In the new collection config package, ensure that there is one entry per old collection. Any
	// number of new collections are allowed.
	for _, committedCollConfig := range committedCollConfPkg.Config {
		committedColl := committedCollConfig.GetStaticCollectionConfig()
		// It cannot be nil
		if committedColl == nil {
			return errors.Errorf("unknown collection configuration type")
		}

		newCollection, ok := proposedCollsMap[committedColl.Name]
		if !ok {
			return errors.Errorf("existing collection [%s] missing in the proposed collection configuration", committedColl.Name)
		}

		if newCollection.BlockToLive != committedColl.BlockToLive {
			return errors.Errorf("the BlockToLive in an existing collection [%s] modified. Existing value [%d]", committedColl.Name, committedColl.BlockToLive)
		}
	}
	return nil
}

func (i *Invocation) createOpaqueStates() ([]OpaqueState, error) {
	if i.ApplicationConfig == nil {
		return nil, errors.Errorf("no application config for channel '%s'", i.Stub.GetChannelID())
	}
	orgs := i.ApplicationConfig.Organizations()
	opaqueStates := make([]OpaqueState, 0, len(orgs))
	for _, org := range orgs {
		opaqueStates = append(opaqueStates, &ChaincodePrivateLedgerShim{
			Collection: implicitcollection.NameForOrg(org.MSPID()),
			Stub:       i.Stub,
		})
	}
	return opaqueStates, nil
}
