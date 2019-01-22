/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"fmt"
	"regexp"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"

	"github.com/pkg/errors"
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

// ChaincodeInNewLifecycle returns whether the chaincode name is defined in the new lifecycle, a shim around
// the SimpleQueryExecutor to work with the serializer, or an error.  If the namespace is defined, but it is
// not a chaincode, this is considered an error.
func (l *Lifecycle) ChaincodeInNewLifecycle(chaincodeName string, qe ledger.SimpleQueryExecutor) (bool, *SimpleQueryExecutorShim, error) {
	state := &SimpleQueryExecutorShim{
		Namespace:           LifecycleNamespace,
		SimpleQueryExecutor: qe,
	}

	if chaincodeName == LifecycleNamespace {
		return true, state, nil
	}

	metadata, err := l.Serializer.DeserializeMetadata(NamespacesName, chaincodeName, state, false)
	if err != nil {
		return false, nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize metadata for chaincode %s", chaincodeName))
	}

	if metadata.Datatype == "" {
		// If the type is unset, then fallback to the legacy definition
		return false, state, nil
	}

	if metadata.Datatype != DefinedChaincodeType {
		return false, nil, errors.Errorf("not a chaincode type: %s", metadata.Datatype)
	}

	return true, state, nil
}

func (l *Lifecycle) ChaincodeInfo(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
	exists, state, err := l.ChaincodeInNewLifecycle(chaincodeName, qe)
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

	if chaincodeName == LifecycleNamespace {
		return &ledger.DeployedChaincodeInfo{
			Name:                chaincodeName,
			ImplicitCollections: ic,
		}, nil
	}

	definedChaincode := &DefinedChaincode{}
	err = l.Serializer.Deserialize(NamespacesName, chaincodeName, definedChaincode, state)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("could not deserialize chaincode definition for chaincode %s", chaincodeName))
	}

	return &ledger.DeployedChaincodeInfo{
		Name:                        chaincodeName,
		Version:                     definedChaincode.Version,
		Hash:                        definedChaincode.Hash,
		ExplicitCollectionConfigPkg: definedChaincode.Collections,
		ImplicitCollections:         ic,
	}, nil
}

// CollectionInfo implements function in interface ledger.DeployedChaincodeInfoProvider, it returns config for
// both static and implicit collections.
func (l *Lifecycle) CollectionInfo(channelName, chaincodeName, collectionName string, qe ledger.SimpleQueryExecutor) (*cb.StaticCollectionConfig, error) {
	return l.LegacyDeployedCCInfoProvider.CollectionInfo(channelName, chaincodeName, collectionName, qe)
}

// ImplicitCollections implements function in interface ledger.DeployedChaincodeInfoProvider.  It returns
//a slice that contains one proto msg for each of the implicit collections
func (l *Lifecycle) ImplicitCollections(channelName, chaincodeName string, qe ledger.SimpleQueryExecutor) ([]*cb.StaticCollectionConfig, error) {
	exists, _, err := l.ChaincodeInNewLifecycle(chaincodeName, qe)
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
