/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"crypto/x509"
	"crypto/x509/pkix"
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

type ApplicationGroup struct {
	applicationGroup *cb.ConfigGroup
}

type UpdatedApplicationGroup struct {
	*ApplicationGroup
}

type ApplicationOrg struct {
	orgGroup *cb.ConfigGroup
	name     string
}

type UpdatedApplicationOrg struct {
	*ApplicationOrg
	name string
}

func (c *Config) Application() *ApplicationGroup {
	applicationGroup := c.ChannelGroup.Groups[ApplicationGroupKey]
	return &ApplicationGroup{applicationGroup: applicationGroup}
}

func (u *UpdatedConfig) Application() *UpdatedApplicationGroup {
	applicationGroup := u.ChannelGroup.Groups[ApplicationGroupKey]
	return &UpdatedApplicationGroup{ApplicationGroup: &ApplicationGroup{applicationGroup: applicationGroup}}
}

func (a *ApplicationGroup) Organization(name string) *ApplicationOrg {
	organizationGroup, ok := a.applicationGroup.Groups[name]
	if !ok {
		return nil
	}
	return &ApplicationOrg{name: name, orgGroup: organizationGroup}
}

func (u *UpdatedApplicationGroup) Organization(name string) *UpdatedApplicationOrg {
	return &UpdatedApplicationOrg{ApplicationOrg: u.ApplicationGroup.Organization(name)}
}

// SetOrganization sets the organization config group for the given application
// org key in an existing Application configuration's Groups map.
// If the application org already exists in the current configuration, its value will be overwritten.
func (u *UpdatedApplicationGroup) SetOrganization(org Organization) error {
	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return fmt.Errorf("failed to create application org %s: %v", org.Name, err)
	}

	u.applicationGroup.Groups[org.Name] = orgGroup

	return nil
}

// RemoveOrganization removes an org from the Application group.
// Removal will panic if the application group does not exist.
func (u *UpdatedApplicationGroup) RemoveOrganization(orgName string) {
	delete(u.applicationGroup.Groups, orgName)
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

func (a *ApplicationOrg) Configuration() (Organization, error) {
	org, err := getOrganization(a.orgGroup, a.name)
	if err != nil {
		return Organization{}, err
	}
	return org, nil
}

func (u *UpdatedApplicationOrg) Configuration() (Organization, error) {
	return u.ApplicationOrg.Configuration()
}

// Capabilities returns a map of enabled application capabilities
// from the original config.
func (a *ApplicationGroup) Capabilities() ([]string, error) {
	capabilities, err := getCapabilities(a.applicationGroup)
	if err != nil {
		return nil, fmt.Errorf("retrieving application capabilities: %v", err)
	}

	return capabilities, nil
}

// Capabilities returns a map of enabled application capabilities
// from the updated config.
func (u *UpdatedApplicationGroup) Capabilities() ([]string, error) {
	return u.ApplicationGroup.Capabilities()
}

// AddCapability sets capability to the provided channel config.
// If the provided capability already exist in current configuration, this action
// will be a no-op.
func (u *UpdatedApplicationGroup) AddCapability(capability string) error {
	capabilities, err := u.Capabilities()
	if err != nil {
		return err
	}

	err = addCapability(u.applicationGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// RemoveCapability removes capability to the provided channel config.
func (u *UpdatedApplicationGroup) RemoveCapability(capability string) error {
	capabilities, err := u.Capabilities()
	if err != nil {
		return err
	}

	err = removeCapability(u.applicationGroup, capabilities, AdminsPolicyKey, capability)
	if err != nil {
		return err
	}

	return nil
}

// Policies returns a map of policies for application config group.
func (a *ApplicationGroup) Policies() (map[string]Policy, error) {
	return getPolicies(a.applicationGroup.Policies)
}

func (u *UpdatedApplicationGroup) Policies() (map[string]Policy, error) {
	return u.ApplicationGroup.Policies()
}

// SetPolicy sets the specified policy in the application group's config policy map.
// If the policy already exist in current configuration, its value will be overwritten.
func (u *UpdatedApplicationGroup) SetPolicy(modPolicy, policyName string, policy Policy) error {
	err := setPolicy(u.applicationGroup, modPolicy, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// RemovePolicy removes an existing policy from an application's configuration.
// Removal will panic if the application group does not exist.
func (u *UpdatedApplicationGroup) RemovePolicy(policyName string) error {
	policies, err := u.Policies()
	if err != nil {
		return err
	}

	removePolicy(u.applicationGroup, policyName, policies)
	return nil
}

// Policies returns a map of policies for a specific application org.
func (a *ApplicationOrg) Policies() (map[string]Policy, error) {
	return getPolicies(a.orgGroup.Policies)
}

func (u *UpdatedApplicationOrg) Policies() (map[string]Policy, error) {
	return u.ApplicationOrg.Policies()
}

// SetPolicy sets the specified policy in the application org group's config policy map.
// If an Organization policy already exist in current configuration, its value will be overwritten.
func (u *UpdatedApplicationOrg) SetPolicy(modPolicy, policyName string, policy Policy) error {
	err := setPolicy(u.orgGroup, modPolicy, policyName, policy)
	if err != nil {
		return fmt.Errorf("failed to set policy '%s': %v", policyName, err)
	}

	return nil
}

// RemovePolicy removes an existing policy from an application organization.
func (u *UpdatedApplicationOrg) RemovePolicy(policyName string) error {
	policies, err := u.Policies()
	if err != nil {
		return err
	}

	removePolicy(u.orgGroup, policyName, policies)
	return nil
}

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

func (u *UpdatedApplicationOrg) AnchorPeers() ([]Address, error) {
	return u.ApplicationOrg.AnchorPeers()
}

func (u *UpdatedApplicationOrg) AddAnchorPeer(newAnchorPeer Address) error {
	anchorPeersProto := &pb.AnchorPeers{}

	if anchorPeerConfigValue, ok := u.ApplicationOrg.orgGroup.Values[AnchorPeersKey]; ok {
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
	err := setValue(u.orgGroup, anchorPeersValue(anchorProtos), AdminsPolicyKey)
	if err != nil {
		return err
	}
	return nil
}

func (u *UpdatedApplicationOrg) RemoveAnchorPeer(anchorPeerToRemove Address) error {
	anchorPeersProto := &pb.AnchorPeers{}

	if anchorPeerConfigValue, ok := u.ApplicationOrg.orgGroup.Values[AnchorPeersKey]; ok {
		// Unmarshal existing anchor peers if the config value exists
		err := proto.Unmarshal(anchorPeerConfigValue.Value, anchorPeersProto)
		if err != nil {
			return fmt.Errorf("failed unmarshaling anchor peer endpoints for application org %s: %v", u.ApplicationOrg.name, err)
		}
	}

	existingAnchorPeers := anchorPeersProto.AnchorPeers[:0]
	for _, anchorPeer := range anchorPeersProto.AnchorPeers {
		if anchorPeer.Host != anchorPeerToRemove.Host || anchorPeer.Port != int32(anchorPeerToRemove.Port) {
			existingAnchorPeers = append(existingAnchorPeers, anchorPeer)

			// Add anchor peers config value back to application org
			err := setValue(u.orgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
			if err != nil {
				return fmt.Errorf("failed to remove anchor peer %v from org %s: %v", anchorPeerToRemove, u.ApplicationOrg.name, err)
			}

			return nil
		}
	}

	if len(existingAnchorPeers) == len(anchorPeersProto.AnchorPeers) {
		return fmt.Errorf("could not find anchor peer %s:%d in application org %s", anchorPeerToRemove.Host, anchorPeerToRemove.Port, u.name)
	}

	// Add anchor peers config value back to application org
	err := setValue(u.orgGroup, anchorPeersValue(existingAnchorPeers), AdminsPolicyKey)
	if err != nil {
		return fmt.Errorf("failed to remove anchor peer %v from org %s: %v", anchorPeerToRemove, u.name, err)
	}

	return nil
}

// ACLs returns a map of ACLS for given config application.
func (a *ApplicationGroup) ACLs() (map[string]string, error) {
	aclProtos := &pb.ACLs{}

	err := unmarshalConfigValueAtKey(a.applicationGroup, ACLsKey, aclProtos)
	if err != nil {
		return nil, err
	}

	retACLs := map[string]string{}
	for apiResource, policyRef := range aclProtos.Acls {
		retACLs[apiResource] = policyRef.PolicyRef
	}

	return retACLs, nil
}

// ACLs returns a map of ACLS for given config application.
func (u *UpdatedApplicationGroup) ACLs() (map[string]string, error) {
	return u.ApplicationGroup.ACLs()
}

// SetACLs sets ACLS to an existing channel config application.
// If an ACL already exist in current configuration, it will be replaced with new ACL.
func (u *UpdatedApplicationGroup) SetACLs(acls map[string]string) error {
	err := setValue(u.applicationGroup, aclValues(acls), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// RemoveACLs a list of ACLs from given channel config application.
// Specifying acls that do not exist in the application ConfigGroup of the channel config will not return a error.
// Removal will panic if application group does not exist.
func (u *UpdatedApplicationGroup) RemoveACLs(acls []string) error {
	configACLs, err := u.ACLs()
	if err != nil {
		return err
	}

	for _, acl := range acls {
		delete(configACLs, acl)
	}

	err = setValue(u.applicationGroup, aclValues(configACLs), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// MSP returns the MSP configuration for an existing application
// org in the original config of a config transaction.
func (a *ApplicationOrg) MSP() (MSP, error) {
	return getMSPConfig(a.orgGroup)
}

// MSP returns the MSP configuration for an existing application
// org in the updated config of a config transaction.
func (u *UpdatedApplicationOrg) MSP() (MSP, error) {
	return getMSPConfig(u.orgGroup)
}

// SetMSP updates the MSP config for the specified application
// org group.
func (u *UpdatedApplicationOrg) SetMSP(updatedMSP MSP) error {
	currentMSP, err := u.MSP()
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

	err = u.setMSPConfig(updatedMSP)
	if err != nil {
		return err
	}

	return nil
}

func (u *UpdatedApplicationOrg) setMSPConfig(updatedMSP MSP) error {
	mspConfig, err := newMSPConfig(updatedMSP)
	if err != nil {
		return fmt.Errorf("new msp config: %v", err)
	}

	err = setValue(u.ApplicationOrg.orgGroup, mspValue(mspConfig), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

// CreateMSPCRL creates a CRL that revokes the provided certificates
// for the specified application org signed by the provided SigningIdentity.
func (u *UpdatedApplicationOrg) CreateMSPCRL(signingIdentity *SigningIdentity, certs ...*x509.Certificate) (*pkix.CertificateList, error) {
	msp, err := u.MSP()
	if err != nil {
		return nil, fmt.Errorf("retrieving application org msp: %s", err)
	}

	return msp.newMSPCRL(signingIdentity, certs...)
}

// newApplicationGroup returns the application component of the channel configuration.
// By default, it sets the mod_policy of all elements to "Admins".
func newApplicationGroup(application Application) (*cb.ConfigGroup, error) {
	var err error

	applicationGroup := newConfigGroup()
	applicationGroup.ModPolicy = AdminsPolicyKey

	if err = setPolicies(applicationGroup, application.Policies, AdminsPolicyKey); err != nil {
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
