/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// ApplicationOrg retrieves an existing org from an application organization config group.
func (c *ConfigTx) ApplicationOrg(orgName string) (Organization, error) {
	orgGroup, ok := c.original.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return Organization{}, fmt.Errorf("application org %s does not exist in channel config", orgName)
	}

	return getOrganization(orgGroup, orgName)
}

// RemoveApplicationOrg removes an org from the Application group.
// Removal will panic if the application group does not exist.
func (c *ConfigTx) RemoveApplicationOrg(orgName string) {
	applicationGroups := c.updated.ChannelGroup.Groups[ApplicationGroupKey].Groups

	delete(applicationGroups, orgName)

}

// OrdererOrg retrieves an existing org from an orderer organization config group.
func (c *ConfigTx) OrdererOrg(orgName string) (Organization, error) {
	orgGroup, ok := c.original.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
	if !ok {
		return Organization{}, fmt.Errorf("orderer org %s does not exist in channel config", orgName)
	}

	org, err := getOrganization(orgGroup, orgName)
	if err != nil {
		return Organization{}, err
	}

	// Orderer organization requires orderer endpoints.
	endpointsProtos := &cb.OrdererAddresses{}
	err = unmarshalConfigValueAtKey(orgGroup, EndpointsKey, endpointsProtos)
	if err != nil {
		return Organization{}, err
	}
	ordererEndpoints := make([]string, len(endpointsProtos.Addresses))
	for i, address := range endpointsProtos.Addresses {
		ordererEndpoints[i] = address
	}
	org.OrdererEndpoints = ordererEndpoints

	// Remove AnchorPeers which are application org specific.
	org.AnchorPeers = nil

	return org, err
}

// RemoveOrdererOrg removes an org from the Orderer group.
// Removal will panic if the orderer group does not exist.
func (c *ConfigTx) RemoveOrdererOrg(orgName string) {
	ordererGroups := c.updated.ChannelGroup.Groups[OrdererGroupKey].Groups

	delete(ordererGroups, orgName)

}

// ConsortiumOrg retrieves an existing org from a consortium organization config group.
func (c *ConfigTx) ConsortiumOrg(consortiumName, orgName string) (Organization, error) {
	consortium, ok := c.original.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName]
	if !ok {
		return Organization{}, fmt.Errorf("consortium %s does not exist in channel config", consortiumName)
	}
	orgGroup, ok := consortium.Groups[orgName]
	if !ok {
		return Organization{}, fmt.Errorf("consortium org %s does not exist in channel config", orgName)
	}

	org, err := getOrganization(orgGroup, orgName)
	if err != nil {
		return Organization{}, err
	}

	// Remove AnchorPeers which are application org specific.
	org.AnchorPeers = nil

	return org, err
}

// RemoveConsortiumOrg removes an org from a consortium's group.
// Removal will panic if either the consortiums group or consortium group does not exist.
func (c *ConfigTx) RemoveConsortiumOrg(consortiumName, orgName string) {
	consortium := c.updated.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName]

	delete(consortium.Groups, orgName)

}

// newOrgConfigGroup returns an config group for an organization.
// It defines the crypto material for the organization (its MSP).
// It sets the mod_policy of all elements to "Admins".
func newOrgConfigGroup(org Organization) (*cb.ConfigGroup, error) {
	orgGroup := newConfigGroup()
	orgGroup.ModPolicy = AdminsPolicyKey

	if err := setPolicies(orgGroup, org.Policies, AdminsPolicyKey); err != nil {
		return nil, err
	}

	fabricMSPConfig, err := org.MSP.toProto()
	if err != nil {
		return nil, fmt.Errorf("converting fabric msp config to proto: %v", err)
	}

	conf, err := proto.Marshal(fabricMSPConfig)
	if err != nil {
		return nil, fmt.Errorf("marshaling msp config: %v", err)
	}

	// mspConfig defaults type to FABRIC which implements an X.509 based provider
	mspConfig := &mb.MSPConfig{
		Config: conf,
	}

	err = setValue(orgGroup, mspValue(mspConfig), AdminsPolicyKey)
	if err != nil {
		return nil, err
	}

	return orgGroup, nil
}

func newOrdererOrgConfigGroup(org Organization) (*cb.ConfigGroup, error) {
	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return nil, err
	}

	// OrdererEndpoints are orderer org specific and are only added when specified for orderer orgs
	if len(org.OrdererEndpoints) > 0 {
		err := setValue(orgGroup, endpointsValue(org.OrdererEndpoints), AdminsPolicyKey)
		if err != nil {
			return nil, err
		}
	}

	return orgGroup, nil
}

func newApplicationOrgConfigGroup(org Organization) (*cb.ConfigGroup, error) {
	orgGroup, err := newOrgConfigGroup(org)
	if err != nil {
		return nil, err
	}

	// AnchorPeers are application org specific and are only added when specified for application orgs
	anchorProtos := make([]*pb.AnchorPeer, len(org.AnchorPeers))
	for i, anchorPeer := range org.AnchorPeers {
		anchorProtos[i] = &pb.AnchorPeer{
			Host: anchorPeer.Host,
			Port: int32(anchorPeer.Port),
		}
	}

	// Avoid adding an unnecessary anchor peers element when one is not required
	// This helps prevent a delta from the orderer system channel when computing
	// more complex channel creation transactions
	if len(anchorProtos) > 0 {
		err := setValue(orgGroup, anchorPeersValue(anchorProtos), AdminsPolicyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to add anchor peers value: %v", err)
		}
	}

	return orgGroup, nil
}

// getOrganization returns a basic Organization struct from org config group.
func getOrganization(orgGroup *cb.ConfigGroup, orgName string) (Organization, error) {
	policies, err := getPolicies(orgGroup.Policies)
	if err != nil {
		return Organization{}, err
	}

	msp, err := getMSPConfig(orgGroup)
	if err != nil {
		return Organization{}, err
	}

	var anchorPeers []Address
	_, ok := orgGroup.Values[AnchorPeersKey]
	if ok {
		anchorProtos := &pb.AnchorPeers{}
		err = unmarshalConfigValueAtKey(orgGroup, AnchorPeersKey, anchorProtos)
		if err != nil {
			return Organization{}, err
		}

		for _, anchorProto := range anchorProtos.AnchorPeers {
			anchorPeers = append(anchorPeers, Address{
				Host: anchorProto.Host,
				Port: int(anchorProto.Port),
			})
		}
	}

	return Organization{
		Name:        orgName,
		Policies:    policies,
		MSP:         msp,
		AnchorPeers: anchorPeers,
	}, nil
}
