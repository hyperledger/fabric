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

package discovery

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
)

const defaultHelloInterval = time.Duration(5) * time.Second

var aliveTimeInterval = defaultHelloInterval
var aliveExpirationTimeout = 5 * aliveTimeInterval
var aliveExpirationCheckInterval = time.Duration(aliveExpirationTimeout / 10)
var reconnectInterval = aliveExpirationTimeout

// SetAliveTimeInternal sets the alive time interval
func SetAliveTimeInternal(interval time.Duration) {
	aliveTimeInterval = interval
}

// SetExpirationTimeout sets the expiration timeout
func SetExpirationTimeout(timeout time.Duration) {
	aliveExpirationTimeout = timeout
}

// SetAliveExpirationCheckInterval sets the expiration check interval
func SetAliveExpirationCheckInterval(interval time.Duration) {
	aliveExpirationCheckInterval = interval
}

// SetReconnectInterval sets the reconnect interval
func SetReconnectInterval(interval time.Duration) {
	reconnectInterval = interval
}

type timestamp struct {
	incTime  time.Time
	seqNum   uint64
	lastSeen time.Time
}

func (ts *timestamp) String() string {
	return fmt.Sprintf("%v, %v", ts.incTime.UnixNano(), ts.seqNum)
}

type gossipDiscoveryImpl struct {
	incTime         uint64
	seqNum          uint64
	self            NetworkMember
	deadLastTS      map[string]*timestamp     // H
	aliveLastTS     map[string]*timestamp     // V
	id2Member       map[string]*NetworkMember // all known members
	aliveMembership membershipStore
	deadMembership  membershipStore

	bootstrapPeers []string

	comm  CommService
	crypt CryptoService
	lock  *sync.RWMutex

	toDieChan chan struct{}
	toDieFlag int32
	logger    *logging.Logger
}

// NewDiscoveryService returns a new discovery service with the comm module passed and the crypto service passed
func NewDiscoveryService(bootstrapPeers []string, self NetworkMember, comm CommService, crypt CryptoService) Discovery {
	d := &gossipDiscoveryImpl{
		self:            self,
		incTime:         uint64(time.Now().UnixNano()),
		seqNum:          uint64(0),
		deadLastTS:      make(map[string]*timestamp),
		aliveLastTS:     make(map[string]*timestamp),
		id2Member:       make(map[string]*NetworkMember),
		aliveMembership: make(membershipStore, 0),
		deadMembership:  make(membershipStore, 0),
		crypt:           crypt,
		comm:            comm,
		lock:            &sync.RWMutex{},
		toDieChan:       make(chan struct{}, 1),
		toDieFlag:       int32(0),
		logger:          util.GetLogger(util.LoggingDiscoveryModule, self.InternalEndpoint.Endpoint),
	}

	go d.periodicalSendAlive()
	go d.periodicalCheckAlive()
	go d.handleMessages()
	go d.periodicalReconnectToDead()
	go d.handlePresumedDeadPeers()

	go d.connect2BootstrapPeers(bootstrapPeers)

	d.logger.Info("Started", self, "incTime is", d.incTime)

	return d
}

// Exists returns whether a peer with given
// PKI-ID is known
func (d *gossipDiscoveryImpl) Exists(PKIID common.PKIidType) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()
	_, exists := d.id2Member[string(PKIID)]
	return exists
}

func (d *gossipDiscoveryImpl) Connect(member NetworkMember) {
	d.logger.Debug("Entering", member)
	defer d.logger.Debug("Exiting")

	if member.PKIid == nil {
		d.logger.Warning("Empty PkiID, aborting")
		return
	}

	d.lock.Lock()
	defer d.lock.Unlock()

	if _, exists := d.id2Member[string(member.PKIid)]; exists {
		d.logger.Info("Member", member, "already known")
		return
	}

	d.deadLastTS[string(member.PKIid)] = &timestamp{
		incTime:  time.Unix(0, 0),
		lastSeen: time.Now(),
		seqNum:   0,
	}
	d.id2Member[string(member.PKIid)] = &member
}

func (d *gossipDiscoveryImpl) connect2BootstrapPeers(endpoints []string) {
	d.logger.Info("Entering:", endpoints)
	defer d.logger.Info("Exiting")

	for !d.somePeerIsKnown() {
		var wg sync.WaitGroup
		req := d.createMembershipRequest()
		wg.Add(len(endpoints))
		for _, endpoint := range endpoints {
			go func(endpoint string) {
				defer wg.Done()
				peer := &NetworkMember{
					Endpoint: endpoint,
					InternalEndpoint: &proto.SignedEndpoint{
						Endpoint: endpoint,
					},
				}
				if !d.comm.Ping(peer) {
					return
				}
				d.comm.SendToPeer(peer, req)
			}(endpoint)
		}
		wg.Wait()
		time.Sleep(reconnectInterval)
	}
}

func (d *gossipDiscoveryImpl) somePeerIsKnown() bool {
	d.lock.RLock()
	defer d.lock.RUnlock()
	return len(d.aliveLastTS) != 0
}

func (d *gossipDiscoveryImpl) InitiateSync(peerNum int) {
	if d.toDie() {
		return
	}
	var peers2SendTo []*NetworkMember
	memReq := d.createMembershipRequest()

	d.lock.RLock()

	n := len(d.aliveMembership)
	k := peerNum
	if k > n {
		k = n
	}

	aliveMembersAsSlice := d.aliveMembership.ToSlice()
	for _, i := range util.GetRandomIndices(k, n-1) {
		pulledPeer := aliveMembersAsSlice[i].GetAliveMsg().Membership
		netMember := &NetworkMember{
			Endpoint:         pulledPeer.Endpoint,
			Metadata:         pulledPeer.Metadata,
			PKIid:            pulledPeer.PkiID,
			InternalEndpoint: pulledPeer.InternalEndpoint,
		}
		peers2SendTo = append(peers2SendTo, netMember)
	}

	d.lock.RUnlock()

	for _, netMember := range peers2SendTo {
		d.comm.SendToPeer(netMember, memReq)
	}
}

func (d *gossipDiscoveryImpl) handlePresumedDeadPeers() {
	defer d.logger.Debug("Stopped")

	for !d.toDie() {
		select {
		case deadPeer := <-d.comm.PresumedDead():
			if d.isAlive(deadPeer) {
				d.expireDeadMembers([]common.PKIidType{deadPeer})
			}
			break
		case s := <-d.toDieChan:
			d.toDieChan <- s
			return
		}
	}
}

func (d *gossipDiscoveryImpl) isAlive(pkiID common.PKIidType) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()
	_, alive := d.aliveLastTS[string(pkiID)]
	return alive
}

func (d *gossipDiscoveryImpl) handleMessages() {
	defer d.logger.Debug("Stopped")

	in := d.comm.Accept()
	for !d.toDie() {
		select {
		case s := <-d.toDieChan:
			d.toDieChan <- s
			return
		case m := <-in:
			d.handleMsgFromComm(m)
			break
		}
	}
}

func (d *gossipDiscoveryImpl) handleMsgFromComm(m *proto.GossipMessage) {
	if m == nil {
		return
	}
	if m.GetAliveMsg() == nil && m.GetMemRes() == nil && m.GetMemReq() == nil {
		d.logger.Warning("Got message with wrong type (expected Alive or MembershipResponse or MembershipRequest message):", m.Content) // TODO: write only message type
		d.logger.Warning(m)
		return
	}

	d.logger.Debug("Got message:", m)
	defer d.logger.Debug("Exiting")

	// TODO: make sure somehow that the membership request is "fresh"
	if memReq := m.GetMemReq(); memReq != nil {
		d.handleAliveMessage(memReq.SelfInformation)
		// Sending a membership response to a peer may block this routine
		// in case the sending is deliberately slow (i.e attack).
		// will keep this async until I'll write a timeout detector in the comm layer
		go d.sendMemResponse(memReq.SelfInformation.GetAliveMsg().Membership, memReq.Known)
		return
	}

	if m.IsAliveMsg() {
		d.handleAliveMessage(m)
		return
	}

	if memResp := m.GetMemRes(); memResp != nil {
		for _, am := range memResp.Alive {
			if !am.IsAliveMsg() {
				d.logger.Warning("Expected alive message, got", am, "instead")
				return
			}
			d.handleAliveMessage(am)
		}

		for _, dm := range memResp.Dead {
			if !d.crypt.ValidateAliveMsg(m) {
				d.logger.Warningf("Alive message isn't authentic, someone spoofed %s's identity", dm.GetAliveMsg().Membership)
				continue
			}

			newDeadMembers := []*proto.GossipMessage{}
			d.lock.RLock()
			if _, known := d.id2Member[string(dm.GetAliveMsg().Membership.PkiID)]; !known {
				newDeadMembers = append(newDeadMembers, dm)
			}
			d.lock.RUnlock()
			d.learnNewMembers([]*proto.GossipMessage{}, newDeadMembers)
		}
	}
}

func (d *gossipDiscoveryImpl) sendMemResponse(member *proto.Member, known [][]byte) {
	d.logger.Debug("Entering", member)

	memResp := d.createMembershipResponse(known)

	defer d.logger.Debug("Exiting, replying with", memResp)

	d.comm.SendToPeer(&NetworkMember{
		Endpoint:         member.Endpoint,
		Metadata:         member.Metadata,
		PKIid:            member.PkiID,
		InternalEndpoint: member.InternalEndpoint,
	}, &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: uint64(0),
		Content: &proto.GossipMessage_MemRes{
			MemRes: memResp,
		},
	})
}

func (d *gossipDiscoveryImpl) createMembershipResponse(known [][]byte) *proto.MembershipResponse {
	aliveMsg := d.createAliveMessage()

	d.lock.RLock()
	defer d.lock.RUnlock()

	deadPeers := []*proto.GossipMessage{}

	for _, dm := range d.deadMembership.ToSlice() {
		isKnown := false
		for _, knownPeer := range known {
			if equalPKIid(knownPeer, dm.GetAliveMsg().Membership.PkiID) {
				isKnown = true
				break
			}
		}
		if !isKnown {
			deadPeers = append(deadPeers, dm.GossipMessage)
			break
		}
	}

	aliveMembersAsSlice := d.aliveMembership.ToSlice()
	aliveSnapshot := make([]*proto.GossipMessage, len(aliveMembersAsSlice))
	for i, msg := range aliveMembersAsSlice {
		aliveSnapshot[i] = msg.GossipMessage
	}

	return &proto.MembershipResponse{
		Alive: append(aliveSnapshot, aliveMsg),
		Dead:  deadPeers,
	}
}

func (d *gossipDiscoveryImpl) handleAliveMessage(m *proto.GossipMessage) {
	d.logger.Debug("Entering", m)
	defer d.logger.Debug("Exiting")

	if !d.crypt.ValidateAliveMsg(m) {
		d.logger.Warningf("Alive message isn't authentic, someone must be spoofing %s's identity", m.GetAliveMsg())
		return
	}

	pkiID := m.GetAliveMsg().Membership.PkiID
	if equalPKIid(pkiID, d.self.PKIid) {
		d.logger.Debug("Got alive message about ourselves,", m)
		return
	}

	ts := m.GetAliveMsg().Timestamp

	d.lock.RLock()
	_, known := d.id2Member[string(pkiID)]
	d.lock.RUnlock()

	if !known {
		d.learnNewMembers([]*proto.GossipMessage{m}, []*proto.GossipMessage{})
		return
	}

	d.lock.RLock()
	lastAliveTS, isAlive := d.aliveLastTS[string(pkiID)]
	lastDeadTS, isDead := d.deadLastTS[string(pkiID)]
	d.lock.RUnlock()

	if !isAlive && !isDead {
		d.logger.Panicf("Member %s is known but not found neither in alive nor in dead lastTS maps, isAlive=%v, isDead=%v", m.GetAliveMsg().Membership.Endpoint, isAlive, isDead)
		return
	}

	if isAlive && isDead {
		d.logger.Panicf("Member %s is both alive and dead at the same time", m.GetAliveMsg().Membership)
		return
	}

	if isDead {
		if before(lastDeadTS, ts) {
			// resurrect peer
			d.resurrectMember(m, *ts)
		} else if !same(lastDeadTS, ts) {
			d.logger.Debug(m.GetAliveMsg().Membership, "lastDeadTS:", lastDeadTS, "but got ts:", ts)
		}
		return
	}

	d.lock.RLock()
	lastAliveTS, isAlive = d.aliveLastTS[string(pkiID)]
	d.lock.RUnlock()

	if isAlive {
		if before(lastAliveTS, ts) {
			d.learnExistingMembers([]*proto.GossipMessage{m})
		} else if !same(lastAliveTS, ts) {
			d.logger.Debug(m.GetAliveMsg().Membership, "lastAliveTS:", lastAliveTS, "but got ts:", ts)
		}

	}
	// else, ignore the message because it is too old
}

func (d *gossipDiscoveryImpl) resurrectMember(am *proto.GossipMessage, t proto.PeerTime) {
	d.logger.Info("Entering, AliveMessage:", am, "t:", t)
	defer d.logger.Info("Exiting")
	d.lock.Lock()
	defer d.lock.Unlock()

	member := am.GetAliveMsg().Membership
	pkiID := member.PkiID
	d.aliveLastTS[string(pkiID)] = &timestamp{
		lastSeen: time.Now(),
		seqNum:   t.SeqNum,
		incTime:  tsToTime(t.IncNumber),
	}

	d.id2Member[string(pkiID)] = &NetworkMember{
		Endpoint:         member.Endpoint,
		Metadata:         member.Metadata,
		PKIid:            member.PkiID,
		InternalEndpoint: member.InternalEndpoint,
	}

	delete(d.deadLastTS, string(pkiID))
	d.deadMembership.Remove(common.PKIidType(pkiID))
	d.aliveMembership.Put(common.PKIidType(pkiID), &message{GossipMessage: am})
}

func (d *gossipDiscoveryImpl) periodicalReconnectToDead() {
	defer d.logger.Debug("Stopped")

	for !d.toDie() {
		wg := &sync.WaitGroup{}

		for _, member := range d.copyLastSeen(d.deadLastTS) {
			wg.Add(1)
			go func(member NetworkMember) {
				defer wg.Done()
				if d.comm.Ping(&member) {
					d.logger.Debug(member, "is responding, sending membership request")
					d.sendMembershipRequest(&member)
				} else {
					d.logger.Debug(member, "is still dead")
				}
			}(member)
		}

		wg.Wait()
		d.logger.Debug("Sleeping", reconnectInterval)
		time.Sleep(reconnectInterval)
	}
}

func (d *gossipDiscoveryImpl) sendMembershipRequest(member *NetworkMember) {
	d.comm.SendToPeer(member, d.createMembershipRequest())
}

func (d *gossipDiscoveryImpl) createMembershipRequest() *proto.GossipMessage {
	req := &proto.MembershipRequest{
		SelfInformation: d.createAliveMessage(),
		Known:           d.getKnownPeers(),
	}
	return &proto.GossipMessage{
		Tag:   proto.GossipMessage_EMPTY,
		Nonce: uint64(0),
		Content: &proto.GossipMessage_MemReq{
			MemReq: req,
		},
	}
}

func (d *gossipDiscoveryImpl) getKnownPeers() [][]byte {
	d.lock.RLock()
	defer d.lock.RUnlock()

	peers := [][]byte{}
	for id := range d.id2Member {
		peers = append(peers, common.PKIidType(id))
	}
	return peers
}

func (d *gossipDiscoveryImpl) copyLastSeen(lastSeenMap map[string]*timestamp) []NetworkMember {
	d.lock.RLock()
	defer d.lock.RUnlock()

	res := []NetworkMember{}
	for pkiIDStr := range lastSeenMap {
		res = append(res, *(d.id2Member[pkiIDStr]))
	}
	return res
}

func (d *gossipDiscoveryImpl) periodicalCheckAlive() {
	defer d.logger.Debug("Stopped")

	for !d.toDie() {
		time.Sleep(aliveExpirationCheckInterval)
		dead := d.getDeadMembers()
		if len(dead) > 0 {
			d.logger.Debugf("Got %v dead members: %v", len(dead), dead)
			d.expireDeadMembers(dead)
		}
	}
}

func (d *gossipDiscoveryImpl) expireDeadMembers(dead []common.PKIidType) {
	d.logger.Warning("Entering", dead)
	defer d.logger.Warning("Exiting")

	var deadMembers2Expire []*NetworkMember

	d.lock.Lock()

	for _, pkiID := range dead {
		if _, isAlive := d.aliveLastTS[string(pkiID)]; !isAlive {
			continue
		}
		deadMembers2Expire = append(deadMembers2Expire, d.id2Member[string(pkiID)])
		// move lastTS from alive to dead
		lastTS, hasLastTS := d.aliveLastTS[string(pkiID)]
		if hasLastTS {
			d.deadLastTS[string(pkiID)] = lastTS
			delete(d.aliveLastTS, string(pkiID))
		}

		if am := d.aliveMembership.msgByID(pkiID); am != nil {
			d.deadMembership.Put(pkiID, am)
			d.aliveMembership.Remove(pkiID)
		}
	}

	d.lock.Unlock()

	for _, member2Expire := range deadMembers2Expire {
		d.logger.Warning("Closing connection to", member2Expire)
		d.comm.CloseConn(member2Expire)
	}
}

func (d *gossipDiscoveryImpl) getDeadMembers() []common.PKIidType {
	d.lock.RLock()
	defer d.lock.RUnlock()

	dead := []common.PKIidType{}
	for id, last := range d.aliveLastTS {
		elapsedNonAliveTime := time.Since(last.lastSeen)
		if elapsedNonAliveTime.Nanoseconds() > aliveExpirationTimeout.Nanoseconds() {
			d.logger.Warning("Haven't heard from", id, "for", elapsedNonAliveTime)
			dead = append(dead, common.PKIidType(id))
		}
	}
	return dead
}

func (d *gossipDiscoveryImpl) periodicalSendAlive() {
	defer d.logger.Debug("Stopped")

	for !d.toDie() {
		d.logger.Debug("Sleeping", aliveTimeInterval)
		time.Sleep(aliveTimeInterval)
		d.comm.Gossip(d.createAliveMessage())
	}
}

func (d *gossipDiscoveryImpl) createAliveMessage() *proto.GossipMessage {
	d.lock.Lock()
	d.seqNum++
	seqNum := d.seqNum

	endpoint := d.self.Endpoint
	meta := d.self.Metadata
	pkiID := d.self.PKIid
	internalEndpoint := d.self.InternalEndpoint

	d.lock.Unlock()

	msg2Gossip := &proto.GossipMessage{
		Tag: proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_AliveMsg{
			AliveMsg: &proto.AliveMessage{
				Membership: &proto.Member{
					Endpoint:         endpoint,
					Metadata:         meta,
					PkiID:            pkiID,
					InternalEndpoint: internalEndpoint,
				},
				Timestamp: &proto.PeerTime{
					IncNumber: uint64(d.incTime),
					SeqNum:    seqNum,
				},
			},
		},
	}

	return d.crypt.SignMessage(msg2Gossip)
}

func (d *gossipDiscoveryImpl) learnExistingMembers(aliveArr []*proto.GossipMessage) {
	d.logger.Infof("Entering: learnedMembers={%v}", aliveArr)
	defer d.logger.Debug("Exiting")

	d.lock.Lock()
	defer d.lock.Unlock()

	for _, m := range aliveArr {
		am := m.GetAliveMsg()
		if m == nil {
			d.logger.Warning("Expected alive message, got instead:", m)
			return
		}
		d.logger.Debug("updating", am)
		// update member's data
		member := d.id2Member[string(am.Membership.PkiID)]
		member.Endpoint = am.Membership.Endpoint
		member.Metadata = am.Membership.Metadata
		member.InternalEndpoint = am.Membership.InternalEndpoint

		if _, isKnownAsDead := d.deadLastTS[string(am.Membership.PkiID)]; isKnownAsDead {
			d.logger.Warning(am.Membership, "has already expired")
			continue
		}

		if _, isKnownAsAlive := d.aliveLastTS[string(am.Membership.PkiID)]; !isKnownAsAlive {
			d.logger.Warning(am.Membership, "has already expired")
			continue
		} else {
			d.logger.Debug("Updating aliveness data:", am)
			// update existing aliveness data
			alive := d.aliveLastTS[string(am.Membership.PkiID)]
			alive.incTime = tsToTime(am.Timestamp.IncNumber)
			alive.lastSeen = time.Now()
			alive.seqNum = am.Timestamp.SeqNum

			if am := d.aliveMembership.msgByID(m.GetAliveMsg().Membership.PkiID); am == nil {
				d.logger.Debug("Appended", am, "to d.cachedMembership.Alive")
				d.aliveMembership.Put(m.GetAliveMsg().Membership.PkiID, &message{GossipMessage: m})
			} else {
				d.logger.Debug("Replaced", am, "in d.cachedMembership.Alive")
				am.GossipMessage = m
			}
		}
	}
}

func (d *gossipDiscoveryImpl) learnNewMembers(aliveMembers []*proto.GossipMessage, deadMembers []*proto.GossipMessage) {
	d.logger.Debugf("Entering: learnedMembers={%v}, deadMembers={%v}", aliveMembers, deadMembers)
	defer d.logger.Debugf("Exiting")

	d.lock.Lock()
	defer d.lock.Unlock()

	for _, am := range aliveMembers {
		if equalPKIid(am.GetAliveMsg().Membership.PkiID, d.self.PKIid) {
			continue
		}
		d.aliveLastTS[string(am.GetAliveMsg().Membership.PkiID)] = &timestamp{
			incTime:  tsToTime(am.GetAliveMsg().Timestamp.IncNumber),
			lastSeen: time.Now(),
			seqNum:   am.GetAliveMsg().Timestamp.SeqNum,
		}

		d.aliveMembership.Put(am.GetAliveMsg().Membership.PkiID, &message{GossipMessage: am})
		d.logger.Infof("Learned about a new alive member: %v", am)
	}

	for _, dm := range deadMembers {
		if equalPKIid(dm.GetAliveMsg().Membership.PkiID, d.self.PKIid) {
			continue
		}
		d.deadLastTS[string(dm.GetAliveMsg().Membership.PkiID)] = &timestamp{
			incTime:  tsToTime(dm.GetAliveMsg().Timestamp.IncNumber),
			lastSeen: time.Now(),
			seqNum:   dm.GetAliveMsg().Timestamp.SeqNum,
		}

		d.deadMembership.Put(dm.GetAliveMsg().Membership.PkiID, &message{GossipMessage: dm})
		d.logger.Infof("Learned about a new dead member: %v", dm)
	}

	// update the member in any case
	for _, a := range [][]*proto.GossipMessage{aliveMembers, deadMembers} {
		for _, m := range a {
			member := m.GetAliveMsg()
			if member == nil {
				d.logger.Warning("Expected alive message, got instead:", m)
				return
			}
			d.id2Member[string(member.Membership.PkiID)] = &NetworkMember{
				Endpoint:         member.Membership.Endpoint,
				Metadata:         member.Membership.Metadata,
				PKIid:            member.Membership.PkiID,
				InternalEndpoint: member.Membership.InternalEndpoint,
			}
		}
	}
}

func (d *gossipDiscoveryImpl) GetMembership() []NetworkMember {
	if d.toDie() {
		return []NetworkMember{}
	}
	d.lock.RLock()
	defer d.lock.RUnlock()

	response := []NetworkMember{}
	for _, m := range d.aliveMembership.ToSlice() {
		member := m.GetAliveMsg()
		response = append(response, NetworkMember{
			PKIid:            member.Membership.PkiID,
			Endpoint:         member.Membership.Endpoint,
			Metadata:         member.Membership.Metadata,
			InternalEndpoint: member.Membership.InternalEndpoint,
		})
	}
	return response

}

func tsToTime(ts uint64) time.Time {
	return time.Unix(int64(0), int64(ts))
}

func (d *gossipDiscoveryImpl) UpdateMetadata(md []byte) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.self.Metadata = md
}

func (d *gossipDiscoveryImpl) UpdateEndpoint(endpoint string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.self.Endpoint = endpoint
}

func (d *gossipDiscoveryImpl) Self() NetworkMember {
	return NetworkMember{
		Endpoint:         d.self.Endpoint,
		Metadata:         d.self.Metadata,
		PKIid:            d.self.PKIid,
		InternalEndpoint: d.self.InternalEndpoint,
	}
}

func (d *gossipDiscoveryImpl) toDie() bool {
	toDie := atomic.LoadInt32(&d.toDieFlag) == int32(1)
	return toDie
}

func (d *gossipDiscoveryImpl) Stop() {
	defer d.logger.Info("Stopped")
	d.logger.Info("Stopping")
	atomic.StoreInt32(&d.toDieFlag, int32(1))
	d.toDieChan <- struct{}{}
}

func equalPKIid(a, b common.PKIidType) bool {
	return bytes.Equal(a, b)
}

func same(a *timestamp, b *proto.PeerTime) bool {
	return uint64(a.incTime.UnixNano()) == b.IncNumber && a.seqNum == b.SeqNum
}

func before(a *timestamp, b *proto.PeerTime) bool {
	return (uint64(a.incTime.UnixNano()) == b.IncNumber && a.seqNum < b.SeqNum) ||
		uint64(a.incTime.UnixNano()) < b.IncNumber
}
