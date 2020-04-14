/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// Application is a copy of the orderer configuration with the addition of an anchor peers
// list in the organization definition.
type Application struct {
	Organizations []Organization
	Capabilities  []string
	Policies      map[string]Policy
	ACLs          map[string]string
}

// ApplicationConfiguration returns the existing application configuration values from a config
// transaction as an Application type. This can be used to retrieve existing values for the application
// prior to updating the application configuration.
func (c *ConfigTx) ApplicationConfiguration() (Application, error) {
	applicationGroup, ok := c.base.ChannelGroup.Groups[ApplicationGroupKey]
	if !ok {
		return Application{}, errors.New("config does not contain application group")
	}

	var applicationOrgs []Organization
	for orgName := range applicationGroup.Groups {
		orgConfig, err := c.ApplicationOrg(orgName)
		if err != nil {
			return Application{}, fmt.Errorf("retrieving application org %s: %v", orgName, err)
		}

		applicationOrgs = append(applicationOrgs, orgConfig)
	}

	capabilities, err := c.ApplicationCapabilities()
	if err != nil {
		return Application{}, fmt.Errorf("retrieving application capabilities: %v", err)
	}

	policies, err := c.ApplicationPolicies()
	if err != nil {
		return Application{}, fmt.Errorf("retrieving application policies: %v", err)
	}

	acls, err := c.ApplicationACLs()
	if err != nil {
		return Application{}, fmt.Errorf("retrieving application acls: %v", err)
	}

	return Application{
		Organizations: applicationOrgs,
		Capabilities:  capabilities,
		Policies:      policies,
		ACLs:          acls,
	}, nil
}

// AddAnchorPeer adds an anchor peer to an existing channel config transaction.
// It must add the anchor peer to an existing org and the anchor peer must not already
// exist in the org.
func (c *ConfigTx) AddAnchorPeer(orgName string, newAnchorPeer Address) error {
	applicationOrgGroup, ok := c.updated.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
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
	err := setValue(applicationOrgGroup, anchorPeersValue(anchorProtos), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// RemoveAnchorPeer removes an anchor peer from an existing channel config transaction.
// The removed anchor peer and org it belongs to must both already exist.
func (c *ConfigTx) RemoveAnchorPeer(orgName string, anchorPeerToRemove Address) error {
	applicationOrgGroup, ok := c.updated.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
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

	existingAnchorPeers := anchorPeersProto.AnchorPeers[:0]
	for _, anchorPeer := range anchorPeersProto.AnchorPeers {
		if anchorPeer.Host != anchorPeerToRemove.Host || anchorPeer.Port != int32(anchorPeerToRemove.Port) {
			existingAnchorPeers = append(existingAnchorPeers, anchorPeer)

			// Add anchor peers config value back to application org
			err := setValue(applicationOrgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
			if err != nil {
				return fmt.Errorf("failed to remove anchor peer %v from org %s: %v", anchorPeerToRemove, orgName, err)
			}

			return nil
		}
	}

	if len(existingAnchorPeers) == len(anchorPeersProto.AnchorPeers) {
		return fmt.Errorf("could not find anchor peer %s:%d in application org %s", anchorPeerToRemove.Host, anchorPeerToRemove.Port, orgName)
	}

	// Add anchor peers config value back to application org
	err := setValue(applicationOrgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to remove anchor peer %v from org %s: %v", anchorPeerToRemove, orgName, err)
	}

	return nil
}

// AddACLs adds ACLS to an existing channel config application.
func (c *ConfigTx) AddACLs(acls map[string]string) error {
	configACLs, err := getACLs(c.updated)
	if err != nil {
		return err
	}

	for apiResource, policyRef := range configACLs {
		acls[apiResource] = policyRef
	}

	err = setValue(c.updated.ChannelGroup.Groups[ApplicationGroupKey], aclValues(acls), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// RemoveACLs a list of ACLs from given channel config application.
func (c *ConfigTx) RemoveACLs(acls []string) error {
	configACLs, err := getACLs(c.updated)
	if err != nil {
		return err
	}

	for _, acl := range acls {
		delete(configACLs, acl)
	}

	err = setValue(c.updated.ChannelGroup.Groups[ApplicationGroupKey], aclValues(configACLs), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// ApplicationACLs returns a map of application acls from a config transaction.
func (c *ConfigTx) ApplicationACLs() (map[string]string, error) {
	return getACLs(c.base)
}

// getACLs returns a map of ACLS for given config application.
func getACLs(config *cb.Config) (map[string]string, error) {
	applicationGroup, ok := config.ChannelGroup.Groups[ApplicationGroupKey]
	if !ok {
		return nil, fmt.Errorf("application does not exist in channel config")
	}

	ACLProtos := &pb.ACLs{}

	err := unmarshalConfigValueAtKey(applicationGroup, ACLsKey, ACLProtos)
	if err != nil {
		return nil, err
	}

	retACLs := map[string]string{}
	for apiResource, policyRef := range ACLProtos.Acls {
		retACLs[apiResource] = policyRef.PolicyRef
	}
	return retACLs, nil
}

// AnchorPeers retrieves existing anchor peers from a application organization.
func (c *ConfigTx) AnchorPeers(orgName string) ([]Address, error) {
	applicationOrgGroup, ok := c.base.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
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

	anchorPeers := []Address{}
	for _, ap := range anchorPeersProto.AnchorPeers {
		anchorPeers = append(anchorPeers, Address{
			Host: ap.Host,
			Port: int(ap.Port),
		})
	}

	return anchorPeers, nil
}

// AddApplicationOrg adds an organization to an existing Application configuration.
// Will not error if organization already exists.
func (c *ConfigTx) AddApplicationOrg(org Organization) error {
	appGroup := c.updated.ChannelGroup.Groups[ApplicationGroupKey]

	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create application org %s: %v", org.Name, err)
	}

	appGroup.Groups[org.Name] = orgGroup

	return nil
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
		err = setValue(applicationGroup, aclValues(application.ACLs), AdminsPolicyKey)
		if err != nil {
			return nil, err
		}
	}

	if len(application.Capabilities) > 0 {
		err = setValue(applicationGroup, capabilitiesValue(application.Capabilities), AdminsPolicyKey)
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
