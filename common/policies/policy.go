/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/flogging"
	mspi "github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	// Path separator is used to separate policy names in paths
	PathSeparator = "/"

	// ChannelPrefix is used in the path of standard channel policy managers
	ChannelPrefix = "Channel"

	// ApplicationPrefix is used in the path of standard application policy paths
	ApplicationPrefix = "Application"

	// OrdererPrefix is used in the path of standard orderer policy paths
	OrdererPrefix = "Orderer"

	// ChannelReaders is the label for the channel's readers policy (encompassing both orderer and application readers)
	ChannelReaders = PathSeparator + ChannelPrefix + PathSeparator + "Readers"

	// ChannelWriters is the label for the channel's writers policy (encompassing both orderer and application writers)
	ChannelWriters = PathSeparator + ChannelPrefix + PathSeparator + "Writers"

	// ChannelApplicationReaders is the label for the channel's application readers policy
	ChannelApplicationReaders = PathSeparator + ChannelPrefix + PathSeparator + ApplicationPrefix + PathSeparator + "Readers"

	// ChannelApplicationWriters is the label for the channel's application writers policy
	ChannelApplicationWriters = PathSeparator + ChannelPrefix + PathSeparator + ApplicationPrefix + PathSeparator + "Writers"

	// ChannelApplicationAdmins is the label for the channel's application admin policy
	ChannelApplicationAdmins = PathSeparator + ChannelPrefix + PathSeparator + ApplicationPrefix + PathSeparator + "Admins"

	// BlockValidation is the label for the policy which should validate the block signatures for the channel
	BlockValidation = PathSeparator + ChannelPrefix + PathSeparator + OrdererPrefix + PathSeparator + "BlockValidation"

	// ChannelOrdererAdmins is the label for the channel's orderer admin policy
	ChannelOrdererAdmins = PathSeparator + ChannelPrefix + PathSeparator + OrdererPrefix + PathSeparator + "Admins"

	// ChannelOrdererWriters is the label for the channel's orderer writers policy
	ChannelOrdererWriters = PathSeparator + ChannelPrefix + PathSeparator + OrdererPrefix + PathSeparator + "Writers"

	// ChannelOrdererReaders is the label for the channel's orderer readers policy
	ChannelOrdererReaders = PathSeparator + ChannelPrefix + PathSeparator + OrdererPrefix + PathSeparator + "Readers"
)

var logger = flogging.MustGetLogger("policies")

// PrincipalSet is a collection of MSPPrincipals
type PrincipalSet []*msp.MSPPrincipal

// PrincipalSets aggregates PrincipalSets
type PrincipalSets []PrincipalSet

// ContainingOnly returns PrincipalSets that contain only principals of the given predicate
func (psSets PrincipalSets) ContainingOnly(f func(*msp.MSPPrincipal) bool) PrincipalSets {
	var res PrincipalSets
	for _, set := range psSets {
		if !set.ContainingOnly(f) {
			continue
		}
		res = append(res, set)
	}
	return res
}

// ContainingOnly returns whether the given PrincipalSet contains only Principals
// that satisfy the given predicate
func (ps PrincipalSet) ContainingOnly(f func(*msp.MSPPrincipal) bool) bool {
	for _, principal := range ps {
		if !f(principal) {
			return false
		}
	}
	return true
}

// UniqueSet returns a histogram that is induced by the PrincipalSet
func (ps PrincipalSet) UniqueSet() map[*msp.MSPPrincipal]int {
	// Create a histogram that holds the MSPPrincipals and counts them
	histogram := make(map[struct {
		cls       int32
		principal string
	}]int)
	// Now, populate the histogram
	for _, principal := range ps {
		key := struct {
			cls       int32
			principal string
		}{
			cls:       int32(principal.PrincipalClassification),
			principal: string(principal.Principal),
		}
		histogram[key]++
	}
	// Finally, convert to a histogram of MSPPrincipal pointers
	res := make(map[*msp.MSPPrincipal]int)
	for principal, count := range histogram {
		res[&msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_Classification(principal.cls),
			Principal:               []byte(principal.principal),
		}] = count
	}
	return res
}

// Converter represents a policy
// which may be translated into a SignaturePolicyEnvelope
type Converter interface {
	Convert() (*cb.SignaturePolicyEnvelope, error)
}

// Policy is used to determine if a signature is valid
type Policy interface {
	// EvaluateSignedData takes a set of SignedData and evaluates whether
	// 1) the signatures are valid over the related message
	// 2) the signing identities satisfy the policy
	EvaluateSignedData(signatureSet []*protoutil.SignedData) error

	// EvaluateIdentities takes an array of identities and evaluates whether
	// they satisfy the policy
	EvaluateIdentities(identities []mspi.Identity) error
}

// InquireablePolicy is a Policy that one can inquire
type InquireablePolicy interface {
	// SatisfiedBy returns a slice of PrincipalSets that each of them
	// satisfies the policy.
	SatisfiedBy() []PrincipalSet
}

// Manager is a read only subset of the policy ManagerImpl
type Manager interface {
	// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default policy
	GetPolicy(id string) (Policy, bool)

	// Manager returns the sub-policy manager for a given path and whether it exists
	Manager(path []string) (Manager, bool)
}

// Provider provides the backing implementation of a policy
type Provider interface {
	// NewPolicy creates a new policy based on the policy bytes
	NewPolicy(data []byte) (Policy, proto.Message, error)
}

// ChannelPolicyManagerGetter is a support interface
// to get access to the policy manager of a given channel
type ChannelPolicyManagerGetter interface {
	// Returns the policy manager associated with the specified channel.
	Manager(channelID string) Manager
}

// PolicyManagerGetterFunc is a function adapater for ChannelPolicyManagerGetter.
type PolicyManagerGetterFunc func(channelID string) Manager

func (p PolicyManagerGetterFunc) Manager(channelID string) Manager { return p(channelID) }

// ManagerImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.ConfigManager
type ManagerImpl struct {
	path     string // The group level path
	Policies map[string]Policy
	managers map[string]*ManagerImpl
}

// NewManagerImpl creates a new ManagerImpl with the given CryptoHelper
func NewManagerImpl(path string, providers map[int32]Provider, root *cb.ConfigGroup) (*ManagerImpl, error) {
	var err error
	_, ok := providers[int32(cb.Policy_IMPLICIT_META)]
	if ok {
		logger.Panicf("ImplicitMetaPolicy type must be provider by the policy manager")
	}

	managers := make(map[string]*ManagerImpl)

	for groupName, group := range root.Groups {
		managers[groupName], err = NewManagerImpl(path+PathSeparator+groupName, providers, group)
		if err != nil {
			return nil, err
		}
	}

	policies := make(map[string]Policy)
	for policyName, configPolicy := range root.Policies {
		policy := configPolicy.Policy
		if policy == nil {
			return nil, fmt.Errorf("policy %s at path %s was nil", policyName, path)
		}

		var cPolicy Policy

		if policy.Type == int32(cb.Policy_IMPLICIT_META) {
			imp, err := NewImplicitMetaPolicy(policy.Value, managers)
			if err != nil {
				return nil, errors.Wrapf(err, "implicit policy %s at path %s did not compile", policyName, path)
			}
			cPolicy = imp
		} else {
			provider, ok := providers[int32(policy.Type)]
			if !ok {
				return nil, fmt.Errorf("policy %s at path %s has unknown policy type: %v", policyName, path, policy.Type)
			}

			var err error
			cPolicy, _, err = provider.NewPolicy(policy.Value)
			if err != nil {
				return nil, errors.Wrapf(err, "policy %s at path %s did not compile", policyName, path)
			}
		}

		policies[policyName] = cPolicy

		logger.Debugf("Proposed new policy %s for %s", policyName, path)
	}

	for groupName, manager := range managers {
		for policyName, policy := range manager.Policies {
			policies[groupName+PathSeparator+policyName] = policy
		}
	}

	return &ManagerImpl{
		path:     path,
		Policies: policies,
		managers: managers,
	}, nil
}

type rejectPolicy string

func (rp rejectPolicy) EvaluateSignedData(signedData []*protoutil.SignedData) error {
	return errors.Errorf("no such policy: '%s'", rp)
}

func (rp rejectPolicy) EvaluateIdentities(identities []mspi.Identity) error {
	return errors.Errorf("no such policy: '%s'", rp)
}

// Manager returns the sub-policy manager for a given path and whether it exists
func (pm *ManagerImpl) Manager(path []string) (Manager, bool) {
	logger.Debugf("Manager %s looking up path %v", pm.path, path)
	for manager := range pm.managers {
		logger.Debugf("Manager %s has managers %s", pm.path, manager)
	}
	if len(path) == 0 {
		return pm, true
	}

	m, ok := pm.managers[path[0]]
	if !ok {
		return nil, false
	}

	return m.Manager(path[1:])
}

type PolicyLogger struct {
	Policy     Policy
	policyName string
}

func (pl *PolicyLogger) EvaluateSignedData(signatureSet []*protoutil.SignedData) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("== Evaluating %T Policy %s ==", pl.Policy, pl.policyName)
		defer logger.Debugf("== Done Evaluating %T Policy %s", pl.Policy, pl.policyName)
	}

	err := pl.Policy.EvaluateSignedData(signatureSet)
	if err != nil {
		logger.Debugf("Signature set did not satisfy policy %s", pl.policyName)
	} else {
		logger.Debugf("Signature set satisfies policy %s", pl.policyName)
	}
	return err
}

func (pl *PolicyLogger) EvaluateIdentities(identities []mspi.Identity) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("== Evaluating %T Policy %s ==", pl.Policy, pl.policyName)
		defer logger.Debugf("== Done Evaluating %T Policy %s", pl.Policy, pl.policyName)
	}

	err := pl.Policy.EvaluateIdentities(identities)
	if err != nil {
		logger.Debugf("Signature set did not satisfy policy %s", pl.policyName)
	} else {
		logger.Debugf("Signature set satisfies policy %s", pl.policyName)
	}
	return err
}

func (pl *PolicyLogger) Convert() (*cb.SignaturePolicyEnvelope, error) {
	logger.Debugf("== Converting %T Policy %s ==", pl.Policy, pl.policyName)

	convertiblePolicy, ok := pl.Policy.(Converter)
	if !ok {
		logger.Errorf("policy (name='%s',type='%T') is not convertible to SignaturePolicyEnvelope", pl.policyName, pl.Policy)
		return nil, errors.Errorf("policy (name='%s',type='%T') is not convertible to SignaturePolicyEnvelope", pl.policyName, pl.Policy)
	}

	cp, err := convertiblePolicy.Convert()
	if err != nil {
		logger.Errorf("== Error Converting %T Policy %s, err %s", pl.Policy, pl.policyName, err.Error())
	} else {
		logger.Debugf("== Done Converting %T Policy %s", pl.Policy, pl.policyName)
	}

	return cp, err
}

// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default reject policy
func (pm *ManagerImpl) GetPolicy(id string) (Policy, bool) {
	if id == "" {
		logger.Errorf("Returning dummy reject all policy because no policy ID supplied")
		return rejectPolicy(id), false
	}
	var relpath string

	if strings.HasPrefix(id, PathSeparator) {
		if !strings.HasPrefix(id, PathSeparator+pm.path) {
			logger.Debugf("Requested absolute policy %s from %s, returning rejectAll", id, pm.path)
			return rejectPolicy(id), false
		}
		// strip off the leading slash, the path, and the trailing slash
		relpath = id[1+len(pm.path)+1:]
	} else {
		relpath = id
	}

	policy, ok := pm.Policies[relpath]
	if !ok {
		logger.Debugf("Returning dummy reject all policy because %s could not be found in %s/%s", id, pm.path, relpath)
		return rejectPolicy(relpath), false
	}

	return &PolicyLogger{
		Policy:     policy,
		policyName: PathSeparator + pm.path + PathSeparator + relpath,
	}, true
}

// SignatureSetToValidIdentities takes a slice of pointers to signed data,
// checks the validity of the signature and of the signer and returns a
// slice of associated identities. The returned identities are deduplicated.
func SignatureSetToValidIdentities(signedData []*protoutil.SignedData, identityDeserializer mspi.IdentityDeserializer) []mspi.Identity {
	idMap := map[string]struct{}{}
	identities := make([]mspi.Identity, 0, len(signedData))

	for i, sd := range signedData {
		identity, err := identityDeserializer.DeserializeIdentity(sd.Identity)
		if err != nil {
			logger.Warnw("invalid identity", "error", err.Error(), "identity", protoutil.LogMessageForSerializedIdentity(sd.Identity))
			continue
		}

		key := identity.GetIdentifier().Mspid + identity.GetIdentifier().Id

		// We check if this identity has already appeared before doing a signature check, to ensure that
		// someone cannot force us to waste time checking the same signature thousands of times
		if _, ok := idMap[key]; ok {
			logger.Warningf("De-duplicating identity [%s] at index %d in signature set", key, i)
			continue
		}

		err = identity.Verify(sd.Data, sd.Signature)
		if err != nil {
			logger.Warningf("signature for identity %d is invalid: %s", i, err)
			continue
		}
		logger.Debugf("signature for identity %d validated", i)

		idMap[key] = struct{}{}
		identities = append(identities, identity)
	}

	return identities
}
