/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"regexp"

	"github.com/hyperledger/fabric/common/cauthdsl"
	validationState "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/protos/msp"
	pb "github.com/hyperledger/fabric/protos/peer"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/pkg/errors"
)

const (
	LifecycleEndorsementPolicyRef = "/Channel/Application/LifecycleEndorsement"
)

var (
	// This is a channel which was created with a lifecycle endorsement policy
	LifecycleDefaultEndorsementPolicyBytes = protoutil.MarshalOrPanic(&pb.ApplicationPolicy{
		Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
			ChannelConfigPolicyReference: LifecycleEndorsementPolicyRef,
		},
	})
)

// Namespaces returns the list of namespaces which are relevant to chaincode lifecycle
func (l *Lifecycle) Namespaces() []string {
	return append([]string{LifecycleNamespace}, l.LegacyDeployedCCInfoProvider.Namespaces()...)
}

var SequenceMatcher = regexp.MustCompile("^" + NamespacesName + "/fields/([^/]+)/Sequence$")

// UpdatedChaincodes returns the chaincodes that are getting updated by the supplied 'stateUpdates'
func (l *Lifecycle) UpdatedChaincodes(stateUpdates map[string][]*kvrwset.KVWrite) ([]*ledger.ChaincodeLifecycleInfo, error) {
	lifecycleInfo := []*ledger.ChaincodeLifecycleInfo{}

	// If the lifecycle table was updated, report only modified chaincodes
	lifecycleUpdates := stateUpdates[LifecycleNamespace]

	for _, kvWrite := range lifecycleUpdates {
		matches := SequenceMatcher.FindStringSubmatch(kvWrite.Key)
		if len(matches) != 2 {
			continue
		}
		// XXX Note, this may not be a chaincode namespace, handle this later
		lifecycleInfo = append(lifecycleInfo, &ledger.ChaincodeLifecycleInfo{Name: matches[1]})
	}

	legacyUpdates, err := l.LegacyDeployedCCInfoProvider.UpdatedChaincodes(stateUpdates)
	if err != nil {
		return nil, errors.WithMessage(err, "error invoking legacy deployed cc info provider")
	}

	return append(lifecycleInfo, legacyUpdates...), nil
}

// ChaincodeDefinitionIfDefined returns whether the chaincode name is defined in the new lifecycle, a shim around
// the SimpleQueryExecutor to work with the serializer, or an error.  If the namespace is defined, but it is
// not a chaincode, this is considered an error.
func (l *Lifecycle) ChaincodeDefinitionIfDefined(chaincodeName string, state ReadableState) (bool, *ChaincodeDefinition, error) {
	if chaincodeName == LifecycleNamespace {
		return true, &ChaincodeDefinition{
			EndorsementInfo: &lb.ChaincodeEndorsementInfo{
				InitRequired: false,
			},
			ValidationInfo: &lb.ChaincodeValidationInfo{},
		}, nil
	}

	metadata, ok, err := l.Serializer.DeserializeMetadata(NamespacesName, chaincodeName, state)
	if err != nil {
		return false, nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize metadata for chaincode %s", chaincodeName))
	}

	if !ok {
		return false, nil, nil
	}

	if metadata.Datatype != ChaincodeDefinitionType {
		return false, nil, errors.Errorf("not a chaincode type: %s", metadata.Datatype)
	}

	definedChaincode := &ChaincodeDefinition{}
	err = l.Serializer.Deserialize(NamespacesName, chaincodeName, metadata, definedChaincode, state)
	if err != nil {
		return false, nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize chaincode definition for chaincode %s", chaincodeName))
	}

	return true, definedChaincode, nil
}

func (l *Lifecycle) ChaincodeInfo(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
	exists, definedChaincode, err := l.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not get info about chaincode")
	}

	if !exists {
		return l.LegacyDeployedCCInfoProvider.ChaincodeInfo(channelName, chaincodeName, qe)
	}

	ic, err := l.ChaincodeImplicitCollections(channelName)
	if err != nil {
		return nil, errors.WithMessage(err, "could not create implicit collections for channel")
	}

	return &ledger.DeployedChaincodeInfo{
		Name:                        chaincodeName,
		Version:                     definedChaincode.EndorsementInfo.Version,
		Hash:                        definedChaincode.EndorsementInfo.Id,
		ExplicitCollectionConfigPkg: definedChaincode.Collections,
		ImplicitCollections:         ic,
	}, nil
}

var ImplicitCollectionMatcher = regexp.MustCompile("^" + ImplicitCollectionNameForOrg("(.+)") + "$")

// CollectionInfo implements function in interface ledger.DeployedChaincodeInfoProvider, it returns config for
// both static and implicit collections.
func (l *Lifecycle) CollectionInfo(channelName, chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*cb.StaticCollectionConfig, error) {
	exists, definedChaincode, err := l.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not get chaincode")
	}

	if !exists {
		return l.LegacyDeployedCCInfoProvider.CollectionInfo(channelName, chaincodeName, collectionName, qe)
	}

	matches := ImplicitCollectionMatcher.FindStringSubmatch(collectionName)
	if len(matches) == 2 {
		return GenerateImplicitCollectionForOrg(matches[1]), nil
	}

	if definedChaincode.Collections != nil {
		for _, conf := range definedChaincode.Collections.Config {
			staticCollConfig := conf.GetStaticCollectionConfig()
			if staticCollConfig != nil && staticCollConfig.Name == collectionName {
				return staticCollConfig, nil
			}
		}
	}
	return nil, nil
}

// ImplicitCollections implements function in interface ledger.DeployedChaincodeInfoProvider.  It returns
//a slice that contains one proto msg for each of the implicit collections
func (l *Lifecycle) ImplicitCollections(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) ([]*cb.StaticCollectionConfig, error) {
	exists, _, err := l.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not get info about chaincode")
	}

	if !exists {
		// Implicit collections are a v2.0 lifecycle concept, if the chaincode is not in the new lifecycle, return nothing
		return nil, nil
	}

	return l.ChaincodeImplicitCollections(channelName)
}

// ChaincodeImplicitCollections assumes the chaincode exists in the new lifecycle and returns the implicit collections
func (l *Lifecycle) ChaincodeImplicitCollections(channelName string) ([]*cb.StaticCollectionConfig, error) {
	channelConfig := l.ChannelConfigSource.GetStableChannelConfig(channelName)
	if channelConfig == nil {
		return nil, errors.Errorf("could not get channelconfig for channel %s", channelName)
	}
	ac, ok := channelConfig.ApplicationConfig()
	if !ok {
		return nil, errors.Errorf("could not get application config for channel %s", channelName)
	}

	orgs := ac.Organizations()
	implicitCollections := make([]*cb.StaticCollectionConfig, 0, len(orgs))
	for _, org := range orgs {
		implicitCollections = append(implicitCollections, GenerateImplicitCollectionForOrg(org.MSPID()))
	}

	return implicitCollections, nil
}

func GenerateImplicitCollectionForOrg(mspid string) *cb.StaticCollectionConfig {
	return &cb.StaticCollectionConfig{
		Name: ImplicitCollectionNameForOrg(mspid),
		MemberOrgsPolicy: &cb.CollectionPolicyConfig{
			Payload: &cb.CollectionPolicyConfig_SignaturePolicy{
				SignaturePolicy: cauthdsl.SignedByMspMember(mspid),
			},
		},
	}
}

func ImplicitCollectionNameForOrg(mspid string) string {
	return fmt.Sprintf("_implicit_org_%s", mspid)
}

func (l *Lifecycle) ImplicitCollectionEndorsementPolicyAsBytes(channelID, orgMSPID string) (policy []byte, unexpectedErr, validationErr error) {
	channelConfig := l.ChannelConfigSource.GetStableChannelConfig(channelID)
	if channelConfig == nil {
		return nil, errors.Errorf("could not get channel config for channel '%s'", channelID), nil
	}

	ac, ok := channelConfig.ApplicationConfig()
	if !ok {
		return nil, errors.Errorf("could not get application config for channel '%s'", channelID), nil
	}

	matchedOrgName := ""
	for orgName, org := range ac.Organizations() {
		if org.MSPID() == orgMSPID {
			matchedOrgName = orgName
			break
		}
	}

	if matchedOrgName == "" {
		return nil, nil, errors.Errorf("no org found in channel with MSPID '%s'", orgMSPID)
	}

	policyName := fmt.Sprintf("/Channel/Application/%s/Endorsement", matchedOrgName)
	if _, ok := channelConfig.PolicyManager().GetPolicy(policyName); ok {
		return protoutil.MarshalOrPanic(&pb.ApplicationPolicy{
			Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
				ChannelConfigPolicyReference: policyName,
			},
		}), nil, nil
	}

	// This was a channel which was upgraded or did not define an org level endorsement policy, use a default
	// of "any member of the org"

	return protoutil.MarshalOrPanic(&pb.ApplicationPolicy{
		Type: &pb.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: cauthdsl.SignedByAnyMember([]string{orgMSPID}),
		},
	}), nil, nil
}

func (l *Lifecycle) LifecycleEndorsementPolicyAsBytes(channelID string) ([]byte, error) {
	channelConfig := l.ChannelConfigSource.GetStableChannelConfig(channelID)
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

	return protoutil.MarshalOrPanic(&pb.ApplicationPolicy{
		Type: &pb.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: cauthdsl.SignedByNOutOfGivenRole(int32(len(mspids)/2+1), msp.MSPRole_MEMBER, mspids),
		},
	}), nil
}

// ValidationInfo returns the name and arguments of the validation plugin for the supplied
// chaincode. The function returns two types of errors, unexpected errors and validation
// errors. The reason for this is that this function is called from the validation code,
// which needs to differentiate the two types of error to halt processing on the channel
// if the unexpected error is not nil and mark the transaction as invalid if the validation
// error is not nil.
func (l *Lifecycle) ValidationInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (plugin string, args []byte, unexpectedErr error, validationErr error) {
	// TODO, this is a bit of an overkill check, and will need to be scaled back for non-chaincode type namespaces
	exists, definedChaincode, err := l.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	})
	if err != nil {
		return "", nil, errors.WithMessage(err, "could not get chaincode"), nil
	}

	if !exists {
		// TODO, this is inconsistent with how the legacy lifecycle reports
		// that a missing chaincode is a validation error.  But, for now
		// this is required to make the passthrough work.
		return "", nil, nil, nil
	}

	if chaincodeName == LifecycleNamespace {
		b, err := l.LifecycleEndorsementPolicyAsBytes(channelID)
		if err != nil {
			return "", nil, errors.WithMessage(err, "unexpected failure to create lifecycle endorsement policy"), nil
		}
		return "vscc", b, nil, nil
	}

	return definedChaincode.ValidationInfo.ValidationPlugin, definedChaincode.ValidationInfo.ValidationParameter, nil, nil
}

// CollectionValidationInfo returns information about collections to the validation component
func (l *Lifecycle) CollectionValidationInfo(channelID, chaincodeName, collectionName string, state validationState.State) (args []byte, unexpectedErr, validationErr error) {
	exists, definedChaincode, err := l.ChaincodeDefinitionIfDefined(chaincodeName, &ValidatorStateShim{
		Namespace:      LifecycleNamespace,
		ValidatorState: state,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not get chaincode"), nil
	}

	if !exists {
		// TODO, this is inconsistent with how the legacy lifecycle reports
		// that a missing chaincode is a validation error.  But, for now
		// this is required to make the passthrough work.
		return nil, nil, nil
	}

	matches := ImplicitCollectionMatcher.FindStringSubmatch(collectionName)
	if len(matches) == 2 {
		return l.ImplicitCollectionEndorsementPolicyAsBytes(channelID, matches[1])
	}

	if definedChaincode.Collections != nil {
		for _, conf := range definedChaincode.Collections.Config {
			staticCollConfig := conf.GetStaticCollectionConfig()
			if staticCollConfig != nil && staticCollConfig.Name == collectionName {
				// TODO, return the collection EP if defined
				return definedChaincode.ValidationInfo.ValidationParameter, nil, nil
			}
		}
	}

	return nil, nil, errors.Errorf("no such collection '%s'", collectionName)
}
