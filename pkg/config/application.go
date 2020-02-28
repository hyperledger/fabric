/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"

	"github.com/golang/protobuf/proto"
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

// RemoveAnchorPeer removes an anchor peer from an existing channel config transaction.
// The removed anchor peer and org it belongs to must both already exist.
func RemoveAnchorPeer(config *cb.Config, orgName string, anchorPeerToRemove *AnchorPeer) error {
	applicationOrgGroup, ok := config.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return fmt.Errorf("application org %s does not exist in channel config", orgName)
	}

	anchorPeersProto := &pb.AnchorPeers{}

	if anchorPeerConfigValue, ok := applicationOrgGroup.Values[AnchorPeersKey]; ok {
		// Unmarshal existing anchor peers if the config value exists
		err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
		if err != nil {
			return fmt.Errorf("failed unmarshalling existing anchor peers: %v", err)
		}
	}

	existingAnchorPeers := anchorPeersProto.AnchorPeers

	for i, anchorPeer := range existingAnchorPeers {
		if anchorPeer.Host == anchorPeerToRemove.Host && anchorPeer.Port == int32(anchorPeerToRemove.Port) {
			existingAnchorPeers = append(existingAnchorPeers[:i], existingAnchorPeers[i+1:]...)

			// Add anchor peers config value back to application org
			err := addValue(applicationOrgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
			if err != nil {
				return fmt.Errorf("failed to remove anchor peer %v: %v", anchorPeerToRemove, err)
			}

			return nil
		}
	}

	return fmt.Errorf("could not find anchor peer with host: %s, port: %d in existing anchor peers", anchorPeerToRemove.Host, anchorPeerToRemove.Port)
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
