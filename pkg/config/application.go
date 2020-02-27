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
// By default, tt sets the mod_policy of all elements to "Admins".
func NewApplicationGroup(application *Application, mspConfig *mb.MSPConfig) (*cb.ConfigGroup, error) {
	var err error

	applicationGroup := newConfigGroup()
	applicationGroup.ModPolicy = AdminsPolicyKey

	if err = addPolicies(applicationGroup, application.Policies, AdminsPolicyKey); err != nil {
		return nil, err
	}

	if len(application.ACLs) > 0 {
		err = addValue(applicationGroup, aclValues(application.ACLs), AdminsPolicyKey)
		if err != nil {
			return nil, err
		}
	}

	if len(application.Capabilities) > 0 {
		err = addValue(applicationGroup, capabilitiesValue(application.Capabilities), AdminsPolicyKey)
		if err != nil {
			return nil, err
		}
	}

	for _, org := range application.Organizations {
		applicationGroup.Groups[org.Name], err = newOrgConfigGroup(org, mspConfig)
		if err != nil {
			return nil, fmt.Errorf("org group '%s': %v", org.Name, err)
		}
	}

	return applicationGroup, nil
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
