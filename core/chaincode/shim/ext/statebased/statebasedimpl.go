/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"fmt"
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	cb "github.com/hyperledger/fabric/protos/common"
	mb "github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

// stateEP implements the KeyEndorsementPolicy
type stateEP struct {
	orgs map[string]mb.MSPRole_MSPRoleType
}

// NewStateEP constructs a state-based endorsement policy from a given
// serialized EP byte array. If the byte array is empty, a new EP is created.
func NewStateEP(policy []byte) (KeyEndorsementPolicy, error) {
	s := &stateEP{orgs: make(map[string]mb.MSPRole_MSPRoleType)}
	if policy != nil {
		spe := &cb.SignaturePolicyEnvelope{}
		if err := proto.Unmarshal(policy, spe); err != nil {
			return nil, fmt.Errorf("Error unmarshaling to SignaturePolicy: %s", err)
		}

		err := s.setMSPIDsFromSP(spe)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Policy returns the endorsement policy as bytes
func (s *stateEP) Policy() ([]byte, error) {
	spe := s.policyFromMSPIDs()
	spBytes, err := proto.Marshal(spe)
	if err != nil {
		return nil, err
	}
	return spBytes, nil
}

// AddOrgs adds the specified channel orgs to the existing key-level EP
func (s *stateEP) AddOrgs(role RoleType, neworgs ...string) error {
	var mspRole mb.MSPRole_MSPRoleType
	switch role {
	case RoleTypeMember:
		mspRole = mb.MSPRole_MEMBER
	case RoleTypePeer:
		mspRole = mb.MSPRole_PEER
	default:
		return &RoleTypeDoesNotExistError{RoleType: role}
	}

	// add new orgs
	for _, addorg := range neworgs {
		s.orgs[addorg] = mspRole
	}

	return nil
}

// DelOrgs delete the specified channel orgs from the existing key-level EP
func (s *stateEP) DelOrgs(delorgs ...string) {
	for _, delorg := range delorgs {
		delete(s.orgs, delorg)
	}
}

// ListOrgs returns an array of channel orgs that are required to endorse chnages
func (s *stateEP) ListOrgs() []string {
	orgNames := make([]string, 0, len(s.orgs))
	for mspid := range s.orgs {
		orgNames = append(orgNames, mspid)
	}
	return orgNames
}

func (s *stateEP) setMSPIDsFromSP(sp *cb.SignaturePolicyEnvelope) error {
	// iterate over the identities in this envelope
	for _, identity := range sp.Identities {
		// this imlementation only supports the ROLE type
		if identity.PrincipalClassification == mb.MSPPrincipal_ROLE {
			msprole := &mb.MSPRole{}
			err := proto.Unmarshal(identity.Principal, msprole)
			if err != nil {
				return errors.Wrapf(err, "error unmarshaling msp principal")
			}
			s.orgs[msprole.GetMspIdentifier()] = msprole.GetRole()
		}
	}
	return nil
}

func (s *stateEP) policyFromMSPIDs() *cb.SignaturePolicyEnvelope {
	mspids := s.ListOrgs()
	sort.Strings(mspids)
	principals := make([]*mb.MSPPrincipal, len(mspids))
	sigspolicy := make([]*cb.SignaturePolicy, len(mspids))
	for i, id := range mspids {
		principals[i] = &mb.MSPPrincipal{
			PrincipalClassification: mb.MSPPrincipal_ROLE,
			Principal:               utils.MarshalOrPanic(&mb.MSPRole{Role: s.orgs[id], MspIdentifier: id}),
		}
		sigspolicy[i] = cauthdsl.SignedBy(int32(i))
	}

	// create the policy: it requires exactly 1 signature from all of the principals
	p := &cb.SignaturePolicyEnvelope{
		Version:    0,
		Rule:       cauthdsl.NOutOf(int32(len(mspids)), sigspolicy),
		Identities: principals,
	}
	return p
}
