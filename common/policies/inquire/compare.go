/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inquire

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/common/policies"
	"google.golang.org/protobuf/proto"
)

// ComparablePrincipal defines an MSPPrincipal that can be compared to other principals
type ComparablePrincipal struct {
	principal *msp.MSPPrincipal
	ou        *msp.OrganizationUnit
	role      *msp.MSPRole
	mspID     string
	idBytes   []byte
}

// Equal returns whether this ComparablePrincipal is equal to the given ComparablePrincipal.
func (cp *ComparablePrincipal) Equal(cp2 *ComparablePrincipal) bool {
	return cp.mspID == cp2.mspID &&
		cp.equalPrincipals(cp2) && cp.equalRoles(cp2) && cp.equalOUs(cp2)
}

// NewComparablePrincipal creates a ComparablePrincipal out of the given MSPPrincipal.
// Returns nil if a failure occurs.
func NewComparablePrincipal(principal *msp.MSPPrincipal) *ComparablePrincipal {
	if principal == nil {
		logger.Warning("Principal is nil")
		return nil
	}
	cp := &ComparablePrincipal{
		principal: principal,
	}
	switch principal.GetPrincipalClassification() {
	case msp.MSPPrincipal_ROLE:
		return cp.ToRole()
	case msp.MSPPrincipal_ORGANIZATION_UNIT:
		return cp.ToOURole()
	case msp.MSPPrincipal_IDENTITY:
		return cp.ToIdentity()
	}
	mapping := msp.MSPPrincipal_Classification_name[int32(principal.GetPrincipalClassification())]
	logger.Warning("Received an unsupported principal type:", principal.GetPrincipalClassification(), "mapped to", mapping)
	return nil
}

// IsFound returns whether the ComparablePrincipal is found among the given set of ComparablePrincipals
// For the ComparablePrincipal x to be found, there needs to be some ComparablePrincipal y in the set
// such that x.IsA(y) will be true.
func (cp *ComparablePrincipal) IsFound(set ...*ComparablePrincipal) bool {
	return slices.ContainsFunc(set, cp.IsA)
}

// IsA determines whether all identities that satisfy this ComparablePrincipal
// also satisfy the other ComparablePrincipal.
// Example: if this ComparablePrincipal is a Peer role,
// and the other ComparablePrincipal is a Member role, then
// all identities that satisfy this ComparablePrincipal (are peers)
// also satisfy the other principal (are members).
func (cp *ComparablePrincipal) IsA(other *ComparablePrincipal) bool {
	this := cp

	if other == nil {
		return false
	}
	if this.principal == nil || other.principal == nil {
		logger.Warning("Used an un-initialized ComparablePrincipal")
		return false
	}
	// Compare the MSP ID
	if this.mspID != other.mspID {
		return false
	}

	// If the other Principal is a member, then any role or OU role
	// fits, because every role or OU role is also a member of the MSP
	if other.role != nil && other.role.GetRole() == msp.MSPRole_MEMBER {
		return true
	}

	// Check if we're both OU roles
	if this.ou != nil && other.ou != nil {
		sameOU := this.ou.GetOrganizationalUnitIdentifier() == other.ou.GetOrganizationalUnitIdentifier()
		sameIssuer := bytes.Equal(this.ou.GetCertifiersIdentifier(), other.ou.GetCertifiersIdentifier())
		return sameOU && sameIssuer
	}

	// Check if we're both the same MSP Role
	if this.role != nil && other.role != nil {
		return this.role.GetRole() == other.role.GetRole()
	}

	if this.principal.GetPrincipalClassification() == msp.MSPPrincipal_IDENTITY {
		return bytes.Equal(this.idBytes, other.idBytes) && this.mspID == other.mspID
	}

	// Else, we can't say anything, because we have no knowledge
	// about the OUs that make up the MSP roles - so return false
	return false
}

// ToOURole converts this ComparablePrincipal to OU principal, and returns nil on failure
func (cp *ComparablePrincipal) ToOURole() *ComparablePrincipal {
	ouRole := &msp.OrganizationUnit{}
	err := proto.Unmarshal(cp.principal.GetPrincipal(), ouRole)
	if err != nil {
		logger.Warning("Failed unmarshalling principal:", err)
		return nil
	}
	cp.mspID = ouRole.GetMspIdentifier()
	cp.ou = ouRole
	return cp
}

// ToIdentity converts this ComparablePrincipal to Identity principal, and returns nil on failure
func (cp *ComparablePrincipal) ToIdentity() *ComparablePrincipal {
	sID := &msp.SerializedIdentity{}
	err := proto.Unmarshal(cp.principal.GetPrincipal(), sID)
	if err != nil {
		logger.Warning("Failed unmarshalling principal:", err)
		return nil
	}
	cp.mspID = sID.GetMspid()
	cp.idBytes = sID.GetIdBytes()
	return cp
}

// ToRole converts this ComparablePrincipal to MSP Role, and returns nil if the conversion failed
func (cp *ComparablePrincipal) ToRole() *ComparablePrincipal {
	mspRole := &msp.MSPRole{}
	err := proto.Unmarshal(cp.principal.GetPrincipal(), mspRole)
	if err != nil {
		logger.Warning("Failed unmarshalling principal:", err)
		return nil
	}
	cp.mspID = mspRole.GetMspIdentifier()
	cp.role = mspRole
	return cp
}

// ComparablePrincipalSet aggregates ComparablePrincipals
type ComparablePrincipalSet []*ComparablePrincipal

// ToPrincipalSet converts this ComparablePrincipalSet to a PrincipalSet
func (cps ComparablePrincipalSet) ToPrincipalSet() policies.PrincipalSet {
	var res policies.PrincipalSet
	for _, cp := range cps {
		res = append(res, cp.principal)
	}
	return res
}

// String returns a string representation of this ComparablePrincipalSet
func (cps ComparablePrincipalSet) String() string {
	buff := bytes.Buffer{}
	buff.WriteString("[")
	for i, cp := range cps {
		buff.WriteString(cp.mspID)
		buff.WriteString(".")
		if cp.role != nil {
			buff.WriteString(fmt.Sprintf("%v", cp.role.GetRole()))
		}
		if cp.ou != nil {
			buff.WriteString(fmt.Sprintf("%v", cp.ou.GetOrganizationalUnitIdentifier()))
		}
		if i < len(cps)-1 {
			buff.WriteString(", ")
		}
	}
	buff.WriteString("]")
	return buff.String()
}

// NewComparablePrincipalSet constructs a ComparablePrincipalSet out of the given PrincipalSet
func NewComparablePrincipalSet(set policies.PrincipalSet) ComparablePrincipalSet {
	var res ComparablePrincipalSet
	for _, principal := range set {
		cp := NewComparablePrincipal(principal)
		if cp == nil {
			return nil
		}
		res = append(res, cp)
	}
	return res
}

// Clone returns a copy of this ComparablePrincipalSet
func (cps ComparablePrincipalSet) Clone() ComparablePrincipalSet {
	res := make(ComparablePrincipalSet, len(cps))
	copy(res, cps)
	return res
}

func (cp *ComparablePrincipal) equalRoles(cp2 *ComparablePrincipal) bool {
	if cp.role == nil && cp2.role == nil {
		return true
	}

	if cp.role == nil || cp2.role == nil {
		return false
	}

	return cp.role.GetMspIdentifier() == cp2.role.GetMspIdentifier() &&
		cp.role.GetRole() == cp2.role.GetRole()
}

func (cp *ComparablePrincipal) equalOUs(cp2 *ComparablePrincipal) bool {
	if cp.ou == nil && cp2.ou == nil {
		return true
	}

	if cp.ou == nil || cp2.ou == nil {
		return false
	}

	if cp.ou.GetOrganizationalUnitIdentifier() != cp2.ou.GetOrganizationalUnitIdentifier() {
		return false
	}

	if cp.ou.GetMspIdentifier() != cp2.ou.GetMspIdentifier() {
		return false
	}

	return bytes.Equal(cp.ou.GetCertifiersIdentifier(), cp2.ou.GetCertifiersIdentifier())
}

func (cp *ComparablePrincipal) equalPrincipals(cp2 *ComparablePrincipal) bool {
	if cp.principal == nil && cp2.principal == nil {
		return true
	}

	if cp.principal == nil || cp2.principal == nil {
		return false
	}

	if cp.principal.GetPrincipalClassification() != cp2.principal.GetPrincipalClassification() {
		return false
	}

	return bytes.Equal(cp.principal.GetPrincipal(), cp2.principal.GetPrincipal())
}
