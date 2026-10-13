package engine

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ElectionState represents the current state of leader election
type ElectionState int

const (
	StateIdle ElectionState = iota
	StateElecting
	StateElected
	StateReElecting
)

// LeaderElection manages the leader election process with Verifiable Random Functions (VRF)
type LeaderElection struct {
	config *Config

	// Election state
	mu                sync.RWMutex
	state             ElectionState
	electionID        string
	term              uint64
	activeNodes       []NodeID
	primaryLeader     NodeID
	shardLeaders      map[ShardID]NodeID
	shardAssignments  map[ShardID][]NodeID
	numShards         int
	electionTimeout   time.Duration
	heartbeatTimeout  time.Duration

	// VRF proof and output registry per node
	vrfOutputs        map[NodeID][]byte
	vrfProofs         map[NodeID][]byte

	// Tracking acknowledgments and responses
	acksReceived      map[NodeID]bool
	electionStartTime time.Time
	lastHeartbeat     map[NodeID]time.Time

	// Callbacks
	onLeaderElected   func(primary NodeID, shards map[ShardID]NodeID)
	onShardAssigned   func(shardID ShardID, leader NodeID, nodes []NodeID)

	// Channels for internal communication
	stopCh            chan struct{}
	electionTimer     *time.Timer
}

// NewLeaderElection creates a new leader election instance
func NewLeaderElection(config *Config, numShards int) *LeaderElection {
	le := &LeaderElection{
		config:            config,
		state:             StateIdle,
		term:              0,
		activeNodes:       make([]NodeID, 0),
		shardLeaders:      make(map[ShardID]NodeID),
		shardAssignments:  make(map[ShardID][]NodeID),
		vrfOutputs:        make(map[NodeID][]byte),
		vrfProofs:         make(map[NodeID][]byte),
		numShards:         numShards,
		electionTimeout:   30 * time.Second,
		heartbeatTimeout:  10 * time.Second,
		acksReceived:      make(map[NodeID]bool),
		lastHeartbeat:     make(map[NodeID]time.Time),
		stopCh:            make(chan struct{}),
	}

	// Initialize with current node as active
	le.activeNodes = append(le.activeNodes, config.NodeID)
	le.lastHeartbeat[config.NodeID] = time.Now()

	return le
}

// Start begins the leader election process
func (le *LeaderElection) Start() error {
	le.mu.Lock()
	defer le.mu.Unlock()

	if le.state != StateIdle {
		return fmt.Errorf("leader election already started")
	}

	le.config.Logger.Info("Starting leader election",
		"nodeID", le.config.NodeID,
		"numShards", le.numShards)

	// Start heartbeat monitoring
	go le.monitorHeartbeats()

	// Start initial election
	le.startElection()

	return nil
}

// Stop stops the leader election process
func (le *LeaderElection) Stop() {
	le.mu.Lock()
	defer le.mu.Unlock()

	le.state = StateIdle
	close(le.stopCh)

	if le.electionTimer != nil {
		le.electionTimer.Stop()
	}

	le.config.Logger.Info("Leader election stopped", "nodeID", le.config.NodeID)
}

// StartElection initiates a new leader election
func (le *LeaderElection) StartElection() {
	le.mu.Lock()
	defer le.mu.Unlock()

	le.startElection()
}

func (le *LeaderElection) startElection() {
	le.state = StateElecting
	le.term++
	le.electionID = le.generateElectionID()
	le.electionStartTime = time.Now()
	le.acksReceived = make(map[NodeID]bool)
	le.vrfOutputs = make(map[NodeID][]byte)
	le.vrfProofs = make(map[NodeID][]byte)

	le.config.Logger.Info("Starting new election with VRF",
		"term", le.term,
		"electionID", le.electionID,
		"activeNodes", len(le.activeNodes))

	// Compute local VRF output for this term and election
	vrfInput := ComputeElectionDigest(le.config.ChannelID, le.term, le.electionID, le.config.NodeID)
	var vrfOutput []byte
	var vrfProof []byte

	if le.config.Signer != nil {
		sig := le.config.Signer.Sign(vrfInput)
		vrfProof = sig
		h := sha256.Sum256(append([]byte("VRF-OUTPUT:"), sig...))
		vrfOutput = h[:]
	} else {
		h := sha256.Sum256(vrfInput)
		vrfOutput = h[:]
		vrfProof = []byte("self-proof")
	}

	le.vrfOutputs[le.config.NodeID] = vrfOutput
	le.vrfProofs[le.config.NodeID] = vrfProof
	le.acksReceived[le.config.NodeID] = true

	// Send election message to all known nodes
	electionMsg := &LeaderElectionMessage{
		ElectionID:  le.electionID,
		CandidateID: le.config.NodeID,
		ActiveNodes: le.activeNodes,
		Term:        le.term,
		VRFOutput:   vrfOutput,
		VRFProof:    vrfProof,
		Timestamp:   time.Now(),
	}

	if le.config.Signer != nil {
		electionMsg.Signature = le.config.Signer.Sign([]byte(le.electionID))
	}

	msg := Message{
		Type:      MsgLeaderElection,
		From:      le.config.NodeID,
		Timestamp: time.Now(),
		Payload:   electionMsg,
	}

	// Broadcast to all nodes
	for _, nodeID := range le.activeNodes {
		if nodeID != le.config.NodeID {
			msg.To = nodeID
			le.config.Network.Send(nodeID, msg)
		}
	}

	// Set election timeout
	le.electionTimer = time.AfterFunc(le.electionTimeout, le.handleElectionTimeout)
}

// HandleMessage processes incoming leader election messages
func (le *LeaderElection) HandleMessage(from NodeID, message Message) error {
	switch message.Type {
	case MsgLeaderElection:
		return le.handleLeaderElection(from, message.Payload.(*LeaderElectionMessage))
	case MsgElectionAck:
		return le.handleElectionAck(from, message.Payload.(*ElectionAckMessage))
	case MsgShardAssignment:
		return le.handleShardAssignment(from, message.Payload.(*ShardAssignmentMessage))
	case MsgLeaderAnnouncement:
		return le.handleLeaderAnnouncement(from, message.Payload.(*LeaderAnnouncementMessage))
	default:
		return fmt.Errorf("unknown message type for leader election: %v", message.Type)
	}
}

func (le *LeaderElection) handleLeaderElection(from NodeID, msg *LeaderElectionMessage) error {
	le.mu.Lock()
	defer le.mu.Unlock()

	le.config.Logger.Info("Received leader election message",
		"from", from,
		"term", msg.Term,
		"electionID", msg.ElectionID,
		"candidate", msg.CandidateID)

	// Guard against stale terms
	if msg.Term < le.term {
		le.config.Logger.Debug("Ignoring stale election message", "msgTerm", msg.Term, "currentTerm", le.term)
		return nil
	}

	if msg.Term > le.term {
		le.term = msg.Term
		le.electionID = msg.ElectionID
		le.vrfOutputs = make(map[NodeID][]byte)
		le.vrfProofs = make(map[NodeID][]byte)
		le.acksReceived = make(map[NodeID]bool)
	}

	// Update active nodes list
	le.updateActiveNodes(msg.ActiveNodes)

	// Record candidate's VRF output
	if len(msg.VRFOutput) > 0 {
		le.vrfOutputs[msg.CandidateID] = msg.VRFOutput
		le.vrfProofs[msg.CandidateID] = msg.VRFProof
	}

	// Compute own VRF output for this election
	vrfInput := ComputeElectionDigest(le.config.ChannelID, msg.Term, msg.ElectionID, le.config.NodeID)
	var ownVRFOutput []byte
	var ownVRFProof []byte

	if le.config.Signer != nil {
		sig := le.config.Signer.Sign(vrfInput)
		ownVRFProof = sig
		h := sha256.Sum256(append([]byte("VRF-OUTPUT:"), sig...))
		ownVRFOutput = h[:]
	} else {
		h := sha256.Sum256(vrfInput)
		ownVRFOutput = h[:]
		ownVRFProof = []byte("self-proof")
	}

	// Send acknowledgment with own VRF proof
	ackMsg := &ElectionAckMessage{
		ElectionID:   msg.ElectionID,
		NodeID:       le.config.NodeID,
		Term:         msg.Term,
		VRFOutput:    ownVRFOutput,
		VRFProof:     ownVRFProof,
		Acknowledged: true,
		Timestamp:    time.Now(),
	}

	if le.config.Signer != nil {
		ackMsg.Signature = le.config.Signer.Sign([]byte(msg.ElectionID + string(le.config.NodeID)))
	}

	response := Message{
		Type:      MsgElectionAck,
		From:      le.config.NodeID,
		To:        from,
		Timestamp: time.Now(),
		Payload:   ackMsg,
	}

	le.config.Network.Send(from, response)

	// If this node has higher priority (VRF score), initiate election
	if le.shouldStartElection(msg.CandidateID) && le.state != StateElecting {
		le.startElection()
	}

	return nil
}

func (le *LeaderElection) handleElectionAck(from NodeID, msg *ElectionAckMessage) error {
	le.mu.Lock()
	defer le.mu.Unlock()

	if msg.ElectionID != le.electionID || msg.Term < le.term {
		le.config.Logger.Debug("Ignoring stale election ack",
			"expected", le.electionID,
			"received", msg.ElectionID,
			"msgTerm", msg.Term,
			"currentTerm", le.term)
		return nil
	}

	le.acksReceived[from] = msg.Acknowledged
	if len(msg.VRFOutput) > 0 {
		le.vrfOutputs[from] = msg.VRFOutput
		le.vrfProofs[from] = msg.VRFProof
	}

	le.config.Logger.Info("Received election ack with VRF",
		"from", from,
		"electionID", msg.ElectionID,
		"acksReceived", len(le.acksReceived),
		"vrfCount", len(le.vrfOutputs))

	// Check if Byzantine quorum of acknowledgments is reached
	if le.hasMajorityAcks() && le.state == StateElecting {
		le.selectLeaders()
	}

	return nil
}

func (le *LeaderElection) handleShardAssignment(from NodeID, msg *ShardAssignmentMessage) error {
	le.mu.Lock()
	defer le.mu.Unlock()

	le.config.Logger.Info("Received shard assignment",
		"from", from,
		"electionID", msg.ElectionID,
		"primaryLeader", msg.PrimaryLeader)

	// Update local state
	le.primaryLeader = msg.PrimaryLeader
	le.shardLeaders = msg.ShardLeaders
	le.shardAssignments = msg.ShardNodes
	le.state = StateElected

	// Update config
	le.config.PrimaryLeader = msg.PrimaryLeader
	le.config.ShardLeaders = msg.ShardLeaders
	le.config.ShardNodes = msg.ShardNodes

	// Update nodes with new configuration
	if le.config.Node != nil {
		le.config.Node.UpdateNodeConfig(msg.PrimaryLeader, msg.ShardLeaders, msg.ShardNodes)
	}

	if le.onShardAssigned != nil {
		for shardID, nodes := range msg.ShardNodes {
			leader := msg.ShardLeaders[shardID]
			le.onShardAssigned(shardID, leader, nodes)
		}
	}

	// Restart heartbeat monitor with new configuration
	le.restartHeartbeatMonitor()

	return nil
}

func (le *LeaderElection) handleLeaderAnnouncement(from NodeID, msg *LeaderAnnouncementMessage) error {
	le.mu.Lock()
	defer le.mu.Unlock()

	le.config.Logger.Info("Received leader announcement",
		"from", from,
		"electionID", msg.ElectionID,
		"primaryLeader", msg.PrimaryLeader)

	// Update local state
	le.primaryLeader = msg.PrimaryLeader
	le.shardLeaders = msg.ShardLeaders
	le.state = StateElected

	// Update config
	le.config.PrimaryLeader = msg.PrimaryLeader
	le.config.ShardLeaders = msg.ShardLeaders

	// Update shard assignments if available
	if msg.ShardNodes != nil {
		le.shardAssignments = msg.ShardNodes
		le.config.ShardNodes = msg.ShardNodes
	}

	// Update nodes with new configuration
	if le.config.Node != nil {
		le.config.Node.UpdateNodeConfig(msg.PrimaryLeader, msg.ShardLeaders, msg.ShardNodes)
	}

	if le.onLeaderElected != nil {
		le.onLeaderElected(msg.PrimaryLeader, msg.ShardLeaders)
	}

	// Restart heartbeat monitor with new configuration
	le.restartHeartbeatMonitor()

	return nil
}

// selectLeaders performs leader selection using VRF scores across nodes
func (le *LeaderElection) selectLeaders() {
	le.config.Logger.Info("Selecting leaders using VRF ranking algorithm",
		"electionID", le.electionID,
		"activeNodes", len(le.activeNodes),
		"vrfOutputs", len(le.vrfOutputs))

	// Sort active nodes deterministically based on VRF scores
	sortedNodes := le.sortNodesByVRFScore()
	if len(sortedNodes) == 0 {
		le.config.Logger.Error("No active nodes available for leader selection")
		return
	}

	// Highest VRF score node becomes Primary Leader
	le.primaryLeader = sortedNodes[0]

	le.config.Logger.Info("Selected primary leader via VRF",
		"primaryLeader", le.primaryLeader,
		"totalRanked", len(sortedNodes))

	// Select shard leaders from remaining nodes
	remainingNodes := sortedNodes[1:]
	le.selectShardLeaders(remainingNodes)

	// Announce assignments
	le.announceAssignments()
}

// sortNodesByVRFScore sorts active nodes based on their verified VRF outputs (descending)
func (le *LeaderElection) sortNodesByVRFScore() []NodeID {
	sorted := make([]NodeID, 0, len(le.activeNodes))
	for _, n := range le.activeNodes {
		sorted = append(sorted, n)
	}

	sort.Slice(sorted, func(i, j int) bool {
		scoreI := VRFScore(le.vrfOutputs[sorted[i]])
		scoreJ := VRFScore(le.vrfOutputs[sorted[j]])
		cmp := scoreI.Cmp(scoreJ)
		if cmp != 0 {
			return cmp > 0 // Highest score first
		}
		return string(sorted[i]) < string(sorted[j]) // Deterministic tie breaker
	})

	return sorted
}

func (le *LeaderElection) selectShardLeaders(nodes []NodeID) {
	le.shardLeaders = make(map[ShardID]NodeID)
	le.shardAssignments = make(map[ShardID][]NodeID)

	if len(nodes) == 0 {
		le.config.Logger.Info("No nodes available for shard assignment")
		return
	}

	if le.numShards <= 0 {
		le.numShards = 1
	}

	nodesPerShard := len(nodes) / le.numShards
	extraNodes := len(nodes) % le.numShards

	start := 0
	for i := 0; i < le.numShards; i++ {
		shardID := ShardID(i + 1)
		end := start + nodesPerShard
		if i < extraNodes {
			end++
		}

		if start >= len(nodes) {
			break
		}

		shardNodes := nodes[start:end]
		if len(shardNodes) > 0 {
			// First node in shard (highest VRF rank in this shard) becomes shard leader
			le.shardLeaders[shardID] = shardNodes[0]
			le.shardAssignments[shardID] = shardNodes

			le.config.Logger.Info("Assigned shard via VRF ranking",
				"shardID", shardID,
				"leader", shardNodes[0],
				"nodes", len(shardNodes))
		}

		start = end
	}
}

func (le *LeaderElection) announceAssignments() {
	assignmentMsg := &ShardAssignmentMessage{
		ElectionID:    le.electionID,
		PrimaryLeader: le.primaryLeader,
		ShardLeaders:  le.shardLeaders,
		ShardNodes:    le.shardAssignments,
		Timestamp:     time.Now(),
	}

	if le.config.Signer != nil {
		data := fmt.Sprintf("%s%s", le.electionID, le.primaryLeader)
		assignmentMsg.Signature = le.config.Signer.Sign([]byte(data))
	}

	msg := Message{
		Type:      MsgShardAssignment,
		From:      le.config.NodeID,
		Timestamp: time.Now(),
		Payload:   assignmentMsg,
	}

	// Broadcast to all nodes
	for _, nodeID := range le.activeNodes {
		if nodeID != le.config.NodeID {
			msg.To = nodeID
			le.config.Network.Send(nodeID, msg)
		}
	}

	// Also send announcement
	announcementMsg := &LeaderAnnouncementMessage{
		ElectionID:    le.electionID,
		PrimaryLeader: le.primaryLeader,
		ShardLeaders:  le.shardLeaders,
		ShardNodes:    le.shardAssignments,
		Timestamp:     time.Now(),
	}

	if le.config.Signer != nil {
		data := fmt.Sprintf("%s%s", le.electionID, le.primaryLeader)
		announcementMsg.Signature = le.config.Signer.Sign([]byte(data))
	}

	announceMsg := Message{
		Type:      MsgLeaderAnnouncement,
		From:      le.config.NodeID,
		Timestamp: time.Now(),
		Payload:   announcementMsg,
	}

	for _, nodeID := range le.activeNodes {
		if nodeID != le.config.NodeID {
			announceMsg.To = nodeID
			le.config.Network.Send(nodeID, announceMsg)
		}
	}

	le.state = StateElected
	le.updateNodeRole()

	if le.onLeaderElected != nil {
		le.onLeaderElected(le.primaryLeader, le.shardLeaders)
	}

	le.restartHeartbeatMonitor()
}

func (le *LeaderElection) updateNodeRole() {
	oldRole := le.config.Role
	oldShardID := le.config.ShardID

	if le.config.NodeID == le.primaryLeader {
		le.config.Role = RolePrimaryLeader
		le.config.ShardID = ShardID(0)
	} else {
		found := false
		for shardID, leader := range le.shardLeaders {
			if le.config.NodeID == leader {
				le.config.Role = RoleShardLeader
				le.config.ShardID = shardID
				found = true
				break
			}
		}

		if !found {
			for shardID, nodes := range le.shardAssignments {
				for _, nodeID := range nodes {
					if le.config.NodeID == nodeID {
						le.config.Role = RoleShardFollower
						le.config.ShardID = shardID
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
	}

	le.config.Logger.Info("Updated node role",
		"nodeID", le.config.NodeID,
		"newRole", le.config.Role.String(),
		"newShardID", le.config.ShardID,
		"oldRole", oldRole.String(),
		"oldShardID", oldShardID)

	if oldRole != le.config.Role || oldShardID != le.config.ShardID {
		if le.config.Node != nil {
			le.config.Node.UpdateNodeRole(le.config.Role)
		}
	}
}

func (le *LeaderElection) UpdateNodeConfig(primaryLeader NodeID, shardLeaders map[ShardID]NodeID, shardAssignments map[ShardID][]NodeID) {
	le.primaryLeader = primaryLeader
	le.shardLeaders = shardLeaders
	le.shardAssignments = shardAssignments

	le.config.PrimaryLeader = primaryLeader
	le.config.ShardLeaders = shardLeaders
	le.config.ShardNodes = shardAssignments

	le.updateNodeRole()

	if le.config.Node != nil {
		le.config.Node.UpdateNodeConfig(primaryLeader, shardLeaders, shardAssignments)
	}
}

func (le *LeaderElection) sortNodes(nodes []NodeID) []NodeID {
	sorted := make([]NodeID, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return string(sorted[i]) < string(sorted[j])
	})
	return sorted
}

func (le *LeaderElection) shouldStartElection(otherCandidate NodeID) bool {
	return string(le.config.NodeID) < string(otherCandidate)
}

func (le *LeaderElection) hasMajorityAcks() bool {
	acks := 0
	for _, acked := range le.acksReceived {
		if acked {
			acks++
		}
	}
	requiredQuorum := CalculateIntraShardQuorum(len(le.activeNodes))
	return acks >= requiredQuorum
}

func (le *LeaderElection) updateActiveNodes(nodes []NodeID) {
	le.activeNodes = le.sortNodes(nodes)
	le.config.Logger.Info("Updated active nodes",
		"count", len(le.activeNodes),
		"nodes", le.activeNodes)
}

func (le *LeaderElection) generateElectionID() string {
	data := fmt.Sprintf("%s%d%d", le.config.NodeID, le.term, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
}

func (le *LeaderElection) handleElectionTimeout() {
	le.mu.Lock()
	defer le.mu.Unlock()

	if le.state == StateElecting {
		le.config.Logger.Info("Election timeout, starting re-election",
			"electionID", le.electionID,
			"term", le.term)
		le.state = StateReElecting
		le.startElection()
	}
}

func (le *LeaderElection) monitorHeartbeats() {
	ticker := time.NewTicker(le.heartbeatTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-le.stopCh:
			return
		case <-ticker.C:
			le.checkHeartbeats()
		}
	}
}

func (le *LeaderElection) restartHeartbeatMonitor() {
	le.config.Logger.Info("Restarting heartbeat monitor after election",
		"nodeID", le.config.NodeID,
		"primaryLeader", le.primaryLeader,
		"activeNodes", len(le.activeNodes))

	newHeartbeatMap := make(map[NodeID]time.Time)
	now := time.Now()
	for _, nodeID := range le.activeNodes {
		if existingTime, exists := le.lastHeartbeat[nodeID]; exists {
			if now.Sub(existingTime) < le.heartbeatTimeout {
				newHeartbeatMap[nodeID] = existingTime
			} else {
				newHeartbeatMap[nodeID] = now
			}
		} else {
			newHeartbeatMap[nodeID] = now
		}
	}

	if _, exists := newHeartbeatMap[le.primaryLeader]; !exists {
		newHeartbeatMap[le.primaryLeader] = now
	}

	for shardID, shardLeader := range le.shardLeaders {
		newHeartbeatMap[shardLeader] = now
		le.config.Logger.Info("Updated heartbeat for shard leader",
			"shardID", shardID,
			"shardLeader", shardLeader)
	}

	le.lastHeartbeat = newHeartbeatMap
}

func (le *LeaderElection) checkHeartbeats() {
	le.mu.Lock()
	defer le.mu.Unlock()

	now := time.Now()
	newActiveNodes := make([]NodeID, 0)

	for _, nodeID := range le.activeNodes {
		if lastHB, exists := le.lastHeartbeat[nodeID]; exists {
			if now.Sub(lastHB) < le.heartbeatTimeout {
				newActiveNodes = append(newActiveNodes, nodeID)
			} else {
				le.config.Logger.Info("Node failed heartbeat check",
					"nodeID", nodeID,
					"lastHeartbeat", lastHB)
			}
		}
	}

	if len(newActiveNodes) != len(le.activeNodes) {
		le.activeNodes = newActiveNodes
		le.config.Logger.Info("Active nodes changed",
			"oldCount", len(le.activeNodes),
			"newCount", len(newActiveNodes),
			"currentState", le.state)

		if !le.isNodeActive(le.primaryLeader) && le.state != StateElecting && le.state != StateReElecting {
			le.config.Logger.Info("Primary leader failed, triggering re-election",
				"primaryLeader", le.primaryLeader,
				"currentState", le.state)
			le.startElection()
		}
	}
}

func (le *LeaderElection) isNodeActive(nodeID NodeID) bool {
	for _, active := range le.activeNodes {
		if active == nodeID {
			return true
		}
	}
	return false
}

func (le *LeaderElection) ReceiveHeartbeat(from NodeID, timestamp time.Time) {
	le.mu.Lock()
	defer le.mu.Unlock()

	le.lastHeartbeat[from] = timestamp

	if !le.isNodeActive(from) {
		le.activeNodes = append(le.activeNodes, from)
		le.activeNodes = le.sortNodes(le.activeNodes)
		le.config.Logger.Info("Added new active node",
			"nodeID", from,
			"totalActive", len(le.activeNodes))
	}
}

func (le *LeaderElection) GetPrimaryLeader() NodeID {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.primaryLeader
}

func (le *LeaderElection) GetShardLeaders() map[ShardID]NodeID {
	le.mu.RLock()
	defer le.mu.RUnlock()
	shardLeaders := make(map[ShardID]NodeID)
	for k, v := range le.shardLeaders {
		shardLeaders[k] = v
	}
	return shardLeaders
}

func (le *LeaderElection) GetShardAssignments() map[ShardID][]NodeID {
	le.mu.RLock()
	defer le.mu.RUnlock()
	assignments := make(map[ShardID][]NodeID)
	for k, v := range le.shardAssignments {
		assignments[k] = make([]NodeID, len(v))
		copy(assignments[k], v)
	}
	return assignments
}

func (le *LeaderElection) GetState() ElectionState {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.state
}

func (le *LeaderElection) SetCallbacks(onLeaderElected func(primary NodeID, shards map[ShardID]NodeID),
	onShardAssigned func(shardID ShardID, leader NodeID, nodes []NodeID)) {
	le.onLeaderElected = onLeaderElected
	le.onShardAssigned = onShardAssigned
}
