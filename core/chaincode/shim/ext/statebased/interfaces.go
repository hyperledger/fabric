/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import "fmt"

// RoleType of an endorsement policy's identity
type RoleType string

const (
	// RoleTypeMember identifies an org's member identity
	RoleTypeMember = RoleType("MEMBER")
	// RoleTypePeer identifies an org's peer identity
	RoleTypePeer = RoleType("PEER")
)

// RoleTypeDoesNotExistError is returned by function AddOrgs of
// KeyEndorsementPolicy if a role type that does not match one
// specified above is passed as an argument.
type RoleTypeDoesNotExistError struct {
	RoleType RoleType
}

func (r *RoleTypeDoesNotExistError) Error() string {
	return fmt.Sprintf("role type %s does not exist", r.RoleType)
}

// KeyEndorsementPolicy provides a set of convenience methods to create and
// modify a state-based endorsement policy. Endorsement policies created by
// this convenience layer will always be a logical AND of "<ORG>.peer"
// principals for one or more ORGs specified by the caller.
type KeyEndorsementPolicy interface {
	// Policy returns the endorsement policy as bytes
	Policy() ([]byte, error)

	// AddOrgs adds the specified orgs to the list of orgs that are required
	// to endorse. All orgs MSP role types will be set to the role that is
	// specified in the first parameter. Among other aspects the desired role
	// depends on the channel's configuration: if it supports node OUs, it is
	// likely going to be the PEER role, while the MEMBER role is the suited
	// one if it does not.
	AddOrgs(roleType RoleType, organizations ...string) error

	// DelOrgs deletes the specified channel orgs from the existing key-level endorsement
	// policy for this KVS key.
	DelOrgs(organizations ...string)

	// ListOrgs returns an array of channel orgs that are required to endorse chnages
	ListOrgs() []string
}
