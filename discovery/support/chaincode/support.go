/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/golang/protobuf/proto"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policies/inquire"
)

var logger = flogging.MustGetLogger("discovery.DiscoverySupport")

type MetadataRetriever interface {
	Metadata(channel string, cc string, collections ...string) *chaincode.Metadata
}

// DiscoverySupport implements support that is used for service discovery
// that is related to chaincode
type DiscoverySupport struct {
	ci MetadataRetriever
}

// NewDiscoverySupport creates a new DiscoverySupport
func NewDiscoverySupport(ci MetadataRetriever) *DiscoverySupport {
	s := &DiscoverySupport{
		ci: ci,
	}
	return s
}

func (s *DiscoverySupport) PoliciesByChaincode(channel string, cc string, collections ...string) []policies.InquireablePolicy {
	chaincodeData := s.ci.Metadata(channel, cc, collections...)
	if chaincodeData == nil {
		logger.Infof("Chaincode %s wasn't found", cc)
		return nil
	}

	// chaincode policy
	pol := &common2.SignaturePolicyEnvelope{}
	if err := proto.Unmarshal(chaincodeData.Policy, pol); err != nil {
		logger.Errorf("Failed unmarshalling policy for chaincode '%s': %s", cc, err)
		return nil
	}
	if len(pol.Identities) == 0 || pol.Rule == nil {
		logger.Errorf("Invalid policy, either Identities(%v) or Rule(%v) are empty", pol.Identities, pol.Rule)
		return nil
	}
	// chaincodeData.CollectionPolicies will be nil when using legacy lifecycle (lscc)
	// because collection endorsement policies are not supported in lscc
	chaincodePolicy := inquire.NewInquireableSignaturePolicy(pol)
	if chaincodeData.CollectionPolicies == nil || len(collections) == 0 {
		return []policies.InquireablePolicy{chaincodePolicy}
	}

	// process any collection policies
	inquireablePolicies := make(map[string]struct{})
	uniqueInquireablePolicies := []policies.InquireablePolicy{}

	for _, collectionName := range collections {
		// default to the chaincode policy if the collection policy doesn't exist
		if _, collectionPolicyExists := chaincodeData.CollectionPolicies[collectionName]; !collectionPolicyExists {
			if _, exists := inquireablePolicies[string(chaincodeData.Policy)]; !exists {
				uniqueInquireablePolicies = append(uniqueInquireablePolicies, chaincodePolicy)
				inquireablePolicies[string(chaincodeData.Policy)] = struct{}{}
			}
			continue
		}

		collectionPolicy := chaincodeData.CollectionPolicies[collectionName]
		pol := &common2.SignaturePolicyEnvelope{}
		if err := proto.Unmarshal(collectionPolicy, pol); err != nil {
			logger.Errorf("Failed unmarshalling collection policy for chaincode '%s' collection '%s': %s", cc, collectionName, err)
			return nil
		}
		if len(pol.Identities) == 0 || pol.Rule == nil {
			logger.Errorf("Invalid collection policy, either Identities(%v) or Rule(%v) are empty", pol.Identities, pol.Rule)
			return nil
		}
		// only add to uniqueInquireablePolicies if the policy doesn't already exist there
		// this prevents duplicate inquireablePolicies from being returned
		if _, exists := inquireablePolicies[string(collectionPolicy)]; !exists {
			uniqueInquireablePolicies = append(uniqueInquireablePolicies, inquire.NewInquireableSignaturePolicy(pol))
			inquireablePolicies[string(collectionPolicy)] = struct{}{}
		}
	}

	return uniqueInquireablePolicies
}
