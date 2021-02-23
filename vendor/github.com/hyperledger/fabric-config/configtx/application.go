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
	ModPolicy     string
}

// ApplicationGroup encapsulates the part of the config that controls
// application channels.
type ApplicationGroup struct {
	applicationGroup *cb.ConfigGroup
}

// ApplicationOrg encapsulates the parts of the config that control
// an application organization's configuration.
type ApplicationOrg struct {
	orgGroup *cb.ConfigGroup
	name     string
}

// MSP returns an OrganizationMSP object that can be used to configure the organization's MSP.
func (a *ApplicationOrg) MSP() *OrganizationMSP {
	return &OrganizationMSP{
		configGroup: a.orgGroup,
	}
}

// Application returns the application group the updated config.
func (c *ConfigTx) Application() *ApplicationGroup {
	applicationGroup := c.updated.ChannelGroup.Groups[ApplicationGroupKey]
	return &ApplicationGroup{applicationGroup: applicationGroup}
}

// Organization returns the application org from the updated config.
func (a *ApplicationGroup) Organization(name string) *ApplicationOrg {
	organizationGroup, ok := a.applicationGroup.Groups[name]
	if !ok {
		return nil
	}
	return &ApplicationOrg{name: name, orgGroup: organizationGroup}
}

// SetOrganization sets the organization config group for the given application
// org key in an existing Application configuration's Groups map.
// If the application org already exists in the current configuration, its value will be overwritten.
func (a *ApplicationGroup) SetOrganization(org Organization) error {
	orgGroup, err := newApplicationOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create application org %s: %v", org.Name, err)
	}

	a.applicationGroup.Groups[org.Name] = orgGroup

	return nil
}

// RemoveOrganization removes an org from the Application group.
// Removal will panic if the application group does not exist.
func (a *ApplicationGroup) RemoveOrganization(orgName string) {
	delete(a.applicationGroup.Groups, orgName)
}

// Configuration returns the existing application configuration values from a config
// transaction as an Application type. This can be used to retrieve existing values for the application
// prior to updating the application configuration.
func (a *ApplicationGroup) Configuration() (Application, error) {
	var applicationOrgs []Organization
	for orgName := range a.applicationGroup.Groups {
		orgConfig, err := a.Organization(orgName).Configuration()
		if err != nil {
			return Application{}, fmt.Errorf("retrieving application org %s: %v", orgName, err)
		}

		applicationOrgs = append(applicationOrgs, orgConfig)
	}

	capabilities, err := a.Capabilities()
	if err != nil {
		return Application{}, fmt.Errorf("retrieving application capabilities: %v", err)
	}

	policies, err := a.Policies()
	if err != nil {
		return Application{}, fmt.Errorf("retrieving application policies: %v", err)
	}

	acls, err := a.ACLs()
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

// Configuration returns the existing application org configuration values
// from the updated config.
func (a *ApplicationOrg) Configuration() (Organization, error) {
	org, err := getOrganization(a.orgGroup, a.name)
	if err != nil {
		return Organization{}, err
	}
	return org, nil
}

// Capabilities returns a map of enabled application capabilities
// from the updated config.
func (a *ApplicationGroup) Capabilities() ([]string, error) {
	capabilities, err := getCapabilities(a.applicationGroup)
	if err != nil {
		return nil, fmt.Errorf("retrieving application capabilities: %v", err)
	}

	return capabilities, nil
}

// AddCapability sets capability to the provided channel config.
// If the provided capability already exists in current configuration, this action
// will be a no-op.
func (a *ApplicationGroup) AddCapability(capability string) error {
	capabilities, err := a.Capabilities()
	if err != nil {
		return err
	}

	err = addCapability(a.applicationGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveCapability removes capability to the provided channel config.
func (a *ApplicationGroup) RemoveCapability(capability string) error {
	capabilities, err := a.Capabilities()
	if err != nil {
		return err
	}

	err = removeCapability(a.applicationGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// Policies returns a map of policies for the application config group in
// the updatedconfig.
func (a *ApplicationGroup) Policies() (map[string]Policy, error) {
	return getPolicies(a.applicationGroup.Policies)
}

// SetModPolicy sets the specified modification policy for the application group.
func (a *ApplicationGroup) SetModPolicy(modPolicy string) error {
	if modPolicy == "" {
		return errors.New("non empty mod policy is required")
	}

	a.applicationGroup.ModPolicy = modPolicy

	return nil
}

// SetPolicy sets the specified policy in the application group's config policy map.
// If the policy already exists in current configuration, its value will be overwritten.
func (a *ApplicationGroup) SetPolicy(policyName string, policy Policy) error {
	err := setPolicy(a.applicationGroup, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// SetPolicies sets the specified policies in the application group's config policy map.
// If the policies already exist in current configuration, the values will be replaced with new policies.
func (a *ApplicationGroup) SetPolicies(policies map[string]Policy) error {
	err := setPolicies(a.applicationGroup, policies)
	if err != nil {
		return fmt.Errorf("failed to set policies: %v", err)
	}

	return nil
}

// RemovePolicy removes an existing policy from an application's configuration.
// Removal will panic if the application group does not exist.
func (a *ApplicationGroup) RemovePolicy(policyName string) error {
	policies, err := a.Policies()
	if err != nil {
		return err
	}

	removePolicy(a.applicationGroup, policyName, policies)
	return nil
}

// Policies returns the map of policies for a specific application org in
// the updated config.
func (a *ApplicationOrg) Policies() (map[string]Policy, error) {
	return getPolicies(a.orgGroup.Policies)
}

// SetModPolicy sets the specified modification policy for the application organization group.
func (a *ApplicationOrg) SetModPolicy(modPolicy string) error {
	if modPolicy == "" {
		return errors.New("non empty mod policy is required")
	}

	a.orgGroup.ModPolicy = modPolicy

	return nil
}

// SetPolicy sets the specified policy in the application org group's config policy map.
// If an Organization policy already exists in current configuration, its value will be overwritten.
func (a *ApplicationOrg) SetPolicy(policyName string, policy Policy) error {
	err := setPolicy(a.orgGroup, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// SetPolicies sets the specified policies in the application org group's config policy map.
// If the policies already exist in current configuration, the values will be replaced with new policies.
func (a *ApplicationOrg) SetPolicies(policies map[string]Policy) error {
	err := setPolicies(a.orgGroup, policies)
	if err != nil {
		return fmt.Errorf("failed to set policies: %v", err)
	}

	return nil
}

// RemovePolicy removes an existing policy from an application organization.
func (a *ApplicationOrg) RemovePolicy(policyName string) error {
	policies, err := a.Policies()
	if err != nil {
		return err
	}

	removePolicy(a.orgGroup, policyName, policies)
	return nil
}

// AnchorPeers returns the list of anchor peers for an application org
// in the updated config.
func (a *ApplicationOrg) AnchorPeers() ([]Address, error) {
	anchorPeerConfigValue, ok := a.orgGroup.Values[AnchorPeersKey]
	if !ok {
		return nil, nil
	}

	anchorPeersProto := &pb.AnchorPeers{}

	err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
	if err != nil {
		return nil, fmt.Errorf("failed unmarshaling %s's anchor peer endpoints: %v", a.name, err)
	}

	if len(anchorPeersProto.AnchorPeers) == 0 {
		return nil, nil
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

// AddAnchorPeer adds an anchor peer to an application org's configuration
// in the updated config.
func (a *ApplicationOrg) AddAnchorPeer(newAnchorPeer Address) error {
	anchorPeersProto := &pb.AnchorPeers{}

	if anchorPeerConfigValue, ok := a.orgGroup.Values[AnchorPeersKey]; ok {
		// Unmarshal existing anchor peers if the config value exists
		err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
		if err != nil {
			return fmt.Errorf("failed unmarshaling anchor peer endpoints: %v", err)
		}
	}

	// Persist existing anchor peers if found
	anchorProtos := anchorPeersProto.AnchorPeers

	for _, anchorPeer := range anchorProtos {
		if anchorPeer.Host == newAnchorPeer.Host && anchorPeer.Port == int32(newAnchorPeer.Port) {
			return nil
		}
	}

	// Append new anchor peer to anchorProtos
	anchorProtos = append(anchorProtos, &pb.AnchorPeer{
		Host: newAnchorPeer.Host,
		Port: int32(newAnchorPeer.Port),
	})

	// Add anchor peers config value back to application org
	err := setValue(a.orgGroup, anchorPeersValue(anchorProtos), AdminsPolicyKey)
	if err != nil {
		return err
	}
	return nil
}

// RemoveAnchorPeer removes an anchor peer from an application org's configuration
// in the updated config.
func (a *ApplicationOrg) RemoveAnchorPeer(anchorPeerToRemove Address) error {
	anchorPeersProto := &pb.AnchorPeers{}

	if anchorPeerConfigValue, ok := a.orgGroup.Values[AnchorPeersKey]; ok {
		// Unmarshal existing anchor peers if the config value exists
		err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
		if err != nil {
			return fmt.Errorf("failed unmarshaling anchor peer endpoints for application org %s: %v", a.name, err)
		}
	}

	existingAnchorPeers := anchorPeersProto.AnchorPeers[:0]
	for _, anchorPeer := range anchorPeersProto.AnchorPeers {
		if anchorPeer.Host != anchorPeerToRemove.Host || anchorPeer.Port != int32(anchorPeerToRemove.Port) {
			existingAnchorPeers = append(existingAnchorPeers, anchorPeer)

			// Add anchor peers config value back to application org
			err := setValue(a.orgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
			if err != nil {
				return fmt.Errorf("failed to remove anchor peer %v from org %s: %v", anchorPeerToRemove, a.name, err)
			}

			return nil
		}
	}

	if len(existingAnchorPeers) == len(anchorPeersProto.AnchorPeers) {
		return fmt.Errorf("could not find anchor peer %s:%d in application org %s", anchorPeerToRemove.Host, anchorPeerToRemove.Port, a.name)
	}

	// Add anchor peers config value back to application org
	err := setValue(a.orgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to remove anchor peer %v from org %s: %v", anchorPeerToRemove, a.name, err)
	}

	return nil
}

// ACLs returns a map of ACLS for given config application.
func (a *ApplicationGroup) ACLs() (map[string]string, error) {
	aclConfigValue, ok := a.applicationGroup.Values[ACLsKey]
	if !ok {
		return nil, nil
	}

	aclProtos := &pb.ACLs{}

	err := proto.Unmarshal(aclConfigValue.Value, aclProtos)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling %s: %v", ACLsKey, err)
	}

	retACLs := map[string]string{}
	for apiResource, policyRef := range aclProtos.Acls {
		retACLs[apiResource] = policyRef.PolicyRef
	}

	return retACLs, nil
}

// SetACLs sets ACLS to an existing channel config application.
// If an ACL already exists in current configuration, it will be replaced with new ACL.
func (a *ApplicationGroup) SetACLs(acls map[string]string) error {
	err := setValue(a.applicationGroup, aclValues(acls), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// RemoveACLs a list of ACLs from given channel config application.
// Specifying acls that do not exist in the application ConfigGroup of the channel config will not return a error.
// Removal will panic if application group does not exist.
func (a *ApplicationGroup) RemoveACLs(acls []string) error {
	configACLs, err := a.ACLs()
	if err != nil {
		return err
	}

	for _, acl := range acls {
		delete(configACLs, acl)
	}

	err = setValue(a.applicationGroup, aclValues(configACLs), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// SetMSP updates the MSP config for the specified application
// org group.
func (a *ApplicationOrg) SetMSP(updatedMSP MSP) error {
	currentMSP, err := a.MSP().Configuration()
	if err != nil {
		return fmt.Errorf("retrieving msp: %v", err)
	}

	if currentMSP.Name != updatedMSP.Name {
		return errors.New("MSP name cannot be changed")
	}

	err = updatedMSP.validateCACerts()
	if err != nil {
		return err
	}

	err = a.setMSPConfig(updatedMSP)
	if err != nil {
		return err
	}

	return nil
}

func (a *ApplicationOrg) setMSPConfig(updatedMSP MSP) error {
	mspConfig, err := newMSPConfig(updatedMSP)
	if err != nil {
		return fmt.Errorf("new msp config: %v", err)
	}

	err = setValue(a.orgGroup, mspValue(mspConfig), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// newApplicationGroupTemplate returns the application component of the channel
// configuration with only the names of the application organizations.
// By default, it sets the mod_policy of all elements to "Admins".
func newApplicationGroupTemplate(application Application) (*cb.ConfigGroup, error) {
	var err error

	applicationGroup := newConfigGroup()
	applicationGroup.ModPolicy = AdminsPolicyKey

	if application.ModPolicy != "" {
		applicationGroup.ModPolicy = application.ModPolicy
	}

	if err = setPolicies(applicationGroup, application.Policies); err != nil {
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

// newApplicationGroup returns the application component of the channel
// configuration with the entire configuration for application organizations.
// By default, it sets the mod_policy of all elements to "Admins".
func newApplicationGroup(application Application) (*cb.ConfigGroup, error) {
	applicationGroup, err := newApplicationGroupTemplate(application)
	if err != nil {
		return nil, err
	}

	for _, org := range application.Organizations {
		applicationGroup.Groups[org.Name], err = newOrgConfigGroup(org)
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
