/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

import (
	m "github.com/hyperledger/fabric-protos-go/msp"
)

// Role : Represents a IdemixRole
type Role int32

// The expected roles are 4; We can combine them using a bitmask
const (
	MEMBER Role = 1
	ADMIN  Role = 2
	CLIENT Role = 4
	PEER   Role = 8
	// Next role values: 16, 32, 64 ...
)

func (role Role) getValue() int {
	return int(role)
}

// checkRole Prove that the desired role is contained or not in the bitmask
func checkRole(bitmask int, role Role) bool {
	return (bitmask & role.getValue()) == role.getValue()
}

// getRoleMaskFromIdemixRoles Receive a list of roles to combine in a single bitmask
func getRoleMaskFromIdemixRoles(roles []Role) int {
	mask := 0
	for _, role := range roles {
		mask = mask | role.getValue()
	}
	return mask
}

// GetRoleMaskFromIdemixRole return a bitmask for one role
func GetRoleMaskFromIdemixRole(role Role) int {
	return getRoleMaskFromIdemixRoles([]Role{role})
}

// getIdemixRoleFromMSPRole gets a MSP Role type and returns the integer value
func getIdemixRoleFromMSPRole(role *m.MSPRole) int {
	return getIdemixRoleFromMSPRoleType(role.GetRole())
}

// GetIdemixRoleFromMSPRoleType gets a MSP role type and returns the integer value
func getIdemixRoleFromMSPRoleType(rtype m.MSPRole_MSPRoleType) int {
	return getIdemixRoleFromMSPRoleValue(int(rtype))
}

// getIdemixRoleFromMSPRoleValue Receives a MSP role value and returns the idemix equivalent
func getIdemixRoleFromMSPRoleValue(role int) int {
	switch role {
	case int(m.MSPRole_ADMIN):
		return ADMIN.getValue()
	case int(m.MSPRole_CLIENT):
		return CLIENT.getValue()
	case int(m.MSPRole_MEMBER):
		return MEMBER.getValue()
	case int(m.MSPRole_PEER):
		return PEER.getValue()
	default:
		return MEMBER.getValue()
	}
}
