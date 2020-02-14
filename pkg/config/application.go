/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// Application encodes the application-level configuration needed in config
// transactions.
type Application struct {
	Organizations []*Organization
	Capabilities  map[string]bool
	Resources     *Resources
	Policies      map[string]*Policy
	ACLs          map[string]string
}

// AnchorPeer encodes the necessary fields to identify an anchor peer.
type AnchorPeer struct {
	Host string
	Port int
}

// NewApplicationGroup returns the application component of the channel configuration.
// It defines the organizations which are involved in application logic like chaincodes,
// and how these members may interact with the orderer.
// It sets the mod_policy of all elements to "Admins".
func NewApplicationGroup(conf *Application, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	var err error

	applicationGroup := newConfigGroup()
	applicationGroup.ModPolicy = AdminsPolicyKey

	if err = addPolicies(applicationGroup, conf.Policies, AdminsPolicyKey); err != nil {
		return nil, fmt.Errorf("failed to add policies: %v", err)
	}

	if len(conf.ACLs) > 0 {
		err = addValue(applicationGroup, aclValues(conf.ACLs), AdminsPolicyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to add acl values: %v", err)
		}
	}

	if len(conf.Capabilities) > 0 {
		err = addValue(applicationGroup, capabilitiesValue(conf.Capabilities), AdminsPolicyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to add capabilities value: %v", err)
		}
	}

	for _, org := range conf.Organizations {
		applicationGroup.Groups[org.Name], err = newApplicationOrgGroup(org, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create application org group %s: %v", org.Name, err)
		}
	}

	return applicationGroup, nil
}

// newApplicationOrgGroup returns an application org component of the channel configuration.
// It defines the crypto material for the organization (its MSP), as well as its anchor peers
// for use by the gossip network.
// It sets the mod_policy of all elements to "Admins".
func newApplicationOrgGroup(conf *Organization, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	var err error

	applicationOrgGroup := newConfigGroup()
	applicationOrgGroup.ModPolicy = AdminsPolicyKey

	if conf.SkipAsForeign {
		return applicationOrgGroup, nil
	}

	if err = addPolicies(applicationOrgGroup, conf.Policies, AdminsPolicyKey); err != nil {
		return nil, fmt.Errorf("failed to add policies: %v", err)
	}

	err = addValue(applicationOrgGroup, mspValue(mspConfig), AdminsPolicyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to add msp value: %v", err)
	}

	var anchorProtos []*pb.AnchorPeer
	for _, anchorPeer := range conf.AnchorPeers {
		anchorProtos = append(anchorProtos, &pb.AnchorPeer{
			Host: anchorPeer.Host,
			Port: int32(anchorPeer.Port),
		})
	}

	// Avoid adding an unnecessary anchor peers element when one is not required
	// This helps prevent a delta from the orderer system channel when computing
	// more complex channel creation transactions
	if len(anchorProtos) > 0 {
		err = addValue(applicationOrgGroup, anchorPeersValue(anchorProtos), AdminsPolicyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to add anchor peers value: %v", err)
		}
	}

	return applicationOrgGroup, nil
}

// aclValues returns the config definition for an application's resources based ACL definitions.
// It is a value for the /Channel/Application/.
func aclValues(acls map[string]string) *standardConfigValue {
	a := &pb.ACLs{
		Acls: make(map[string]*pb.APIResource),
	}

	for apiResource, policyRef := range acls {
		a.Acls[apiResource] = &pb.APIResource{PolicyRef: policyRef}
	}

	return &standardConfigValue{
		key:   ACLsKey,
		value: a,
	}
}

// anchorPeersValue returns the config definition for an org's anchor peers.
// It is a value for the /Channel/Application/*.
func anchorPeersValue(anchorPeers []*pb.AnchorPeer) *standardConfigValue {
	return &standardConfigValue{
		key:   AnchorPeersKey,
		value: &pb.AnchorPeers{AnchorPeers: anchorPeers},
	}
}
