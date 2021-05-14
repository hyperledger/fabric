/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statemachine

import (
	"bytes"
	"container/list"
	"fmt"
	"sort"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/status"
)

// TODO this comment section is woefully out of date, but contains so much information
// simply deleting it seems inappropriate.  So, we should come back and clean this up.
//
// The client_tracker code is responsible for tracking the requests made by clients,
// for ack-ing those requests to the network, and replicating requests from other replicas.
// There are two general paths through which a request may arrive.
//
//   1. The replica has the request given to it directly by a client, or from some other
//      verifiable source (like a signed gossip message).  The replica verifies this request
//      to be correct using whatever application logic is required, and, then injects it
//      into the state machine via the 'Propose' API.
//
//   2. The replica receives a weak quorum of request-ack messages, indicating that at least
//      one correct replica has a request from the client and has validated it according to the
//      application logic as required.  This request may be replicated in a number of ways.
//
//        a) The replica may receive an unsolicited 'forward' of the request from a primary
//           who is including this request in a batch but has not received this replica's ack.
//
//        b) The replica may, after sufficient ticks elapse, send a fetch request for this
//           request from a subset of the weak quorum.  This is to handle the case where a
//           non-byzantine client crashes after disseminating the request to f+1 correct nodes
//           but before disseminating it to 2f+1 correct nodes.
//
//        c) During an epoch change, a request may be selected, which requires f+1 nodes to
//           agree on the digest, which implies at least one correct node has validate the request.
//           In this case, we apply the epoch-change messages as request-acks because we may
//           not have a weak quorum of request-ack messages yet.
//
// Because we do not have reliable message transmission, it is possible that some replicas may 'miss'
// a request-ack and a client's progress could stall.  To counter this, we rebroadcast the request-ack
// for a client request, with a backoff, such that eventually, all replicas receive all acks (or the
// request number for that client first commits).
//
// It is possible that the client is byzantine (or, behaves in a byzantine manner due to a crash),
// and ends up submitting two valid requests for the same request number.  First, to some set of f+1
// replicas, then to another disjoint set of f+1.  With cooperation of byzantine replicas, it becomes
// clear that even more duplicated requests could be injected into the system.  To counter this
// resource drain, once a replica observes two distinct valid requests for a request number, it
// begins to advocate via its request-ack for a null request (consuming the request number, but
// committing no data).  A byzantine client may of course stall its own progress by carefully selecting
// quorums which prevent the null request from being generated, but, this has no impact on other
// clients, so the null request recourse is only assistance for clients which are accidentally
// byzantine.
//
// When a client connects to a replica, it should solicit the last request number that this replica
// has stored and acknowledged for this client as well as the currently committed low watermark
// for this client.  Based on this information, the client should be able to compute a request range
// which is uncommitted, and it may either:
//
//   1. If the client persisted its request log, it may simply resubmit the same persisted
//      requests, and committing should resume across the network with no loss of data, or holes.
//
//   2. If the client did not persist its requests to a log, it may solicit the uncommitted requests
//      from the replicas, and verify them itself.  This could be checking a signature, a hash-mac
//      or other signal to indicate that the request is valid.  Then, it can resubmit the uncommitted
//      requests just as in (1).
//
//   3. If the client is willing to tolerate request loss, it may simply submit a null request (a
//      a request with no data) for each uncommitted request number for which some replica claims it
//      has received a request.  The replicas will preferentially acknowledge the null request and the
//      client may then begin submitting new requests from a new common low watermark.  Note, that
//      because the client cannot validate whether a replica is byzantinely claiming it received
//      a request that it did not commit, in the worst case, the client will be forced to fill its
//      entire set of watermarks with null requests.  Fortunately, null requests are very lightweight
//      and are of little consequence, so this is an uninteresting attack vector.
//
// Both the normal epoch operation and epoch change code have a dependency on waiting for requests to
// become available (which means both correct, and persisted).  In the normal operation case, a
// replica receives a preprepare, and validates that all of the requests contained in the preprepare
// are present.  If they are not, it waits to preprepare the request until they are available.  In
// the case of a non-byzantine primary, at least f+1 correct replicas have already acked this request,
// so the request will eventually be known as correct, and, the primary will forward the request
// because we have not acked it.  Any preprepare/prepare or commit message also indicates that a
// replica acks this request and we can update the set of agreements accordingly.
//
// In the epoch change case, a batch which contains requests we do not have may be selected for a
// sequence in accordance with the normal view-change rules.  In this case, we must fetch the request
// before continuing with the epoch change, but since f+1 replicas must have included this request
// in their q-set, we know it to be correct, and therefore, we may update our agreements and fetch
// accordingly.
//
// In order to prevent spamming the network and to allow for message dropping, we flag a request
// as being fetched, and, after some expiration attempt to fetch once more.  Any concurrent instructions
// to fetch the request are ignored so long as the request is currently being fetched.
//
// To additionally enforce that a byzantine replica does not spam many different digests for the same
// request, we only store the first non-null ack a replica sends for a given client and request number.
// However, if a request is known to be correct, because of a weak quorum during the standard three
// phase commit, or during epoch change, then we do record the replicas agreement with this correct
// digest.  This bounds the maximum possible number of digests a replica stores for a given sequence
// at n (though requires a byzantine client colluding with f byzantine replicas, and the client
// will be detected as byzantine).
//
// The client tracker stores
// availableList -- all requests we have stored (and are therefore correct) -- a list of clientReqNos
// since multiple requests for the same reqno could become available.
// strongList -- all requests which have a continuous strong quorum cert for each reqNo after the last commited reqNo, and can therefore safely be proposed -- a list of clientReqNos (allowing for the null request to become strong and supersede another strong request)
// weakList -- all requests which are correct, but we have not replicated locally -- a list of
// replicatingMap -- all requests which are currently being fetched

type clientHashDisseminator struct {
	logger      Logger
	myConfig    *state.EventInitialParameters
	nodeBuffers *nodeBuffers

	allocatedThrough uint64
	networkConfig    *msgs.NetworkState_Config
	clientStates     []*msgs.NetworkState_Client
	msgBuffers       map[nodeID]*msgBuffer
	clients          map[uint64]*client
	clientTracker    *clientTracker
}

func newClientHashDisseminator(nodeBuffers *nodeBuffers, myConfig *state.EventInitialParameters, logger Logger, clientTracker *clientTracker) *clientHashDisseminator {
	return &clientHashDisseminator{
		logger:        logger,
		myConfig:      myConfig,
		nodeBuffers:   nodeBuffers,
		clientTracker: clientTracker,
	}
}

func (ct *clientHashDisseminator) reinitialize(seqNo uint64, networkState *msgs.NetworkState) *ActionList {
	actions := &ActionList{}
	reconfiguring := len(networkState.PendingReconfigurations) > 0

	latestClientStates := map[uint64]*msgs.NetworkState_Client{}
	for _, clientState := range networkState.Clients {
		latestClientStates[clientState.Id] = clientState
	}

	ct.allocatedThrough = seqNo
	ct.networkConfig = networkState.Config

	oldClients := ct.clients
	ct.clients = map[uint64]*client{}
	ct.clientStates = networkState.Clients
	for _, clientState := range ct.clientStates {
		client, ok := oldClients[clientState.Id]
		if !ok {
			client = newClient(ct.myConfig, ct.logger, ct.clientTracker)
		}

		ct.clients[clientState.Id] = client
		// TODO, this assumes no client deletion, need to handle that
		actions.concat(client.reinitialize(seqNo, networkState.Config, clientState, reconfiguring))
	}

	oldMsgBuffers := ct.msgBuffers
	ct.msgBuffers = map[nodeID]*msgBuffer{}
	for _, id := range networkState.Config.Nodes {
		if oldBuffer, ok := oldMsgBuffers[nodeID(id)]; ok {
			ct.msgBuffers[nodeID(id)] = oldBuffer
		} else {
			ct.msgBuffers[nodeID(id)] = newMsgBuffer("clients", ct.nodeBuffers.nodeBuffer(nodeID(id)))
		}
	}

	return actions
}

func (ct *clientHashDisseminator) tick() *ActionList {
	actions := &ActionList{}
	for _, clientState := range ct.clientStates {
		client := ct.clients[clientState.Id]
		actions.concat(client.tick())
	}
	return actions
}

func (ct *clientHashDisseminator) filter(_ nodeID, msg *msgs.Msg) applyable {
	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_RequestAck:
		// TODO, prevent ack spam of multiple msg digests from the same node
		ack := innerMsg.RequestAck
		client, ok := ct.client(ack.ClientId)
		if !ok {
			return future
		}
		switch {
		case client.clientState.LowWatermark > ack.ReqNo:
			return past
		case client.highWatermark < ack.ReqNo:
			return future
		default:
			return current
		}
	case *msgs.Msg_FetchRequest:
		return current // TODO push outside state machine
	default:
		panic(fmt.Sprintf("unexpected bad client window message type %T, this indicates a bug", msg.Type))
	}
}

func (ct *clientHashDisseminator) step(source nodeID, msg *msgs.Msg) *ActionList {
	switch ct.filter(source, msg) {
	case past:
		// discard
		return &ActionList{}
	case future:
		ct.msgBuffers[source].store(msg)
		return &ActionList{}
	}

	// current
	return ct.applyMsg(source, msg)
}

func (ct *clientHashDisseminator) applyMsg(source nodeID, msg *msgs.Msg) *ActionList {
	switch innerMsg := msg.Type.(type) {
	case *msgs.Msg_RequestAck:
		actions, _ := ct.ack(source, innerMsg.RequestAck)
		return actions
	case *msgs.Msg_FetchRequest:
		msg := innerMsg.FetchRequest
		return ct.replyFetchRequest(source, msg.ClientId, msg.ReqNo, msg.Digest)
	default:
		panic(fmt.Sprintf("unexpected bad client window message type %T, this indicates a bug", msg.Type))
	}
}

func (ct *clientHashDisseminator) applyNewRequest(ack *msgs.RequestAck) *ActionList {
	client, ok := ct.clients[ack.ClientId]
	if !ok {
		// Unusual, client must have been removed since we processed the request
		return &ActionList{}
	}

	if !client.inWatermarks(ack.ReqNo) {
		// We've already committed this reqno
		return &ActionList{}
	}

	client.reqNo(ack.ReqNo).applyNewRequest(ack)

	return client.advanceAcks()
}

// allocate should be invoked after the checkpoint is computed and advances the high watermark.
func (ct *clientHashDisseminator) allocate(seqNo uint64, networkState *msgs.NetworkState) *ActionList {
	assertEqual(seqNo, uint64(networkState.Config.CheckpointInterval)+ct.allocatedThrough, "unexpected skip in allocate, expected next allocation at next checkpoint")

	actions := &ActionList{}

	ct.allocatedThrough = seqNo
	reconfiguring := len(networkState.PendingReconfigurations) > 0

	for _, client := range networkState.Clients {
		actions.concat(ct.clients[client.Id].allocate(seqNo, client, reconfiguring))
	}

	for _, id := range ct.networkConfig.Nodes {
		ct.msgBuffers[nodeID(id)].iterate(ct.filter, func(source nodeID, msg *msgs.Msg) {
			actions.concat(ct.applyMsg(source, msg))
		})
	}
	return actions
}

func (ct *clientHashDisseminator) replyFetchRequest(source nodeID, clientID, reqNo uint64, digest []byte) *ActionList {
	c, ok := ct.client(clientID)
	if !ok {
		return &ActionList{}
	}

	if !c.inWatermarks(reqNo) {
		return &ActionList{}
	}

	creq := c.reqNo(reqNo)
	data, ok := creq.requests[string(digest)]
	if !ok {
		return &ActionList{}
	}

	if _, ok := data.agreements[nodeID(ct.myConfig.Id)]; !ok {
		return &ActionList{}
	}

	return (&ActionList{}).ForwardRequest(
		[]uint64{uint64(source)},
		&msgs.RequestAck{
			ClientId: clientID,
			ReqNo:    reqNo,
			Digest:   digest,
		},
	)
}

func (ct *clientHashDisseminator) ack(source nodeID, ack *msgs.RequestAck) (*ActionList, *clientRequest) {
	c, ok := ct.clients[ack.ClientId]
	assertEqual(ok, true, "the step filtering should delay reqs for non-existent clients")

	return c.ack(source, ack)
}

func (ct *clientHashDisseminator) client(clientID uint64) (*client, bool) {
	// TODO, we could do lazy initialization here
	c, ok := ct.clients[clientID]
	return c, ok
}

// clientReqNo accumulates acks for this request number
// and attempts to determine which ack is correct, and if the request
// we have is correct.  A replica only ever acks requests it knows to be correct,
// and has the data for.  This means it was injected locally via the Propose API,
// or, a weak quorum exists and something triggered a fetch of the request such
// as an epoch change, or tick based reconcilliation logic.  Because in both the epoch
// change, or reconcilliation paths at least some correct replica validated the
// request, we know the request is correct.  Once we observe two valid requests
// we know that the client is behaving in a byzantine way, and will allow a null
// request to be substituted.  Further, the replica will not ack any request other
// than the null request once two correct requests are observed.
// Additionally, a client may inject a null request via the propose API when attempting
// to recover from a crash without persistence, which will also cause other acks to cease.
// A correct replica will never ack two different non-null requests.  We therefore
// track which replicas have already acked a non-null request and ignore any further
// non-null acks.
type clientReqNo struct {
	myConfig        *state.EventInitialParameters
	networkConfig   *msgs.NetworkState_Config
	clientID        uint64
	reqNo           uint64
	validAfterSeqNo uint64
	nonNullVoters   map[nodeID]struct{}
	requests        map[string]*clientRequest // all requests, correct or not we've observed
	weakRequests    map[string]*clientRequest // all correct requests we have observed
	strongRequests  map[string]*clientRequest // strongly correct requests (at most 1 null, 1 non-null)
	myRequests      map[string]*clientRequest // requests we have persisted
	committed       bool
	acksSent        uint
	ticksSinceAck   uint
}

func newClientReqNo(myConfig *state.EventInitialParameters, clientID, reqNo uint64, networkConfig *msgs.NetworkState_Config, validAfterSeqNo uint64) *clientReqNo {

	return &clientReqNo{
		myConfig:        myConfig,
		clientID:        clientID,
		reqNo:           reqNo,
		networkConfig:   networkConfig,
		validAfterSeqNo: validAfterSeqNo,
		requests:        map[string]*clientRequest{},
		weakRequests:    map[string]*clientRequest{},
		strongRequests:  map[string]*clientRequest{},
		myRequests:      map[string]*clientRequest{},
		nonNullVoters:   map[nodeID]struct{}{},
	}
}

func (crn *clientReqNo) reinitialize(networkConfig *msgs.NetworkState_Config) {
	crn.networkConfig = networkConfig

	oldRequests := crn.requests

	crn.nonNullVoters = map[nodeID]struct{}{}
	crn.requests = map[string]*clientRequest{}
	crn.weakRequests = map[string]*clientRequest{}
	crn.strongRequests = map[string]*clientRequest{}
	crn.myRequests = map[string]*clientRequest{}

	digests := make([]string, len(oldRequests))
	i := 0
	for digest := range oldRequests {
		digests[i] = digest
		i++
	}
	sort.Slice(digests, func(i, j int) bool {
		return i <= j
	})

	for _, digest := range digests {
		oldClientReq := oldRequests[digest]
		for _, id := range networkConfig.Nodes {
			if _, ok := oldClientReq.agreements[nodeID(id)]; !ok {
				continue
			}

			crn.applyRequestAck(nodeID(id), oldClientReq.ack, true)
		}

		if oldClientReq.stored {
			newClientReq := crn.clientReq(oldClientReq.ack)
			newClientReq.stored = true
			crn.myRequests[digest] = newClientReq
		}
	}
}

func (crn *clientReqNo) clientReq(ack *msgs.RequestAck) *clientRequest {
	var digestKey string
	if len(ack.Digest) == 0 {
		digestKey = ""
	} else {
		digestKey = string(ack.Digest)
	}

	clientReq, ok := crn.requests[digestKey]
	if !ok {
		clientReq = &clientRequest{
			myConfig:   crn.myConfig,
			ack:        ack,
			agreements: map[nodeID]struct{}{},
		}
		crn.requests[digestKey] = clientReq
	}

	return clientReq
}

func (crn *clientReqNo) applyNewRequest(ack *msgs.RequestAck) {
	_, ok := crn.myRequests[string(ack.Digest)]
	if ok {
		// We have already persisted this request, likely
		// a race between a forward and a local proposal, do nothing
		return
	}

	clientReq := crn.clientReq(ack)
	clientReq.stored = true

	crn.myRequests[string(ack.Digest)] = clientReq
}

func (crn *clientReqNo) generateAck() *msgs.Msg {
	if len(crn.myRequests) == 0 {
		return nil
	}

	if len(crn.myRequests) == 1 {
		crn.acksSent = 1
		crn.ticksSinceAck = 0
		for _, cr := range crn.myRequests {
			return &msgs.Msg{
				Type: &msgs.Msg_RequestAck{
					RequestAck: cr.ack,
				},
			}
		}
	}

	nullAck := &msgs.RequestAck{
		ClientId: crn.clientID,
		ReqNo:    crn.reqNo,
	}

	nullReq := crn.clientReq(nullAck)
	nullReq.stored = true
	crn.myRequests[""] = nullReq

	crn.acksSent = 1
	crn.ticksSinceAck = 0

	return &msgs.Msg{
		Type: &msgs.Msg_RequestAck{
			RequestAck: nullAck,
		},
	}
}

func (crn *clientReqNo) applyRequestAck(source nodeID, ack *msgs.RequestAck, force bool) {
	if len(ack.Digest) != 0 {
		_, ok := crn.nonNullVoters[source]
		if !ok && !force {
			return
		}

		crn.nonNullVoters[source] = struct{}{}
	}

	clientReq := crn.clientReq(ack)
	clientReq.agreements[source] = struct{}{}

	if len(clientReq.agreements) < someCorrectQuorum(crn.networkConfig) {
		return
	}

	crn.weakRequests[string(ack.Digest)] = clientReq

	if len(clientReq.agreements) < intersectionQuorum(crn.networkConfig) {
		return
	}

	crn.strongRequests[string(ack.Digest)] = clientReq
}

func (crn *clientReqNo) tick() *ActionList {
	if crn.committed {
		return &ActionList{}
	}

	actions := &ActionList{}

	// First, if we have accumulated conflicting correct requests and not committed,
	// we switch to promoting the null request
	if _, ok := crn.myRequests[""]; !ok && len(crn.weakRequests) > 1 {
		nullAck := &msgs.RequestAck{
			ClientId: crn.clientID,
			ReqNo:    crn.reqNo,
		}

		nullReq := crn.clientReq(nullAck)
		nullReq.stored = true
		crn.myRequests[""] = nullReq

		crn.acksSent = 1
		crn.ticksSinceAck = 0

		actions.Send(
			crn.networkConfig.Nodes,
			&msgs.Msg{
				Type: &msgs.Msg_RequestAck{
					RequestAck: nullAck,
				},
			},
		).CorrectRequest(
			nullAck,
		)
	}

	// Second, if there is only one correct request, and we don't have it,
	// and it's been around long enough, let's go proactively fetch it.
	if len(crn.weakRequests) == 1 {
		correctFetchTicks := uint(4)
		for _, cr := range crn.weakRequests {
			if cr.stored || cr.fetching {
				break
			}

			if cr.ticksCorrect <= correctFetchTicks {
				cr.ticksCorrect++
				break
			}

			actions.concat(cr.fetch())
			break
		}
	}

	var toFetch []*clientRequest

	// Third, for every correct request we have, if we have been trying to fetch it
	// long enough, but received no response, let's try fetching it again.
	for _, cr := range crn.weakRequests {
		if !cr.fetching {
			continue
		}

		fetchTimeoutTicks := uint(4) // TODO make configurable

		if cr.ticksFetching <= fetchTimeoutTicks {
			cr.ticksFetching++
			continue
		}

		cr.fetching = false

		toFetch = append(toFetch, cr)
	}

	sort.Slice(toFetch, func(i, j int) bool {
		return bytes.Compare(toFetch[i].ack.Digest, toFetch[j].ack.Digest) > 0
	})

	for _, cr := range toFetch {
		actions.concat(cr.fetch())
	}

	// Finally, if we have sent any acks, and it has been long enough, we re-send.
	// Since it's possible the client did not send the request to enough parties,
	// we perform a linear backoff, waiting an additional interval longer after each re-ack
	ackResendTicks := uint(20) // TODO make configurable

	if crn.acksSent == 0 {
		return actions
	}

	if crn.ticksSinceAck != crn.acksSent*ackResendTicks {
		crn.ticksSinceAck++
		return actions
	}

	var ack *msgs.RequestAck
	switch {
	case len(crn.myRequests) > 1:
		ack = crn.myRequests[""].ack
	case len(crn.myRequests) == 1:
		for _, cr := range crn.myRequests {
			ack = cr.ack
			break
		}
	default:
		panic("we have sent an ack for a request, but do not have the ack")
	}

	crn.acksSent++
	crn.ticksSinceAck = 0

	actions.Send(
		crn.networkConfig.Nodes,
		&msgs.Msg{
			Type: &msgs.Msg_RequestAck{
				RequestAck: ack,
			},
		},
	)

	return actions
}

type clientRequest struct {
	myConfig      *state.EventInitialParameters
	ack           *msgs.RequestAck
	agreements    map[nodeID]struct{}
	stored        bool // set when the request is persisted locally
	fetching      bool // set when we have sent a request for this request
	ticksFetching uint // incremented by one each tick while fetching is true
	ticksCorrect  uint // incremented by one each tick while not stored
}

func (cr *clientRequest) fetch() *ActionList {
	if cr.fetching {
		return &ActionList{}
	}

	// TODO, with access to network config, we could pick f+1
	nodes := make([]uint64, len(cr.agreements))
	i := 0
	for nodeID := range cr.agreements {
		nodes[i] = uint64(nodeID)
		i++
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i] <= nodes[j]
	})

	cr.fetching = true
	cr.ticksFetching = 0

	return (&ActionList{}).Send(
		nodes,
		&msgs.Msg{
			Type: &msgs.Msg_FetchRequest{
				FetchRequest: cr.ack,
			},
		},
	)
}

type client struct {
	myConfig      *state.EventInitialParameters
	logger        Logger
	networkConfig *msgs.NetworkState_Config
	clientState   *msgs.NetworkState_Client
	clientTracker *clientTracker
	highWatermark uint64 // TODO, remove, it's just convenient for the moment
	nextReadyMark uint64
	nextAckMark   uint64

	reqNoList *list.List
	reqNoMap  map[uint64]*list.Element
}

func newClient(myConfig *state.EventInitialParameters, logger Logger, tracker *clientTracker) *client {
	return &client{
		myConfig:      myConfig,
		logger:        logger,
		clientTracker: tracker,
	}
}

func (c *client) reinitialize(seqNo uint64, networkConfig *msgs.NetworkState_Config, clientState *msgs.NetworkState_Client, reconfiguring bool) *ActionList {
	actions := &ActionList{}
	oldReqNoMap := c.reqNoMap

	intermediateHighWatermark := clientState.LowWatermark + uint64(clientState.Width) - uint64(clientState.WidthConsumedLastCheckpoint)

	c.networkConfig = networkConfig
	c.clientState = clientState
	if !reconfiguring {
		c.highWatermark = clientState.LowWatermark + uint64(clientState.Width)
	} else {
		c.highWatermark = intermediateHighWatermark
	}
	c.nextReadyMark = clientState.LowWatermark
	if c.nextAckMark < clientState.LowWatermark {
		c.nextAckMark = clientState.LowWatermark
	}
	c.reqNoList = list.New()
	c.reqNoMap = map[uint64]*list.Element{}

	for reqNo := clientState.LowWatermark; reqNo <= c.highWatermark; reqNo++ {
		var crn *clientReqNo

		committed := isCommitted(reqNo, clientState)

		if oldReqNoEl, ok := oldReqNoMap[reqNo]; ok {
			crn = oldReqNoEl.Value.(*clientReqNo)
		} else {
			var validAfterSeqNo uint64
			if reqNo > intermediateHighWatermark {
				validAfterSeqNo = seqNo + uint64(networkConfig.CheckpointInterval)
			} else {
				validAfterSeqNo = seqNo
			}
			crn = newClientReqNo(c.myConfig, clientState.Id, reqNo, c.networkConfig, validAfterSeqNo)
			actions.AllocateRequest(clientState.Id, reqNo)
		}

		crn.committed = committed

		crn.reinitialize(networkConfig)

		el := c.reqNoList.PushBack(crn)
		c.reqNoMap[reqNo] = el
	}

	c.advanceReady()

	c.logger.Log(LevelDebug, "reinitialized client", "client_id", c.clientState.Id, "low_watermark", c.clientState.LowWatermark, "high_watermark", c.highWatermark, "next_ready_mark", c.nextReadyMark, "next_ack_mark", c.nextAckMark)

	return actions
}

func (c *client) allocate(seqNo uint64, state *msgs.NetworkState_Client, reconfiguring bool) *ActionList {
	actions := &ActionList{}

	intermediateHighWatermark := state.LowWatermark + uint64(state.Width) - uint64(state.WidthConsumedLastCheckpoint)
	assertEqualf(intermediateHighWatermark, c.highWatermark, "new intermediate high watermark should always be the old high watemark, in the allocation path", state.Id)
	var newHighWatermark uint64
	if !reconfiguring {
		newHighWatermark = state.LowWatermark + uint64(state.Width)
	} else {
		newHighWatermark = intermediateHighWatermark
	}

	if state.LowWatermark > c.nextReadyMark {
		// It's possible that a request we never saw as ready commits
		// because it was correct, so advance the ready mark
		c.nextReadyMark = state.LowWatermark
	}

	if state.LowWatermark > c.nextAckMark {
		c.nextAckMark = state.LowWatermark
	}

	for el := c.reqNoList.Front(); el != nil; {
		crn := el.Value.(*clientReqNo)
		if crn.reqNo == state.LowWatermark {
			break
		}

		oel := el
		el = el.Next()

		c.reqNoList.Remove(oel)
		delete(c.reqNoMap, crn.reqNo)
	}

	for reqNo := state.LowWatermark; reqNo <= c.highWatermark; reqNo++ {
		if isCommitted(reqNo, state) {
			crn := c.reqNoMap[reqNo].Value.(*clientReqNo)
			crn.committed = true
		}
	}

	c.clientState = state

	// per the assertions above, the intermediateHighWatermark is always the
	// old high watermark, so allocate starting after that one.
	validAfterSeqNo := seqNo + uint64(c.networkConfig.CheckpointInterval)
	for reqNo := intermediateHighWatermark + 1; reqNo <= newHighWatermark; reqNo++ {
		actions.AllocateRequest(state.Id, reqNo)
		el := c.reqNoList.PushBack(newClientReqNo(c.myConfig, state.Id, reqNo, c.networkConfig, validAfterSeqNo))
		c.reqNoMap[reqNo] = el
	}

	c.highWatermark = newHighWatermark
	c.advanceReady()

	c.logger.Log(LevelDebug, "allocated new reqs for client", "client_id", c.clientState.Id, "low_watermark", c.clientState.LowWatermark, "high_watermark", c.highWatermark, "next_ready_mark", c.nextReadyMark)

	return actions
}

func (c *client) ack(source nodeID, ack *msgs.RequestAck) (*ActionList, *clientRequest) {
	actions := &ActionList{}
	crne, ok := c.reqNoMap[ack.ReqNo]
	assertEqualf(ok, true, "client_id=%d got ack for req_no=%d, but lowWatermark=%d highWatermark=%d", c.clientState.Id, ack.ReqNo, c.clientState.LowWatermark, c.highWatermark)

	crn := crne.Value.(*clientReqNo)

	cr := crn.clientReq(ack)
	cr.agreements[source] = struct{}{}

	newlyCorrect := len(cr.agreements) == someCorrectQuorum(c.networkConfig)
	if newlyCorrect {
		crn.weakRequests[string(ack.Digest)] = cr

		if !cr.stored {
			// If we already have the req stored, we know it's correct
			actions.CorrectRequest(ack)
		}
	}

	correctAndMyAck := len(cr.agreements) >= someCorrectQuorum(c.networkConfig) && uint64(source) == c.myConfig.Id
	if cr.stored && (newlyCorrect || correctAndMyAck) {
		// This request just became 'available', add it to the list
		c.clientTracker.addAvailable(ack)
	}

	if len(cr.agreements) == intersectionQuorum(c.networkConfig) {
		crn.strongRequests[string(ack.Digest)] = cr

		// Check to see if this request just becoming 'ready' can advance the ready mark
		c.advanceReady()
	}

	return actions, cr
}

func (c *client) inWatermarks(reqNo uint64) bool {
	return reqNo <= c.highWatermark && reqNo >= c.clientState.LowWatermark
}

func (c *client) reqNo(reqNo uint64) *clientReqNo {
	el := c.reqNoMap[reqNo]
	assertNotEqualf(el, nil, "client_id=%d should have req_no=%d but does not", c.clientState.Id, reqNo)
	return el.Value.(*clientReqNo)
}

func (c *client) advanceReady() {
	for i := c.nextReadyMark; i <= c.highWatermark; i++ {
		if i != c.nextReadyMark {
			// last time through the loop, we must not have updated the ready mark
			return
		}

		crn := c.reqNo(i)
		if crn.committed {
			c.nextReadyMark = i + 1
			continue
		}

		for digest := range crn.strongRequests {
			if _, ok := crn.myRequests[digest]; !ok {
				continue
			}

			c.clientTracker.addReady(crn)
			c.nextReadyMark = i + 1

			break
		}
	}
}

func (c *client) advanceAcks() *ActionList {
	actions := &ActionList{}
	for i := c.nextAckMark; i <= c.highWatermark; i++ {
		ack := c.reqNo(i).generateAck()
		if ack == nil {
			break
		}

		actions.Send(
			c.networkConfig.Nodes,
			ack,
		)

		c.nextAckMark = i + 1
	}

	return actions
}

func (c *client) tick() *ActionList {
	actions := &ActionList{}
	for el := c.reqNoList.Front(); el != nil; el = el.Next() {
		crn := el.Value.(*clientReqNo)
		actions.concat(crn.tick())
	}
	return actions
}

func (c *client) status() *status.ClientTracker {
	allocated := make([]uint64, c.reqNoList.Len())
	i := 0
	lastNonZero := 0
	for el := c.reqNoList.Front(); el != nil; el = el.Next() {
		crn := el.Value.(*clientReqNo)
		if crn.committed {
			allocated[i] = 2 // TODO, actually report the seqno it committed to
			lastNonZero = i
		} else if len(crn.requests) > 0 {
			allocated[i] = 1
			lastNonZero = i
		}
		i++
	}

	return &status.ClientTracker{
		ClientID:      c.clientState.Id,
		LowWatermark:  c.clientState.LowWatermark,
		HighWatermark: c.highWatermark,
		Allocated:     allocated[:lastNonZero],
	}
}
