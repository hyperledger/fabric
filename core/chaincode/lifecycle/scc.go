/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"regexp"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	persistenceintf "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/dispatcher"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

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

	// QueryApprovalStatusFuncName is the chaincode function name used to query
	// the approval status for a given definition over a given set of orgs
	QueryApprovalStatusFuncName = "QueryApprovalStatus"

	// CommitChaincodeDefinitionFuncName is the chaincode function name used to
	// 'commit' (previously 'instantiate') a chaincode in a channel.
	CommitChaincodeDefinitionFuncName = "CommitChaincodeDefinition"

	// QueryChaincodeDefinitionFuncName is the chaincode function name used to
	// query the committed chaincode definitions in a channel.
	QueryChaincodeDefinitionFuncName = "QueryChaincodeDefinition"

	// QueryNamespaceDefinitionsFuncName is the chaincode function name used
	// to query which namespaces are currently defined and what type those
	// namespaces are.
	QueryNamespaceDefinitionsFuncName = "QueryNamespaceDefinitions"
)

// SCCFunctions provides a backing implementation with concrete arguments
// for each of the SCC functions
type SCCFunctions interface {
	// InstallChaincode persists a chaincode definition to disk
	InstallChaincode([]byte) (*chaincode.InstalledChaincode, error)

	// QueryInstalledChaincode returns the hash for a given name and version of an installed chaincode
	QueryInstalledChaincode(packageID persistenceintf.PackageID) (*chaincode.InstalledChaincode, error)

	// QueryInstalledChaincodes returns the currently installed chaincodes
	QueryInstalledChaincodes() (chaincodes []chaincode.InstalledChaincode, err error)

	// ApproveChaincodeDefinitionForOrg records a chaincode definition into this org's implicit collection.
	ApproveChaincodeDefinitionForOrg(chname, ccname string, cd *ChaincodeDefinition, packageID persistenceintf.PackageID, publicState ReadableState, orgState ReadWritableState) error

	// QueryApprovalStatus returns an array of boolean to signal whether the orgs
	// whose orgStates was supplied as argument have approveed the specified definition
	QueryApprovalStatus(chname, ccname string, cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) ([]bool, error)

	// CommitChaincodeDefinition records a new chaincode definition into the public state and returns the orgs which agreed with that definition.
	CommitChaincodeDefinition(chname, ccname string, cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) ([]bool, error)

	// QueryChaincodeDefinition reads a chaincode definition from the public state.
	QueryChaincodeDefinition(name string, publicState ReadableState) (*ChaincodeDefinition, error)

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

	// Dispatcher handles the rote protobuf boilerplate for unmarshaling/marshaling
	// the inputs and outputs of the SCC functions.
	Dispatcher *dispatcher.Dispatcher
}

// Name returns "_lifecycle"
func (scc *SCC) Name() string {
	return LifecycleNamespace
}

// Path returns "github.com/hyperledger/fabric/core/chaincode/lifecycle"
func (scc *SCC) Path() string {
	return "github.com/hyperledger/fabric/core/chaincode/lifecycle"
}

// InitArgs returns nil
func (scc *SCC) InitArgs() [][]byte {
	return nil
}

// Chaincode returns a reference to itself
func (scc *SCC) Chaincode() shim.Chaincode {
	return scc
}

// InvokableExternal returns true
func (scc *SCC) InvokableExternal() bool {
	return true
}

// InvokableCC2CC returns true
func (scc *SCC) InvokableCC2CC() bool {
	return true
}

// Enabled returns true
func (scc *SCC) Enabled() bool {
	return true
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
		PackageId: installedCC.PackageID.String(),
	}, nil
}

// QueryInstalledChaincode is a SCC function that may be dispatched to which
// routes to the underlying lifecycle implementation.
func (i *Invocation) QueryInstalledChaincode(input *lb.QueryInstalledChaincodeArgs) (proto.Message, error) {

	logger.Debugf("received invocation of QueryInstalledChaincode for install package ID '%s'",
		input.PackageId,
	)

	chaincode, err := i.SCC.Functions.QueryInstalledChaincode(persistenceintf.PackageID(input.PackageId))
	if err != nil {
		return nil, err
	}

	return &lb.QueryInstalledChaincodeResult{
		Label:     chaincode.Label,
		PackageId: chaincode.PackageID.String(),
	}, nil
}

// QueryInstalledChaincodes is a SCC function that may be dispatched to which
// routes to the underlying lifecycle implementation.
func (i *Invocation) QueryInstalledChaincodes(input *lb.QueryInstalledChaincodesArgs) (proto.Message, error) {

	logger.Debugf("received invocation of QueryInstalledChaincodes")

	chaincodes, err := i.SCC.Functions.QueryInstalledChaincodes()
	if err != nil {
		return nil, err
	}

	result := &lb.QueryInstalledChaincodesResult{}
	for _, chaincode := range chaincodes {
		result.InstalledChaincodes = append(
			result.InstalledChaincodes,
			&lb.QueryInstalledChaincodesResult_InstalledChaincode{
				Label:     chaincode.Label,
				PackageId: chaincode.PackageID.String(),
			})
	}
	return result, nil
}

// ApproveChaincodeDefinitionForMyOrg is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) ApproveChaincodeDefinitionForMyOrg(input *lb.ApproveChaincodeDefinitionForMyOrgArgs) (proto.Message, error) {
	if err := i.validateInput(input.Name, input.Version, input.Collections); err != nil {
		return nil, err
	}
	collectionName := ImplicitCollectionNameForOrg(i.SCC.OrgMSPID)
	var collectionConfig []*cb.CollectionConfig
	if input.Collections != nil {
		collectionConfig = input.Collections.Config
	}

	var packageID persistenceintf.PackageID
	if input.Source != nil {
		switch source := input.Source.Type.(type) {
		case *lb.ChaincodeSource_LocalPackage:
			packageID = persistenceintf.PackageID(source.LocalPackage.PackageId)
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
		Collections: &cb.CollectionConfigPackage{
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

// QueryApprovalStatus is a SCC function that may be dispatched to the underlying
// lifecycle implementation
func (i *Invocation) QueryApprovalStatus(input *lb.QueryApprovalStatusArgs) (proto.Message, error) {
	if i.ApplicationConfig == nil {
		return nil, errors.Errorf("no application config for channel '%s'", i.Stub.GetChannelID())
	}

	orgs := i.ApplicationConfig.Organizations()
	opaqueStates := make([]OpaqueState, 0, len(orgs))
	orgNames := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgNames = append(orgNames, org.MSPID())
		opaqueStates = append(opaqueStates, &ChaincodePrivateLedgerShim{
			Collection: ImplicitCollectionNameForOrg(org.MSPID()),
			Stub:       i.Stub,
		})
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

	logger.Debugf("received invocation of QueryApprovalStatus on channel '%s' for definition '%s'",
		i.Stub.GetChannelID(),
		cd,
	)

	approved, err := i.SCC.Functions.QueryApprovalStatus(
		i.Stub.GetChannelID(),
		input.Name,
		cd,
		i.Stub,
		opaqueStates,
	)
	if err != nil {
		return nil, err
	}

	orgApproval := make(map[string]bool)
	for i, org := range orgNames {
		orgApproval[org] = approved[i]
	}

	return &lb.QueryApprovalStatusResults{
		Approved: orgApproval,
	}, nil
}

// CommitChaincodeDefinition is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) CommitChaincodeDefinition(input *lb.CommitChaincodeDefinitionArgs) (proto.Message, error) {
	if err := i.validateInput(input.Name, input.Version, input.Collections); err != nil {
		return nil, err
	}

	if i.ApplicationConfig == nil {
		return nil, errors.Errorf("no application config for channel '%s'", i.Stub.GetChannelID())
	}

	orgs := i.ApplicationConfig.Organizations()
	opaqueStates := make([]OpaqueState, 0, len(orgs))
	myOrgIndex := -1
	for _, org := range orgs {
		opaqueStates = append(opaqueStates, &ChaincodePrivateLedgerShim{
			Collection: ImplicitCollectionNameForOrg(org.MSPID()),
			Stub:       i.Stub,
		})
		if org.MSPID() == i.SCC.OrgMSPID {
			myOrgIndex = len(opaqueStates) - 1
		}
	}

	if myOrgIndex == -1 {
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

	agreement, err := i.SCC.Functions.CommitChaincodeDefinition(
		i.Stub.GetChannelID(),
		input.Name,
		cd,
		i.Stub,
		opaqueStates,
	)

	if err != nil {
		return nil, err
	}

	if !agreement[myOrgIndex] {
		return nil, errors.Errorf("chaincode definition not agreed to by this org (%s)", i.SCC.OrgMSPID)
	}

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

	return &lb.QueryChaincodeDefinitionResult{
		Sequence:            definedChaincode.Sequence,
		Version:             definedChaincode.EndorsementInfo.Version,
		EndorsementPlugin:   definedChaincode.EndorsementInfo.EndorsementPlugin,
		ValidationPlugin:    definedChaincode.ValidationInfo.ValidationPlugin,
		ValidationParameter: definedChaincode.ValidationInfo.ValidationParameter,
		InitRequired:        definedChaincode.EndorsementInfo.InitRequired,
		Collections:         definedChaincode.Collections,
	}, nil
}

// QueryNamespaceDefinitions is a SCC function that may be dispatched
// to which routes to the underlying lifecycle implementation.
func (i *Invocation) QueryNamespaceDefinitions(input *lb.QueryNamespaceDefinitionsArgs) (proto.Message, error) {

	logger.Debugf("received invocation of QueryNamespaceDefinitions on channel '%s'",
		i.Stub.GetChannelID(),
	)

	namespaces, err := i.SCC.Functions.QueryNamespaceDefinitions(&ChaincodePublicLedgerShim{ChaincodeStubInterface: i.Stub})
	if err != nil {
		return nil, err
	}
	result := map[string]*lb.QueryNamespaceDefinitionsResult_Namespace{}
	for namespace, nType := range namespaces {
		result[namespace] = &lb.QueryNamespaceDefinitionsResult_Namespace{
			Type: nType,
		}
	}
	return &lb.QueryNamespaceDefinitionsResult{
		Namespaces: result,
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

func (i *Invocation) validateInput(name, version string, collections *cb.CollectionConfigPackage) error {
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
	channelConfig := i.SCC.ChannelConfigSource.GetStableChannelConfig(i.ChannelID)
	if channelConfig == nil {
		return errors.Errorf("could not get channelconfig for channel '%s'", i.ChannelID)
	}
	mspMgr := channelConfig.MSPManager()
	if mspMgr == nil {
		return errors.Errorf(fmt.Sprintf("could not get MSP manager for channel '%s'", i.ChannelID))
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

func extractStaticCollectionConfigs(collConfigPkg *common.CollectionConfigPackage) ([]*common.StaticCollectionConfig, error) {
	if collConfigPkg == nil || len(collConfigPkg.Config) == 0 {
		return nil, nil
	}
	collConfigs := make([]*common.StaticCollectionConfig, len(collConfigPkg.Config))
	for i, c := range collConfigPkg.Config {
		switch t := c.Payload.(type) {
		case *cb.CollectionConfig_StaticCollectionConfig:
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

func validateCollectionConfigs(collConfigs []*common.StaticCollectionConfig, mspMgr msp.MSPManager) error {
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
			return fmt.Errorf("collection-name: %s -- found duplicate in collection configuration",
				c.Name)
		}
		collNamesMap[c.Name] = struct{}{}
		// Validate gossip related parameters present in the collection config
		if c.MaximumPeerCount < c.RequiredPeerCount {
			return fmt.Errorf("collection-name: %s -- maximum peer count (%d) cannot be greater than the required peer count (%d)",
				c.Name, c.MaximumPeerCount, c.RequiredPeerCount)
		}
		if c.RequiredPeerCount < 0 {
			return fmt.Errorf("collection-name: %s -- requiredPeerCount (%d) cannot be less than zero",
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
func validateCollectionConfigMemberOrgsPolicy(coll *common.StaticCollectionConfig, mspMgr msp.MSPManager) error {
	if coll.MemberOrgsPolicy == nil {
		return fmt.Errorf("collection member policy is not set for collection '%s'", coll.Name)
	}
	if coll.MemberOrgsPolicy.GetSignaturePolicy() == nil {
		return fmt.Errorf("collection member org policy is empty for collection '%s'", coll.Name)
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
		return errors.New(fmt.Sprintf("signature policy is not an OR concatenation, NOutOf %d", sp.GetNOutOf().N))
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
	proposedCollConfs []*common.StaticCollectionConfig,
	committedCollConfPkg *common.CollectionConfigPackage,
) error {
	if committedCollConfPkg == nil || len(committedCollConfPkg.Config) == 0 {
		return nil
	}

	if len(proposedCollConfs) == 0 {
		return errors.Errorf("the proposed collection config does not contain previously defined collections")
	}

	proposedCollsMap := map[string]*common.StaticCollectionConfig{}
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
