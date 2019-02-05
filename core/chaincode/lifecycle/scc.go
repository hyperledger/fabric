/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/dispatcher"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	//InstalledChaincodeFuncName is the chaincode function name used to install a chaincode
	InstallChaincodeFuncName = "InstallChaincode"

	// QueryInstalledChaincodeFuncName is the chaincode function name used to query an installed chaincode
	QueryInstalledChaincodeFuncName = "QueryInstalledChaincode"

	// QueryInstalledChaincodesFuncName is the chaincode function name used to query all installed chaincodes
	QueryInstalledChaincodesFuncName = "QueryInstalledChaincodes"

	// DefineForMyOrgFuncName is the chaincode function name used to approve a chaincode definition for
	// execution by the user's own org
	DefineChaincodeForMyOrgFuncName = "DefineChaincodeForMyOrg"

	// DefineChaincodeFuncName is the chaincode function name used to 'define' (previously 'instantiate')
	// a chaincode in a channel.
	DefineChaincodeFuncName = "DefineChaincode"

	// QueryDefinedChaincodeFuncName is the chaincode function name used to 'define' (previously 'instantiate')
	// a chaincode in a channel.
	QueryDefinedChaincodeFuncName = "QueryDefinedChaincode"

	// QueryDefinedNamespaces is the chaincode function name used query which namespaces are currently defined
	// and what type those namespaces are.
	QueryDefinedNamespacesFuncName = "QueryDefinedNamespaces"
)

// SCCFunctions provides a backing implementation with concrete arguments
// for each of the SCC functions
type SCCFunctions interface {
	// InstallChaincode persists a chaincode definition to disk
	InstallChaincode(name, version string, chaincodePackage []byte) (hash []byte, err error)

	// QueryInstalledChaincode returns the hash for a given name and version of an installed chaincode
	QueryInstalledChaincode(name, version string) (hash []byte, err error)

	// QueryInstalledChaincodes returns the currently installed chaincodes
	QueryInstalledChaincodes() (chaincodes []chaincode.InstalledChaincode, err error)

	// DefineChaincodeForOrg records a chaincode definition into this org's implicit collection.
	DefineChaincodeForOrg(cd *ChaincodeDefinition, publicState ReadableState, orgState ReadWritableState) error

	// DefineChaincode records a new chaincode definition into the public state and returns the orgs which agreed with that definition.
	DefineChaincode(cd *ChaincodeDefinition, publicState ReadWritableState, orgStates []OpaqueState) ([]bool, error)

	// QueryDefinedChaincode reads a chaincode definition from the public state.
	QueryDefinedChaincode(name string, publicState ReadableState) (*DefinedChaincode, error)

	// QueryDefinedNamespaces returns all defined namespaces
	QueryDefinedNamespaces(publicState RangeableState) (map[string]string, error)
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

	ChannelConfigSource ChannelConfigSource

	// Functions provides the backing implementation of lifecycle.
	Functions SCCFunctions

	// Dispatcher handles the rote protobuf boilerplate for unmarshaling/marshaling
	// the inputs and outputs of the SCC functions.
	Dispatcher *dispatcher.Dispatcher
}

// Name returns "+lifecycle"
func (scc *SCC) Name() string {
	return "+lifecycle"
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

	// TODO add ACLs

	outputBytes, err := scc.Dispatcher.Dispatch(
		args[1],
		string(args[0]),
		&Invocation{
			SCC:  scc,
			Stub: stub,
		},
	)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to invoke backing implementation of '%s': %s", string(args[0]), err.Error()))
	}

	return shim.Success(outputBytes)
}

type Invocation struct {
	Stub shim.ChaincodeStubInterface
	SCC  *SCC
}

// InstallChaincode is a SCC function that may be dispatched to which routes to the underlying
// lifecycle implementation.
func (i *Invocation) InstallChaincode(input *lb.InstallChaincodeArgs) (proto.Message, error) {
	hash, err := i.SCC.Functions.InstallChaincode(input.Name, input.Version, input.ChaincodeInstallPackage)
	if err != nil {
		return nil, err
	}

	return &lb.InstallChaincodeResult{
		Hash: hash,
	}, nil
}

// QueryInstalledChaincode is a SCC function that may be dispatched to which routes to the underlying
// lifecycle implementation.
func (i *Invocation) QueryInstalledChaincode(input *lb.QueryInstalledChaincodeArgs) (proto.Message, error) {
	hash, err := i.SCC.Functions.QueryInstalledChaincode(input.Name, input.Version)
	if err != nil {
		return nil, err
	}

	return &lb.QueryInstalledChaincodeResult{
		Hash: hash,
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
				Name:    chaincode.Name,
				Version: chaincode.Version,
				Hash:    chaincode.Id,
			})
	}
	return result, nil
}

// DefineChaincodeForMyOrg is a SCC function that may be dispatched to which routes to the underlying
// lifecycle implementation
func (i *Invocation) DefineChaincodeForMyOrg(input *lb.DefineChaincodeForMyOrgArgs) (proto.Message, error) {
	collectionName := fmt.Sprintf("_implicit_org_%s", i.SCC.OrgMSPID)
	if err := i.SCC.Functions.DefineChaincodeForOrg(
		&ChaincodeDefinition{
			Name:     input.Name,
			Sequence: input.Sequence,
			Parameters: &ChaincodeParameters{
				Hash:                input.Hash,
				Version:             input.Version,
				EndorsementPlugin:   input.EndorsementPlugin,
				ValidationPlugin:    input.ValidationPlugin,
				ValidationParameter: input.ValidationParameter,
				Collections:         input.Collections,
			},
		},
		i.Stub,
		&ChaincodePrivateLedgerShim{
			Collection: collectionName,
			Stub:       i.Stub,
		},
	); err != nil {
		return nil, err
	}
	return &lb.DefineChaincodeForMyOrgResult{}, nil
}

func (i *Invocation) DefineChaincode(input *lb.DefineChaincodeArgs) (proto.Message, error) {
	channelConfig := i.SCC.ChannelConfigSource.GetStableChannelConfig(i.Stub.GetChannelID())
	if channelConfig == nil {
		return nil, errors.Errorf("could not get channelconfig for channel %s", i.Stub.GetChannelID())
	}
	ac, ok := channelConfig.ApplicationConfig()
	if !ok {
		return nil, errors.Errorf("could not get application config for channel %s", i.Stub.GetChannelID())
	}

	orgs := ac.Organizations()
	opaqueStates := make([]OpaqueState, 0, len(orgs))
	myOrgIndex := -1
	for _, org := range orgs {
		opaqueStates = append(opaqueStates, &ChaincodePrivateLedgerShim{
			Collection: fmt.Sprintf("_implicit_org_%s", org.MSPID()),
			Stub:       i.Stub,
		})
		if org.MSPID() == i.SCC.OrgMSPID {
			myOrgIndex = len(opaqueStates) - 1
		}
	}

	if myOrgIndex == -1 {
		return nil, errors.Errorf("impossibly, this peer's org is processing requests for a channel it is not a member of")
	}

	agreement, err := i.SCC.Functions.DefineChaincode(
		&ChaincodeDefinition{
			Name:     input.Name,
			Sequence: input.Sequence,
			Parameters: &ChaincodeParameters{
				Hash:                input.Hash,
				Version:             input.Version,
				EndorsementPlugin:   input.EndorsementPlugin,
				ValidationPlugin:    input.ValidationPlugin,
				ValidationParameter: input.ValidationParameter,
				Collections:         input.Collections,
			},
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

	return &lb.DefineChaincodeResult{}, nil
}

func (i *Invocation) QueryDefinedChaincode(input *lb.QueryDefinedChaincodeArgs) (proto.Message, error) {
	definedChaincode, err := i.SCC.Functions.QueryDefinedChaincode(input.Name, i.Stub)
	if err != nil {
		return nil, err
	}

	return &lb.QueryDefinedChaincodeResult{
		Sequence:            definedChaincode.Sequence,
		Version:             definedChaincode.Version,
		EndorsementPlugin:   definedChaincode.EndorsementPlugin,
		ValidationPlugin:    definedChaincode.ValidationPlugin,
		ValidationParameter: definedChaincode.ValidationParameter,
		Hash:                definedChaincode.Hash,
		Collections:         definedChaincode.Collections,
	}, nil
}

func (i *Invocation) QueryDefinedNamespaces(input *lb.QueryDefinedNamespacesArgs) (proto.Message, error) {
	namespaces, err := i.SCC.Functions.QueryDefinedNamespaces(&ChaincodePublicLedgerShim{ChaincodeStubInterface: i.Stub})
	if err != nil {
		return nil, err
	}
	result := map[string]*lb.QueryDefinedNamespacesResult_Namespace{}
	for namespace, nType := range namespaces {
		result[namespace] = &lb.QueryDefinedNamespacesResult_Namespace{
			Type: nType,
		}
	}
	return &lb.QueryDefinedNamespacesResult{
		Namespaces: result,
	}, nil
}
