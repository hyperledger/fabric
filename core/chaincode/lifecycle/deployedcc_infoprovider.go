/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"regexp"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	validationState "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/gossip/privdata"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mock/legacy_ccinfo.go --fake-name LegacyDeployedCCInfoProvider . LegacyDeployedCCInfoProvider
type LegacyDeployedCCInfoProvider interface {
	ledger.DeployedChaincodeInfoProvider
}

const (
	LifecycleEndorsementPolicyRef = "/Channel/Application/LifecycleEndorsement"
)

// This is a channel which was created with a lifecycle endorsement policy
var LifecycleDefaultEndorsementPolicyBytes = protoutil.MarshalOrPanic(&cb.ApplicationPolicy{
	Type: &cb.ApplicationPolicy_ChannelConfigPolicyReference{
		ChannelConfigPolicyReference: LifecycleEndorsementPolicyRef,
	},
})

type ValidatorCommitter struct {
	CoreConfig                   *peer.Config
	PrivdataConfig               *privdata.PrivdataConfig
	Resources                    *Resources
	LegacyDeployedCCInfoProvider LegacyDeployedCCInfoProvider
}

// Namespaces returns the list of namespaces which are relevant to chaincode lifecycle
func (vc *ValidatorCommitter) Namespaces() []string {
	return append([]string{LifecycleNamespace}, vc.LegacyDeployedCCInfoProvider.Namespaces()...)
}

var SequenceMatcher = regexp.MustCompile("^" + NamespacesName + "/fields/([^/]+)/Sequence$")

// UpdatedChaincodes returns the chaincodes that are getting updated by the supplied 'stateUpdates'
func (vc *ValidatorCommitter) UpdatedChaincodes(stateUpdates map[string][]*kvrwset.KVWrite) ([]*ledger.ChaincodeLifecycleInfo, error) {
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

	legacyUpdates, err := vc.LegacyDeployedCCInfoProvider.UpdatedChaincodes(stateUpdates)
	if err != nil {
		return nil, errors.WithMessage(err, "error invoking legacy deployed cc info provider")
	}

	return append(lifecycleInfo, legacyUpdates...), nil
}

func (vc *ValidatorCommitter) ChaincodeInfo(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
	exists, definedChaincode, err := vc.Resources.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not get info about chaincode")
	}

	if !exists {
		return vc.LegacyDeployedCCInfoProvider.ChaincodeInfo(channelName, chaincodeName, qe)
	}

	return &ledger.DeployedChaincodeInfo{
		Name:                        chaincodeName,
		Version:                     definedChaincode.EndorsementInfo.Version,
		Hash:                        util.ComputeSHA256([]byte(chaincodeName + ":" + definedChaincode.EndorsementInfo.Version)),
		ExplicitCollectionConfigPkg: definedChaincode.Collections,
		IsLegacy:                    false,
	}, nil
}

// AllChaincodesInfo returns the mapping of chaincode name to DeployedChaincodeInfo for all the deployed chaincodes
func (vc *ValidatorCommitter) AllChaincodesInfo(channelName string, sqe ledger.SimpleQueryExecutor) (map[string]*ledger.DeployedChaincodeInfo, error) {
	sqes := &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: sqe,
	}
	ccQuery := &ExternalFunctions{Resources: vc.Resources}
	namespaceDefs, err := ccQuery.QueryNamespaceDefinitions(sqes)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*ledger.DeployedChaincodeInfo)
	for ccName, value := range namespaceDefs {
		if value != FriendlyChaincodeDefinitionType {
			continue
		}
		deployedccInfo, err := vc.ChaincodeInfo(channelName, ccName, sqe)
		if err != nil {
			return nil, err
		}
		result[ccName] = deployedccInfo
	}

	legacyCCs, err := vc.LegacyDeployedCCInfoProvider.AllChaincodesInfo(channelName, sqe)
	if err != nil {
		return nil, err
	}

	for ccName, deployedccInfo := range legacyCCs {
		if _, ok := result[ccName]; !ok {
			result[ccName] = deployedccInfo
		}
	}

	return result, nil
}

// AllCollectionsConfigPkg implements function in interface ledger.DeployedChaincodeInfoProvider
// this implementation returns a combined collection config pkg that contains both explicit and implicit collections
func (vc *ValidatorCommitter) AllCollectionsConfigPkg(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*pb.CollectionConfigPackage, error) {
	chaincodeInfo, err := vc.ChaincodeInfo(channelName, chaincodeName, qe)
	if err != nil {
		return nil, err
	}
	explicitCollectionConfigPkg := chaincodeInfo.ExplicitCollectionConfigPkg

	if chaincodeInfo.IsLegacy {
		return explicitCollectionConfigPkg, nil
	}

	implicitCollections, err := vc.ImplicitCollections(channelName, chaincodeName, qe)
	if err != nil {
		return nil, err
	}

	var combinedColls []*pb.CollectionConfig
	if explicitCollectionConfigPkg != nil {
		combinedColls = append(combinedColls, explicitCollectionConfigPkg.Config...)
	}
	for _, implicitColl := range implicitCollections {
		c := &pb.CollectionConfig{}
		c.Payload = &pb.CollectionConfig_StaticCollectionConfig{StaticCollectionConfig: implicitColl}
		combinedColls = append(combinedColls, c)
	}
	return &pb.CollectionConfigPackage{
		Config: combinedColls,
	}, nil
}

// CollectionInfo implements function in interface ledger.DeployedChaincodeInfoProvider, it returns config for
// both static and implicit collections.
func (vc *ValidatorCommitter) CollectionInfo(channelName, chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*pb.StaticCollectionConfig, error) {
	exists, definedChaincode, err := vc.Resources.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "could not get chaincode")
	}

	if !exists {
		return vc.LegacyDeployedCCInfoProvider.CollectionInfo(channelName, chaincodeName, collectionName, qe)
	}

	isImplicitCollection, mspID := implicitcollection.MspIDIfImplicitCollection(collectionName)
	if isImplicitCollection {
		return vc.GenerateImplicitCollectionForOrg(mspID), nil
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
// a slice that contains one proto msg for each of the implicit collections
func (vc *ValidatorCommitter) ImplicitCollections(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) ([]*pb.StaticCollectionConfig, error) {
	exists, _, err := vc.Resources.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
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

	return vc.ChaincodeImplicitCollections(channelName)
}

// ChaincodeImplicitCollections assumes the chaincode exists in the new lifecycle and returns the implicit collections
func (vc *ValidatorCommitter) ChaincodeImplicitCollections(channelName string) ([]*pb.StaticCollectionConfig, error) {
	channelConfig := vc.Resources.ChannelConfigSource.GetStableChannelConfig(channelName)
	if channelConfig == nil {
		return nil, errors.Errorf("could not get channelconfig for channel %s", channelName)
	}
	ac, ok := channelConfig.ApplicationConfig()
	if !ok {
		return nil, errors.Errorf("could not get application config for channel %s", channelName)
	}

	orgs := ac.Organizations()
	implicitCollections := make([]*pb.StaticCollectionConfig, 0, len(orgs))
	for _, org := range orgs {
		implicitCollections = append(implicitCollections, vc.GenerateImplicitCollectionForOrg(org.MSPID()))
	}

	return implicitCollections, nil
}

// GenerateImplicitCollectionForOrg generates implicit collection for the org
func (vc *ValidatorCommitter) GenerateImplicitCollectionForOrg(mspid string) *pb.StaticCollectionConfig {
	// set Required/MaxPeerCount to 0 if it is other org's implicit collection (mspid does not match peer's local mspid)
	// set Required/MaxPeerCount to the config values if it is the peer org's implicit collection (mspid matches peer's local mspid)
	requiredPeerCount := 0
	maxPeerCount := 0
	if mspid == vc.CoreConfig.LocalMSPID {
		requiredPeerCount = vc.PrivdataConfig.ImplicitCollDisseminationPolicy.RequiredPeerCount
		maxPeerCount = vc.PrivdataConfig.ImplicitCollDisseminationPolicy.MaxPeerCount
	}
	return &pb.StaticCollectionConfig{
		Name: implicitcollection.NameForOrg(mspid),
		MemberOrgsPolicy: &pb.CollectionPolicyConfig{
			Payload: &pb.CollectionPolicyConfig_SignaturePolicy{
				SignaturePolicy: policydsl.SignedByMspMember(mspid),
			},
		},
		RequiredPeerCount: int32(requiredPeerCount),
		MaximumPeerCount:  int32(maxPeerCount),
	}
}

func (vc *ValidatorCommitter) ImplicitCollectionEndorsementPolicyAsBytes(channelID, orgMSPID string) (policy []byte, unexpectedErr, validationErr error) {
	channelConfig := vc.Resources.ChannelConfigSource.GetStableChannelConfig(channelID)
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
		return protoutil.MarshalOrPanic(&cb.ApplicationPolicy{
			Type: &cb.ApplicationPolicy_ChannelConfigPolicyReference{
				ChannelConfigPolicyReference: policyName,
			},
		}), nil, nil
	}

	// This was a channel which was upgraded or did not define an org level endorsement policy, use a default
	// of "any member of the org"

	return protoutil.MarshalOrPanic(&cb.ApplicationPolicy{
		Type: &cb.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: policydsl.SignedByAnyMember([]string{orgMSPID}),
		},
	}), nil, nil
}

// ValidationInfo returns the name and arguments of the validation plugin for the supplied
// chaincode. The function returns two types of errors, unexpected errors and validation
// errors. The reason for this is that this function is called from the validation code,
// which needs to differentiate the two types of error to halt processing on the channel
// if the unexpected error is not nil and mark the transaction as invalid if the validation
// error is not nil.
func (vc *ValidatorCommitter) ValidationInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (plugin string, args []byte, unexpectedErr error, validationErr error) {
	// TODO, this is a bit of an overkill check, and will need to be scaled back for non-chaincode type namespaces
	exists, definedChaincode, err := vc.Resources.ChaincodeDefinitionIfDefined(chaincodeName, &SimpleQueryExecutorShim{
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
		b, err := vc.Resources.LifecycleEndorsementPolicyAsBytes(channelID)
		if err != nil {
			return "", nil, errors.WithMessage(err, "unexpected failure to create lifecycle endorsement policy"), nil
		}
		return "vscc", b, nil, nil
	}

	return definedChaincode.ValidationInfo.ValidationPlugin, definedChaincode.ValidationInfo.ValidationParameter, nil, nil
}

// CollectionValidationInfo returns information about collections to the validation component
func (vc *ValidatorCommitter) CollectionValidationInfo(channelID, chaincodeName, collectionName string, state validationState.State) (args []byte, unexpectedErr, validationErr error) {
	exists, definedChaincode, err := vc.Resources.ChaincodeDefinitionIfDefined(chaincodeName, &ValidatorStateShim{
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

	isImplicitCollection, mspID := implicitcollection.MspIDIfImplicitCollection(collectionName)
	if isImplicitCollection {
		return vc.ImplicitCollectionEndorsementPolicyAsBytes(channelID, mspID)
	}

	if definedChaincode.Collections != nil {
		for _, conf := range definedChaincode.Collections.Config {
			staticCollConfig := conf.GetStaticCollectionConfig()
			if staticCollConfig != nil && staticCollConfig.Name == collectionName {
				if staticCollConfig.EndorsementPolicy != nil {
					return protoutil.MarshalOrPanic(staticCollConfig.EndorsementPolicy), nil, nil
				}
				// default to chaincode endorsement policy
				return definedChaincode.ValidationInfo.ValidationParameter, nil, nil
			}
		}
	}

	return nil, nil, errors.Errorf("no such collection '%s'", collectionName)
}
