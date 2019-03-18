/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/dispatcher"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	"github.com/pkg/errors"
)

const (
	// LifecycleNamespace is the namespace in the statedb where lifecycle information is stored
	LifecycleNamespace = "_lifecycle"

	//InstalledChaincodeFuncName is the chaincode function name used to install a chaincode
	InstallChaincodeFuncName = "InstallChaincode"

	// QueryInstalledChaincodeFuncName is the chaincode function name used to query an installed chaincode
	QueryInstalledChaincodeFuncName = "QueryInstalledChaincode"

	// QueryInstalledChaincodesFuncName is the chaincode function name used to query all installed chaincodes
	QueryInstalledChaincodesFuncName = "QueryInstalledChaincodes"

	// DefineForMyOrgFuncName is the chaincode function name used to approve a chaincode definition for
	// execution by the user's own org
	ApproveChaincodeDefinitionForMyOrgFuncName = "ApproveChaincodeDefinitionForMyOrg"

	// CommitChaincodeDefinitionFuncName is the chaincode function name used to 'define' (previously 'instantiate')
	// a chaincode in a channel.
	CommitChaincodeDefinitionFuncName = "CommitChaincodeDefinition"

	// QueryChaincodeDefinitionFuncName is the chaincode function name used to 'define' (previously 'instantiate')
	// a chaincode in a channel.
	QueryChaincodeDefinitionFuncName = "QueryChaincodeDefinition"

	// QueryNamespaceDefinitions is the chaincode function name used query which namespaces are currently defined
	// and what type those namespaces are.
	QueryNamespaceDefinitionsFuncName = "QueryNamespaceDefinitions"
)

// SCCFunctions provides a backing implementation with concrete arguments
// for each of the SCC functions
type SCCFunctions interface {
	// InstallChaincode persists a chaincode definition to disk
	InstallChaincode([]byte) (*chaincode.InstalledChaincode, error)

	// QueryInstalledChaincode returns the hash for a given name and version of an installed chaincode
	QueryInstalledChaincode(packageID persistence.PackageID) (*chaincode.InstalledChaincode, error)

	// QueryInstalledChaincodes returns the currently installed chaincodes
	QueryInstalledChaincodes() (chaincodes []chaincode.InstalledChaincode, err error)

	// ApproveChaincodeDefinitionForOrg records a chaincode definition into this org's implicit collection.
	ApproveChaincodeDefinitionForOrg(chname, ccname string, cd *ChaincodeDefinition, packageID persistence.PackageID, publicState ReadableState, orgState ReadWritableState) error

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

// SCC implements the required methods to satisfy the chaincode interface.
// It routes the invocation calls to the backing implementations.
type SCC struct {
	OrgMSPID string

	ACLProvider aclmgmt.ACLProvider

	ChannelConfigSource ChannelConfigSource

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
	if channelID := stub.GetChannelID(); channelID != "" {
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
			ApplicationConfig: ac,
			SCC:               scc,
			Stub:              stub,
		},
	)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to invoke backing implementation of '%s': %s", string(args[0]), err.Error()))
	}

	return shim.Success(outputBytes)
}

type Invocation struct {
	ApplicationConfig channelconfig.Application // Note this may be nil
	Stub              shim.ChaincodeStubInterface
	SCC               *SCC
}

// InstallChaincode is a SCC function that may be dispatched to which routes to the underlying
// lifecycle implementation.
func (i *Invocation) InstallChaincode(input *lb.InstallChaincodeArgs) (proto.Message, error) {
	installedCC, err := i.SCC.Functions.InstallChaincode(input.ChaincodeInstallPackage)
	if err != nil {
		return nil, err
	}

	return &lb.InstallChaincodeResult{
		Label:     installedCC.Label,
		PackageId: installedCC.PackageID.String(),
	}, nil
}

// QueryInstalledChaincode is a SCC function that may be dispatched to which routes to the underlying
// lifecycle implementation.
func (i *Invocation) QueryInstalledChaincode(input *lb.QueryInstalledChaincodeArgs) (proto.Message, error) {
	chaincode, err := i.SCC.Functions.QueryInstalledChaincode(persistence.PackageID(input.PackageId))
	if err != nil {
		return nil, err
	}

	return &lb.QueryInstalledChaincodeResult{
		Label:     chaincode.Label,
		PackageId: chaincode.PackageID.String(),
	}, nil
}

// QueryInstalledChaincodes is a SCC function that may be dispatch to which routes to the underlying
// lifecycle implementation.
func (i *Invocation) QueryInstalledChaincodes(input *lb.QueryInstalledChaincodesArgs) (proto.Message, error) {
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

// ApproveChaincodeDefinitionForMyOrg is a SCC function that may be dispatched to which routes to the underlying
// lifecycle implementation
func (i *Invocation) ApproveChaincodeDefinitionForMyOrg(input *lb.ApproveChaincodeDefinitionForMyOrgArgs) (proto.Message, error) {
	collectionName := ImplicitCollectionNameForOrg(i.SCC.OrgMSPID)
	var collectionConfig []*cb.CollectionConfig
	if input.Collections != nil {
		collectionConfig = input.Collections.Config
	}

	var packageID persistence.PackageID
	if input.Source != nil {
		switch source := input.Source.Type.(type) {
		case *lb.ChaincodeSource_LocalPackage:
			packageID = persistence.PackageID(source.LocalPackage.PackageId)
		case *lb.ChaincodeSource_Unavailable:
		default:
		}
	}

	if err := i.SCC.Functions.ApproveChaincodeDefinitionForOrg(
		i.Stub.GetChannelID(),
		input.Name,
		&ChaincodeDefinition{
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
		},
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

func (i *Invocation) CommitChaincodeDefinition(input *lb.CommitChaincodeDefinitionArgs) (proto.Message, error) {
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

	agreement, err := i.SCC.Functions.CommitChaincodeDefinition(
		i.Stub.GetChannelID(),
		input.Name,
		&ChaincodeDefinition{
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
		},
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

func (i *Invocation) QueryChaincodeDefinition(input *lb.QueryChaincodeDefinitionArgs) (proto.Message, error) {
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

func (i *Invocation) QueryNamespaceDefinitions(input *lb.QueryNamespaceDefinitionsArgs) (proto.Message, error) {
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
