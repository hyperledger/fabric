/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bddtests

import (
	"fmt"

	pb "github.com/hyperledger/fabric/protos/peer"
)

// UserRegistration represents a user in the peer network
type UserRegistration struct {
	enrollID   string
	secret     string
	tags       map[string]interface{}
	lastResult interface{}
}

func (u *UserRegistration) hasTag(tagName string) bool {
	return u.tags[tagName] != nil
}

// GetTagValue returns a tag value for the user given the supplied tag name
func (u *UserRegistration) GetTagValue(tagName string) (interface{}, error) {
	if result, ok := u.tags[tagName]; ok == true {
		return result, nil
	}
	return nil, fmt.Errorf("User '%s' does not have value for tagname '%s'", u.enrollID, tagName)
}

// SetTagValue sets the tag value for the user given the supplied tag name. Fails if tag name already used.
func (u *UserRegistration) SetTagValue(tagName string, tagValue interface{}) (interface{}, error) {
	if _, ok := u.tags[tagName]; ok == true {
		return nil, fmt.Errorf("User '%s' already has value for tagname '%s'", u.enrollID, tagName)
	}
	u.tags[tagName] = tagValue
	return tagValue, nil
}

// GetChaincodeSpec returns a chaincode spec for the supplied tag name (uses type assertion).  Fails if not of expected type.
func (u *UserRegistration) GetChaincodeSpec(tagName string) (ccSpec *pb.ChaincodeSpec, err error) {
	errRetFunc := func() error {
		return fmt.Errorf("Error getting chaincodeSpec '%s' for User '%s': '%s'", tagName, u.enrollID, err)
	}
	var value interface{}
	var ok bool
	if value, err = u.GetTagValue(tagName); err != nil {
		return nil, errRetFunc()
	}
	// Now assert value is pointer to chaincodeSpec
	if ccSpec, ok = value.(*pb.ChaincodeSpec); ok != true {
		err = fmt.Errorf("Unexpected type found for tag '%s', found '%v'", tagName, value)
		return nil, errRetFunc()
	}
	return ccSpec, err
}

// GetProposal returns a Proposal for the supplied tag name (uses type assertion).  Fails if not of expected type.
func (u *UserRegistration) GetProposal(tagName string) (result *pb.Proposal, err error) {
	errRetFunc := func() error {
		return fmt.Errorf("Error getting Proposal '%s' for User '%s': '%s'", tagName, u.enrollID, err)
	}
	var value interface{}
	var ok bool
	if value, err = u.GetTagValue(tagName); err != nil {
		return nil, errRetFunc()
	}
	// Now assert value is pointer to Proposal
	if result, ok = value.(*pb.Proposal); ok != true {
		err = fmt.Errorf("Unexpected type found for tag '%s', found '%v'", tagName, value)
		return nil, errRetFunc()
	}
	return result, err
}

// GetChaincodeDeploymentSpec returns a ChaincodeDeploymentSpec for the supplied tag name (uses type assertion).  Fails if not of expected type.
func (u *UserRegistration) GetChaincodeDeploymentSpec(tagName string) (spec *pb.ChaincodeDeploymentSpec, err error) {
	errRetFunc := func() error {
		return fmt.Errorf("Error getting ChaincodeDeploymentSpec '%s' for User '%s': '%s'", tagName, u.enrollID, err)
	}
	var value interface{}
	var ok bool
	if value, err = u.GetTagValue(tagName); err != nil {
		return nil, errRetFunc()
	}
	// Now assert value is pointer to ChaincodeDeploymentSpec
	if spec, ok = value.(*pb.ChaincodeDeploymentSpec); ok != true {
		err = fmt.Errorf("Unexpected type found for tag '%s', found '%v'", tagName, value)
		return nil, errRetFunc()
	}
	return spec, err
}

// GetKeyedProposalResponseDict returns a KeyedProposalResponseMap for the supplied tag name (uses type assertion).  Fails if not of expected type.
func (u *UserRegistration) GetKeyedProposalResponseDict(tagName string) (result KeyedProposalResponseMap, err error) {
	var value interface{}
	var ok bool
	errRetFunc := func() error {
		return fmt.Errorf("Error getting keyed Proposal Responses dictionary '%s' for User '%s': '%s'", tagName, u.enrollID, err)
	}
	if value, err = u.GetTagValue(tagName); err != nil {
		return nil, errRetFunc()
	}
	// Now assert value is pointer to chaincodeSpec
	if result, ok = value.(KeyedProposalResponseMap); ok != true {
		err = fmt.Errorf("Unexpected type found for tag '%s', found '%v'", tagName, value)
		return nil, errRetFunc()
	}
	return result, err
}

func (b *BDDContext) registerUser(enrollID, secret string) error {
	if b.users[enrollID] != nil {
		return fmt.Errorf("User already registered (%s)", enrollID)
	}
	b.users[enrollID] = &UserRegistration{enrollID: enrollID, secret: secret, tags: make(map[string]interface{})}
	return nil
}

// GetUserRegistration return the UserRegistration for a given enrollID
func (b *BDDContext) GetUserRegistration(enrollID string) (*UserRegistration, error) {
	if b.users[enrollID] == nil {
		return nil, fmt.Errorf("User not registered (%s)", enrollID)
	}
	return b.users[enrollID], nil
}
