/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// Application is a copy of the orderer configuration with the addition of an anchor peers
// list in the organization definition.
type Application struct {
	Organizations []Organization
	Capabilities  map[string]bool
	Policies      map[string]Policy
	ACLs          map[string]string
}

// AnchorPeer defines the endpoint of peers for each application organization.
type AnchorPeer struct {
	Host string
	Port int
}

// AddAnchorPeer adds an anchor peer to an existing channel config transaction.
// It must add the anchor peer to an existing org and the anchor peer must not already
// exist in the org.
func AddAnchorPeer(config *cb.Config, orgName string, newAnchorPeer AnchorPeer) error {
	applicationOrgGroup, ok := config.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return fmt.Errorf("application org %s does not exist in channel config", orgName)
	}

	anchorPeersProto := &pb.AnchorPeers{}

	if anchorPeerConfigValue, ok := applicationOrgGroup.Values[AnchorPeersKey]; ok {
		// Unmarshal existing anchor peers if the config value exists
		err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
		if err != nil {
			return fmt.Errorf("failed unmarshaling %s's anchor peer endpoints: %v", orgName, err)
		}
	}

	// Persist existing anchor peers if found
	anchorProtos := anchorPeersProto.AnchorPeers

	for _, anchorPeer := range anchorProtos {
		if anchorPeer.Host == newAnchorPeer.Host && anchorPeer.Port == int32(newAnchorPeer.Port) {
			return fmt.Errorf("application org %s already contains anchor peer endpoint %s:%d",
				orgName, newAnchorPeer.Host, newAnchorPeer.Port)
		}
	}

	// Append new anchor peer to anchorProtos
	anchorProtos = append(anchorProtos, &pb.AnchorPeer{
		Host: newAnchorPeer.Host,
		Port: int32(newAnchorPeer.Port),
	})

	// Add anchor peers config value back to application org
	err := addValue(applicationOrgGroup, anchorPeersValue(anchorProtos), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// RemoveAnchorPeer removes an anchor peer from an existing channel config transaction.
// The removed anchor peer and org it belongs to must both already exist.
func RemoveAnchorPeer(config *cb.Config, orgName string, anchorPeerToRemove AnchorPeer) error {
	applicationOrgGroup, ok := config.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return fmt.Errorf("application org %s does not exist in channel config", orgName)
	}

	anchorPeersProto := &pb.AnchorPeers{}

	if anchorPeerConfigValue, ok := applicationOrgGroup.Values[AnchorPeersKey]; ok {
		// Unmarshal existing anchor peers if the config value exists
		err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
		if err != nil {
			return fmt.Errorf("failed unmarshaling %s's anchor peer endpoints: %v", orgName, err)
		}
	}

	existingAnchorPeers := anchorPeersProto.AnchorPeers

	for i, anchorPeer := range existingAnchorPeers {
		if anchorPeer.Host == anchorPeerToRemove.Host && anchorPeer.Port == int32(anchorPeerToRemove.Port) {
			existingAnchorPeers = append(existingAnchorPeers[:i], existingAnchorPeers[i+1:]...)

			// Add anchor peers config value back to application org
			err := addValue(applicationOrgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
			if err != nil {
				return fmt.Errorf("failed to remove anchor peer %v from org %s: %v", anchorPeerToRemove, orgName, err)
			}

			return nil
		}
	}

	return fmt.Errorf("could not find anchor peer %s:%d in %s's anchor peer endpoints", anchorPeerToRemove.Host, anchorPeerToRemove.Port, orgName)
}

// GetAnchorPeers retrieves existing anchor peers from a application organization.
func GetAnchorPeers(config *cb.Config, orgName string) ([]AnchorPeer, error) {
	applicationOrgGroup, ok := config.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return nil, fmt.Errorf("application org %s does not exist in channel config", orgName)
	}

	anchorPeerConfigValue, ok := applicationOrgGroup.Values[AnchorPeersKey]
	if !ok {
		return nil, fmt.Errorf("application org %s does not have anchor peers", orgName)
	}

	anchorPeersProto := &pb.AnchorPeers{}

	err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
	if err != nil {
		return nil, fmt.Errorf("failed unmarshaling %s's anchor peer endpoints: %v", orgName, err)
	}

	anchorPeers := []AnchorPeer{}
	for _, ap := range anchorPeersProto.AnchorPeers {
		anchorPeers = append(anchorPeers, AnchorPeer{
			Host: ap.Host,
			Port: int(ap.Port),
		})
	}

	return anchorPeers, nil
}

// newApplicationGroup returns the application component of the channel configuration.
// By default, it sets the mod_policy of all elements to "Admins".
func newApplicationGroup(application Application) (*cb.ConfigGroup, error) {
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
		applicationGroup.Groups[org.Name] = newConfigGroup()
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

// getApplicationOrg returns the organization config group for an org in the
// provided config. It returns an error if an application org was not found
// in the config with the specified name.
func getApplicationOrg(config *cb.Config, orgName string) (*cb.ConfigGroup, error) {
	org, ok := config.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return nil, fmt.Errorf("application org with name '%s' not found", orgName)
	}
	return org, nil
}

// AddApplicationOrg adds an organization to an existing config's Application configuration.
// Will not error if organization already exists.
func AddApplicationOrg(config *cb.Config, org Organization) error {
	appGroup := config.ChannelGroup.Groups[ApplicationGroupKey]

	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create application org %s: %v", org.Name, err)
	}

	appGroup.Groups[org.Name] = orgGroup

	return nil
}
