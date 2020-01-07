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
		logger.Warningf("Failed unmarshaling policy for chaincode '%s': %s", cc, err)
		return nil
	}
	if len(pol.Identities) == 0 || pol.Rule == nil {
		logger.Warningf("Invalid policy, either Identities(%v) or Rule(%v) are empty", pol.Identities, pol.Rule)
		return nil
	}
	chaincodePolicy := inquire.NewInquireableSignaturePolicy(pol)

	inquireablePolicies := map[policies.InquireablePolicy]bool{}
	uniqueInquireablePolicies := []policies.InquireablePolicy{}

	// chaincodeData.CollectionPolicies will be nil when using legacy lifecycle (lscc)
	// because collection endorsement policies are not supported in lscc
	if chaincodeData.CollectionPolicies == nil || len(collections) == 0 {
		return []policies.InquireablePolicy{chaincodePolicy}
	}
	// process any collection policies
	for _, collectionName := range collections {
		// add the collection policy if it exists otherwise default to the chaincode policy
		if policy, collectionPolicyExists := chaincodeData.CollectionPolicies[collectionName]; collectionPolicyExists {
			pol := &common2.SignaturePolicyEnvelope{}
			if err := proto.Unmarshal(policy, pol); err != nil {
				logger.Warningf("Failed unmarshaling collection policy for chaincode '%s' collection '%s': %s", cc, collectionName, err)
				return nil
			}
			if len(pol.Identities) == 0 || pol.Rule == nil {
				logger.Warningf("Invalid collection policy, either Identities(%v) or Rule(%v) are empty", pol.Identities, pol.Rule)
				return nil
			}
			inquireablePolicy := inquire.NewInquireableSignaturePolicy(pol)
			// only add to uniqueInquireablePolicies if the inquireablePolicy doesn't already exist there
			// this prevents duplicate inquireablePolicies from being returned
			if _, exists := inquireablePolicies[inquireablePolicy]; !exists {
				uniqueInquireablePolicies = append(uniqueInquireablePolicies, inquireablePolicy)
				inquireablePolicies[inquireablePolicy] = true
			}
		} else if _, exists := inquireablePolicies[chaincodePolicy]; !exists {
			uniqueInquireablePolicies = append(uniqueInquireablePolicies, chaincodePolicy)
			inquireablePolicies[chaincodePolicy] = true
		}
	}

	return uniqueInquireablePolicies
}
