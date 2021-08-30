/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsement

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/peer"

	"github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/graph"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/policies/inquire"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	gossipdiscovery "github.com/hyperledger/fabric/gossip/discovery"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("discovery.endorsement")

type principalEvaluator interface {
	// SatisfiesPrincipal returns whether a given peer identity satisfies a certain principal
	// on a given channel
	SatisfiesPrincipal(channel string, identity []byte, principal *msp.MSPPrincipal) error
}

type chaincodeMetadataFetcher interface {
	// ChaincodeMetadata returns the metadata of the chaincode as appears in the ledger,
	// or nil if the channel doesn't exist, or the chaincode isn't found in the ledger
	Metadata(channel string, cc string, collections ...string) *chaincode.Metadata
}

type policyFetcher interface {
	// PoliciesByChaincode returns the chaincode policy or existing collection level policies that can be
	// inquired for which identities satisfy them
	PoliciesByChaincode(channel string, cc string, collections ...string) []policies.InquireablePolicy
}

type gossipSupport interface {
	// IdentityInfo returns identity information about peers
	IdentityInfo() api.PeerIdentitySet

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common.ChannelID) gossipdiscovery.Members

	// Peers returns the NetworkMembers considered alive
	Peers() gossipdiscovery.Members
}

type membersChaincodeMapping struct {
	members          gossipdiscovery.Members
	chaincodeMapping map[string]gossipdiscovery.NetworkMember
}

type endorsementAnalyzer struct {
	gossipSupport
	principalEvaluator
	policyFetcher
	chaincodeMetadataFetcher
}

// NewEndorsementAnalyzer constructs an NewEndorsementAnalyzer out of the given support
func NewEndorsementAnalyzer(gs gossipSupport, pf policyFetcher, pe principalEvaluator, mf chaincodeMetadataFetcher) *endorsementAnalyzer {
	return &endorsementAnalyzer{
		gossipSupport:            gs,
		policyFetcher:            pf,
		principalEvaluator:       pe,
		chaincodeMetadataFetcher: mf,
	}
}

type peerPrincipalEvaluator func(member gossipdiscovery.NetworkMember, principal *msp.MSPPrincipal) bool

// PeersForEndorsement returns an EndorsementDescriptor for a given set of peers, channel, and chaincode
func (ea *endorsementAnalyzer) PeersForEndorsement(channelID common.ChannelID, interest *peer.ChaincodeInterest) (*discovery.EndorsementDescriptor, error) {
	membersAndCC, err := ea.peersByCriteria(channelID, interest, false)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	channelMembersById := membersAndCC.members.ByID()
	// Choose only the alive messages of those that have joined the channel
	aliveMembership := ea.Peers().Intersect(membersAndCC.members)
	membersById := aliveMembership.ByID()
	// Compute a mapping between the PKI-IDs of members to their identities
	identitiesOfMembers := computeIdentitiesOfMembers(ea.IdentityInfo(), membersById)
	principalsSets, err := ea.computePrincipalSets(channelID, interest)
	if err != nil {
		logger.Warningf("Principal set computation failed: %v", err)
		return nil, errors.WithStack(err)
	}

	return ea.computeEndorsementResponse(&context{
		chaincode:           interest.Chaincodes[0].Name,
		channel:             string(channelID),
		principalsSets:      principalsSets,
		channelMembersById:  channelMembersById,
		aliveMembership:     aliveMembership,
		identitiesOfMembers: identitiesOfMembers,
		chaincodeMapping:    membersAndCC.chaincodeMapping,
	})
}

func (ea *endorsementAnalyzer) PeersAuthorizedByCriteria(channelID common.ChannelID, interest *peer.ChaincodeInterest) (gossipdiscovery.Members, error) {
	res, err := ea.peersByCriteria(channelID, interest, true)
	return res.members, err
}

func (ea *endorsementAnalyzer) peersByCriteria(channelID common.ChannelID, interest *peer.ChaincodeInterest, excludePeersWithoutChaincode bool) (membersChaincodeMapping, error) {
	peersOfChannel := ea.PeersOfChannel(channelID)
	if interest == nil || len(interest.Chaincodes) == 0 {
		return membersChaincodeMapping{members: peersOfChannel}, nil
	}
	identities := ea.IdentityInfo()
	identitiesByID := identities.ByID()
	metadataAndCollectionFilters, err := loadMetadataAndFilters(metadataAndFilterContext{
		identityInfoByID: identitiesByID,
		interest:         interest,
		chainID:          channelID,
		evaluator:        ea,
		fetch:            ea,
	})
	if err != nil {
		return membersChaincodeMapping{}, errors.WithStack(err)
	}
	metadata := metadataAndCollectionFilters.md
	// Filter out peers that don't have the chaincode installed on them if required
	peersWithChaincode := peersOfChannel.Filter(peersWithChaincode(metadata...))
	chanMembership := peersOfChannel
	if excludePeersWithoutChaincode {
		chanMembership = peersWithChaincode
	}

	// Filter out peers that aren't authorized by the collection configs of the chaincode invocation chain
	members := chanMembership.Filter(metadataAndCollectionFilters.isMemberAuthorized)
	return membersChaincodeMapping{
		members:          members,
		chaincodeMapping: peersWithChaincode.ByID(),
	}, nil
}

type context struct {
	chaincode           string
	channel             string
	aliveMembership     gossipdiscovery.Members
	principalsSets      []policies.PrincipalSet
	channelMembersById  map[string]gossipdiscovery.NetworkMember
	identitiesOfMembers memberIdentities
	chaincodeMapping    map[string]gossipdiscovery.NetworkMember
}

func (ea *endorsementAnalyzer) computeEndorsementResponse(ctx *context) (*discovery.EndorsementDescriptor, error) {
	// mapPrincipalsToGroups returns a mapping from principals to their corresponding groups.
	// groups are just human readable representations that mask the principals behind them
	principalGroups := mapPrincipalsToGroups(ctx.principalsSets)
	// principalsToPeersGraph computes a bipartite graph (V1 U V2 , E)
	// such that V1 is the peers, V2 are the principals,
	// and each e=(peer,principal) is in E if the peer satisfies the principal
	satGraph := principalsToPeersGraph(principalAndPeerData{
		members: ctx.aliveMembership,
		pGrps:   principalGroups,
	}, ea.satisfiesPrincipal(ctx.channel, ctx.identitiesOfMembers))

	layouts := computeLayouts(ctx.principalsSets, principalGroups, satGraph)
	if len(layouts) == 0 {
		return nil, errors.New("no peer combination can satisfy the endorsement policy")
	}

	criteria := &peerMembershipCriteria{
		possibleLayouts:  layouts,
		satGraph:         satGraph,
		chanMemberById:   ctx.channelMembersById,
		idOfMembers:      ctx.identitiesOfMembers,
		chaincodeMapping: ctx.chaincodeMapping,
	}

	groupToEndorserListMapping := endorsersByGroup(criteria)
	layouts = filterOutUnsatisfiedLayouts(groupToEndorserListMapping, layouts)

	if len(layouts) == 0 {
		return nil, errors.New("required chaincodes are not installed on sufficient peers")
	}

	return &discovery.EndorsementDescriptor{
		Chaincode:         ctx.chaincode,
		Layouts:           layouts,
		EndorsersByGroups: groupToEndorserListMapping,
	}, nil
}

func filterOutUnsatisfiedLayouts(endorsersByGroup map[string]*discovery.Peers, layouts []*discovery.Layout) []*discovery.Layout {
	// Iterate once again over all layouts and ensure every layout has enough peers in the EndorsersByGroups
	// as required by the quantity in the layout.
	filteredLayouts := make([]*discovery.Layout, 0, len(layouts))
	for _, layout := range layouts {
		var layoutInvalid bool
		for group, quantity := range layout.QuantitiesByGroup {
			peerList := endorsersByGroup[group]
			if peerList == nil || len(peerList.Peers) < int(quantity) {
				layoutInvalid = true
			}
		}
		if layoutInvalid {
			continue
		}
		filteredLayouts = append(filteredLayouts, layout)
	}
	return filteredLayouts
}

func computeStateBasedPrincipalSets(chaincodes []*peer.ChaincodeCall, logger *flogging.FabricLogger) (inquire.ComparablePrincipalSets, error) {
	var stateBasedCPS []inquire.ComparablePrincipalSets
	for _, chaincode := range chaincodes {
		if len(chaincode.KeyPolicies) == 0 {
			continue
		}

		logger.Debugf("Chaincode call to %s is satisfied by %d state based policies of %v",
			chaincode.Name, len(chaincode.KeyPolicies), chaincode.KeyPolicies)

		for _, stateBasedPolicy := range chaincode.KeyPolicies {
			var cmpsets inquire.ComparablePrincipalSets
			stateBasedPolicy := inquire.NewInquireableSignaturePolicy(stateBasedPolicy)
			for _, ps := range stateBasedPolicy.SatisfiedBy() {
				cps := inquire.NewComparablePrincipalSet(ps)
				if cps == nil {
					return nil, errors.New("failed creating a comparable principal set for state based endorsement")
				}
				cmpsets = append(cmpsets, cps)
			}
			if len(cmpsets) == 0 {
				return nil, errors.New("state based endorsement policy cannot be satisfied")
			}
			stateBasedCPS = append(stateBasedCPS, cmpsets)
		}
	}

	if len(stateBasedCPS) > 0 {
		stateBasedPrincipalSet, err := mergePrincipalSets(stateBasedCPS)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		logger.Debugf("Merging state based policies: %v --> %v", stateBasedCPS, stateBasedPrincipalSet)

		return stateBasedPrincipalSet, nil
	}

	logger.Debugf("No state based policies requested")

	return nil, nil
}

func (ea *endorsementAnalyzer) computePrincipalSets(channelID common.ChannelID, interest *peer.ChaincodeInterest) (policies.PrincipalSets, error) {
	sessionLogger := logger.With("channel", string(channelID))
	var inquireablePoliciesForChaincodeAndCollections []policies.InquireablePolicy
	for _, chaincode := range interest.Chaincodes {
		policies := ea.PoliciesByChaincode(string(channelID), chaincode.Name, chaincode.CollectionNames...)
		if len(policies) == 0 {
			sessionLogger.Debug("Policy for chaincode '", chaincode, "'doesn't exist")
			return nil, errors.New("policy not found")
		}
		if chaincode.DisregardNamespacePolicy && len(chaincode.KeyPolicies) == 0 && len(policies) == 1 {
			sessionLogger.Warnf("Client requested to disregard chaincode %s's policy, but it did not specify any "+
				"collection policies or key policies. This is probably a bug in the client side code, as the client should"+
				"either not specify DisregardNamespacePolicy, or specify at least one key policy or at least one collection policy", chaincode.Name)
			return nil, errors.Errorf("requested to disregard chaincode %s's policy but key and collection policies are missing, either "+
				"disable DisregardNamespacePolicy or specify at least one key policy or at least one collection policy", chaincode.Name)
		}
		if chaincode.DisregardNamespacePolicy {
			if len(policies) == 1 {
				sessionLogger.Debugf("Client requested to disregard the namespace policy for chaincode %s,"+
					" and no collection policies are present", chaincode.Name)
				continue
			}
			sessionLogger.Debugf("Client requested to disregard the namespace policy for chaincode %s,"+
				" however there exist %d collection policies taken into account", chaincode.Name, len(policies)-1)
			policies = policies[1:]
		}
		inquireablePoliciesForChaincodeAndCollections = append(inquireablePoliciesForChaincodeAndCollections, policies...)
	}

	var cpss []inquire.ComparablePrincipalSets

	for _, policy := range inquireablePoliciesForChaincodeAndCollections {
		var cmpsets inquire.ComparablePrincipalSets
		for _, ps := range policy.SatisfiedBy() {
			cps := inquire.NewComparablePrincipalSet(ps)
			if cps == nil {
				return nil, errors.New("failed creating a comparable principal set")
			}
			cmpsets = append(cmpsets, cps)
		}
		if len(cmpsets) == 0 {
			return nil, errors.New("endorsement policy cannot be satisfied")
		}
		cpss = append(cpss, cmpsets)
	}

	stateBasedCPS, err := computeStateBasedPrincipalSets(interest.Chaincodes, sessionLogger)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(stateBasedCPS) > 0 {
		cpss = append(cpss, stateBasedCPS)
	}

	cps, err := mergePrincipalSets(cpss)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	sessionLogger.Debugf("Merging principal sets: %v --> %v", cpss, cps)

	return cps.ToPrincipalSets(), nil
}

type metadataAndFilterContext struct {
	chainID          common.ChannelID
	interest         *peer.ChaincodeInterest
	fetch            chaincodeMetadataFetcher
	identityInfoByID map[string]api.PeerIdentityInfo
	evaluator        principalEvaluator
}

// metadataAndColFilter holds metadata and member filters
type metadataAndColFilter struct {
	md                 []*chaincode.Metadata
	isMemberAuthorized memberFilter
}

func loadMetadataAndFilters(ctx metadataAndFilterContext) (*metadataAndColFilter, error) {
	sessionLogger := logger.With("channel", string(ctx.chainID))
	var metadata []*chaincode.Metadata
	var filters []identityFilter

	for _, chaincode := range ctx.interest.Chaincodes {
		ccMD := ctx.fetch.Metadata(string(ctx.chainID), chaincode.Name, chaincode.CollectionNames...)
		if ccMD == nil {
			return nil, errors.Errorf("No metadata was found for chaincode %s in channel %s", chaincode.Name, string(ctx.chainID))
		}
		metadata = append(metadata, ccMD)
		if len(chaincode.CollectionNames) == 0 {
			sessionLogger.Debugf("No collections for %s, skipping", chaincode.Name)
			continue
		}
		if chaincode.NoPrivateReads {
			sessionLogger.Debugf("No private reads, skipping")
			continue
		}
		principalSetByCollections, err := principalsFromCollectionConfig(ccMD.CollectionsConfig)
		if err != nil {
			sessionLogger.Warningf("Failed initializing collection filter for chaincode %s: %v", chaincode.Name, err)
			return nil, errors.WithStack(err)
		}
		filter, err := principalSetByCollections.toIdentityFilter(string(ctx.chainID), ctx.evaluator, chaincode)
		if err != nil {
			sessionLogger.Warningf("Failed computing collection principal sets for chaincode %s due to %v", chaincode.Name, err)
			return nil, errors.WithStack(err)
		}
		filters = append(filters, filter)
	}

	return computeFiltersWithMetadata(filters, metadata, ctx.identityInfoByID), nil
}

func computeFiltersWithMetadata(filters identityFilters, metadata []*chaincode.Metadata, identityInfoByID map[string]api.PeerIdentityInfo) *metadataAndColFilter {
	if len(filters) == 0 {
		return &metadataAndColFilter{
			md:                 metadata,
			isMemberAuthorized: noopMemberFilter,
		}
	}
	filter := filters.combine().toMemberFilter(identityInfoByID)
	return &metadataAndColFilter{
		isMemberAuthorized: filter,
		md:                 metadata,
	}
}

// identityFilter accepts or rejects peer identities
type identityFilter func(api.PeerIdentityType) bool

// identityFilters aggregates multiple identityFilters
type identityFilters []identityFilter

// memberFilter accepts or rejects NetworkMembers
type memberFilter func(member gossipdiscovery.NetworkMember) bool

// noopMemberFilter accepts every NetworkMember
func noopMemberFilter(_ gossipdiscovery.NetworkMember) bool {
	return true
}

// combine combines all identityFilters into a single identityFilter which only accepts identities
// which all the original filters accept
func (filters identityFilters) combine() identityFilter {
	return func(identity api.PeerIdentityType) bool {
		for _, f := range filters {
			if !f(identity) {
				return false
			}
		}
		return true
	}
}

// toMemberFilter converts this identityFilter to a memberFilter based on the given mapping
// from PKI-ID as strings, to PeerIdentityInfo which holds the peer identities
func (idf identityFilter) toMemberFilter(identityInfoByID map[string]api.PeerIdentityInfo) memberFilter {
	return func(member gossipdiscovery.NetworkMember) bool {
		identity, exists := identityInfoByID[string(member.PKIid)]
		if !exists {
			return false
		}
		return idf(identity.Identity)
	}
}

func (ea *endorsementAnalyzer) satisfiesPrincipal(channel string, identitiesOfMembers memberIdentities) peerPrincipalEvaluator {
	return func(member gossipdiscovery.NetworkMember, principal *msp.MSPPrincipal) bool {
		err := ea.SatisfiesPrincipal(channel, identitiesOfMembers.identityByPKIID(member.PKIid), principal)
		if err == nil {
			// TODO: log the principals in a human readable form
			logger.Debug(member, "satisfies principal", principal)
			return true
		}
		logger.Debug(member, "doesn't satisfy principal", principal, ":", err)
		return false
	}
}

type peerMembershipCriteria struct {
	satGraph         *principalPeerGraph
	idOfMembers      memberIdentities
	chanMemberById   map[string]gossipdiscovery.NetworkMember
	possibleLayouts  layouts
	chaincodeMapping map[string]gossipdiscovery.NetworkMember
}

// endorsersByGroup computes a map from groups to peers.
// Each group included, is found in some layout, which means
// that there is some principal combination that includes the corresponding
// group.
// This means that if a group isn't included in the result, there is no
// principal combination (that includes the principal corresponding to the group),
// such that there are enough peers to satisfy the principal combination.
func endorsersByGroup(criteria *peerMembershipCriteria) map[string]*discovery.Peers {
	satGraph := criteria.satGraph
	idOfMembers := criteria.idOfMembers
	chanMemberById := criteria.chanMemberById
	includedGroups := criteria.possibleLayouts.groupsSet()

	res := make(map[string]*discovery.Peers)
	// Map endorsers to their corresponding groups.
	// Iterate the principals, and put the peers into each group that corresponds with a principal vertex
	for grp, principalVertex := range satGraph.principalVertices {
		if _, exists := includedGroups[grp]; !exists {
			// If the current group is not found in any layout, skip the corresponding principal
			continue
		}
		peerList := &discovery.Peers{}
		for _, peerVertex := range principalVertex.Neighbors() {
			member := peerVertex.Data.(gossipdiscovery.NetworkMember)
			// Check if this peer has the chaincode installed
			stateInfo := chanMemberById[string(member.PKIid)]
			_, hasChaincodeInstalled := criteria.chaincodeMapping[string(stateInfo.PKIid)]
			if !hasChaincodeInstalled {
				continue
			}
			peerList.Peers = append(peerList.Peers, &discovery.Peer{
				Identity:       idOfMembers.identityByPKIID(member.PKIid),
				StateInfo:      stateInfo.Envelope,
				MembershipInfo: member.Envelope,
			})
		}

		if len(peerList.Peers) > 0 {
			res[grp] = peerList
		}
	}
	return res
}

// computeLayouts computes all possible principal combinations
// that can be used to satisfy the endorsement policy, given a graph
// of available peers that maps each peer to a principal it satisfies.
// Each such a combination is called a layout, because it maps
// a group (alias for a principal) to a threshold of peers that need to endorse,
// and that satisfy the corresponding principal
func computeLayouts(principalsSets []policies.PrincipalSet, principalGroups principalGroupMapper, satGraph *principalPeerGraph) []*discovery.Layout {
	var layouts []*discovery.Layout
	// principalsSets is a collection of combinations of principals,
	// such that each combination (given enough peers) satisfies the endorsement policy.
	for _, principalSet := range principalsSets {
		layout := &discovery.Layout{
			QuantitiesByGroup: make(map[string]uint32),
		}
		// Since principalsSet has repetitions, we first
		// compute a mapping from the principal to repetitions in the set.
		for principal, plurality := range principalSet.UniqueSet() {
			key := principalKey{
				cls:       int32(principal.PrincipalClassification),
				principal: string(principal.Principal),
			}
			// We map the principal to a group, which is an alias for the principal.
			layout.QuantitiesByGroup[principalGroups.group(key)] = uint32(plurality)
		}
		// Check that the layout can be satisfied with the current known peers
		// This is done by iterating the current layout, and ensuring that
		// each principal vertex is connected to at least <plurality> peer vertices.
		if isLayoutSatisfied(layout.QuantitiesByGroup, satGraph) {
			// If so, then add the layout to the layouts, since we have enough peers to satisfy the
			// principal combination
			layouts = append(layouts, layout)
		}
	}
	return layouts
}

func isLayoutSatisfied(layout map[string]uint32, satGraph *principalPeerGraph) bool {
	for grp, plurality := range layout {
		// Do we have more than <plurality> peers connected to the principal?
		if len(satGraph.principalVertices[grp].Neighbors()) < int(plurality) {
			return false
		}
	}
	return true
}

type principalPeerGraph struct {
	peerVertices      []*graph.Vertex
	principalVertices map[string]*graph.Vertex
}

type principalAndPeerData struct {
	members gossipdiscovery.Members
	pGrps   principalGroupMapper
}

func principalsToPeersGraph(data principalAndPeerData, satisfiesPrincipal peerPrincipalEvaluator) *principalPeerGraph {
	// Create the peer vertices
	peerVertices := make([]*graph.Vertex, len(data.members))
	for i, member := range data.members {
		peerVertices[i] = graph.NewVertex(string(member.PKIid), member)
	}

	// Create the principal vertices
	principalVertices := make(map[string]*graph.Vertex)
	for pKey, grp := range data.pGrps {
		principalVertices[grp] = graph.NewVertex(grp, pKey.toPrincipal())
	}

	// Connect principals and peers
	for _, principalVertex := range principalVertices {
		for _, peerVertex := range peerVertices {
			// If the current peer satisfies the principal, connect their corresponding vertices with an edge
			principal := principalVertex.Data.(*msp.MSPPrincipal)
			member := peerVertex.Data.(gossipdiscovery.NetworkMember)
			if satisfiesPrincipal(member, principal) {
				peerVertex.AddNeighbor(principalVertex)
			}
		}
	}
	return &principalPeerGraph{
		peerVertices:      peerVertices,
		principalVertices: principalVertices,
	}
}

func mapPrincipalsToGroups(principalsSets []policies.PrincipalSet) principalGroupMapper {
	groupMapper := make(principalGroupMapper)
	totalPrincipals := make(map[principalKey]struct{})
	for _, principalSet := range principalsSets {
		for _, principal := range principalSet {
			totalPrincipals[principalKey{
				principal: string(principal.Principal),
				cls:       int32(principal.PrincipalClassification),
			}] = struct{}{}
		}
	}
	for principal := range totalPrincipals {
		groupMapper.group(principal)
	}
	return groupMapper
}

type memberIdentities map[string]api.PeerIdentityType

func (m memberIdentities) identityByPKIID(id common.PKIidType) api.PeerIdentityType {
	return m[string(id)]
}

func computeIdentitiesOfMembers(identitySet api.PeerIdentitySet, members map[string]gossipdiscovery.NetworkMember) memberIdentities {
	identitiesByPKIID := make(map[string]api.PeerIdentityType)
	identitiesOfMembers := make(map[string]api.PeerIdentityType, len(members))
	for _, identity := range identitySet {
		identitiesByPKIID[string(identity.PKIId)] = identity.Identity
	}
	for _, member := range members {
		if identity, exists := identitiesByPKIID[string(member.PKIid)]; exists {
			identitiesOfMembers[string(member.PKIid)] = identity
		}
	}
	return identitiesOfMembers
}

// principalGroupMapper maps principals to names of groups
type principalGroupMapper map[principalKey]string

func (mapper principalGroupMapper) group(principal principalKey) string {
	if grp, exists := mapper[principal]; exists {
		return grp
	}
	grp := fmt.Sprintf("G%d", len(mapper))
	mapper[principal] = grp
	return grp
}

type principalKey struct {
	cls       int32
	principal string
}

func (pk principalKey) toPrincipal() *msp.MSPPrincipal {
	return &msp.MSPPrincipal{
		PrincipalClassification: msp.MSPPrincipal_Classification(pk.cls),
		Principal:               []byte(pk.principal),
	}
}

// layouts is an aggregation of several layouts
type layouts []*discovery.Layout

// groupsSet returns a set of groups that the layouts contain
func (l layouts) groupsSet() map[string]struct{} {
	m := make(map[string]struct{})
	for _, layout := range l {
		for grp := range layout.QuantitiesByGroup {
			m[grp] = struct{}{}
		}
	}
	return m
}

func peersWithChaincode(metadata ...*chaincode.Metadata) func(member gossipdiscovery.NetworkMember) bool {
	return func(member gossipdiscovery.NetworkMember) bool {
		if member.Properties == nil {
			return false
		}
		for _, ccMD := range metadata {
			var found bool
			for _, cc := range member.Properties.Chaincodes {
				if cc.Name == ccMD.Name && cc.Version == ccMD.Version {
					found = true
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
}

func mergePrincipalSets(cpss []inquire.ComparablePrincipalSets) (inquire.ComparablePrincipalSets, error) {
	// Obtain the first ComparablePrincipalSet first
	var cps inquire.ComparablePrincipalSets
	cps, cpss, err := popComparablePrincipalSets(cpss)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	for _, cps2 := range cpss {
		cps = inquire.Merge(cps, cps2)
	}
	return cps, nil
}

func popComparablePrincipalSets(sets []inquire.ComparablePrincipalSets) (inquire.ComparablePrincipalSets, []inquire.ComparablePrincipalSets, error) {
	if len(sets) == 0 {
		return nil, nil, errors.New("no principal sets remained after filtering")
	}
	cps, cpss := sets[0], sets[1:]
	return cps, cpss, nil
}
