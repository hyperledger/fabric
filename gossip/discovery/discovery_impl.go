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
	"github.com/hyperledger/fabric/protos/gossip"
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

func samePKIidAliveMessage(a interface{}, b interface{}) bool {
	return equalPKIid(a.(*proto.GossipMessage).GetAliveMsg().Membership.PkiID, b.(*proto.GossipMessage).GetAliveMsg().Membership.PkiID)
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
	pkiID    common.PKIidType
	endpoint string
	incTime  uint64
	metadata []byte

	seqNum uint64

	deadLastTS       map[string]*timestamp     // H
	aliveLastTS      map[string]*timestamp     // V
	id2Member        map[string]*NetworkMember // all known members
	cachedMembership *proto.MembershipResponse

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
		endpoint:    self.Endpoint,
		incTime:     uint64(time.Now().UnixNano()),
		metadata:    self.Metadata,
		pkiID:       self.PKIid,
		seqNum:      uint64(0),
		deadLastTS:  make(map[string]*timestamp),
		aliveLastTS: make(map[string]*timestamp),
		id2Member:   make(map[string]*NetworkMember),
		cachedMembership: &proto.MembershipResponse{
			Alive: make([]*proto.GossipMessage, 0),
			Dead:  make([]*proto.GossipMessage, 0),
		},
		crypt:          crypt,
		bootstrapPeers: bootstrapPeers,
		comm:           comm,
		lock:           &sync.RWMutex{},
		toDieChan:      make(chan struct{}, 1),
		toDieFlag:      int32(0),
		logger:         util.GetLogger(util.LoggingDiscoveryModule, self.Endpoint),
	}

	go d.periodicalSendAlive()
	go d.periodicalCheckAlive()
	go d.handleMessages()
	go d.periodicalReconnectToDead()
	go d.handlePresumedDeadPeers()

	d.connect2BootstrapPeers(bootstrapPeers)

	d.logger.Info("Started", self, "incTime is", d.incTime)

	return d
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
	wg := sync.WaitGroup{}
	req := d.createMembershipRequest()
	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()
			peer := &NetworkMember{
				Endpoint: endpoint,
			}
			d.comm.SendToPeer(peer, req)
		}(endpoint)
	}
	wg.Wait()
}

func (d *gossipDiscoveryImpl) InitiateSync(peerNum int) {
	if d.toDie() {
		return
	}
	var peers2SendTo []*NetworkMember
	memReq := d.createMembershipRequest()

	d.lock.RLock()

	n := len(d.cachedMembership.Alive)
	k := peerNum
	if k > n {
		k = n
	}

	for _, i := range util.GetRandomIndices(k, n-1) {
		pulledPeer := d.cachedMembership.Alive[i].GetAliveMsg().Membership
		netMember := &NetworkMember{
			Endpoint: pulledPeer.Endpoint,
			Metadata: pulledPeer.Metadata,
			PKIid:    pulledPeer.PkiID,
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
				d.logger.Warningf("Alive message isn't authentic, someone spoofed %s's identity", dm.GetAliveMsg().Membership.Endpoint)
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
		Endpoint: member.Endpoint,
		Metadata: member.Metadata,
		PKIid:    member.PkiID,
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

	for _, dm := range d.cachedMembership.Dead {
		isKnown := false
		for _, knownPeer := range known {
			if equalPKIid(knownPeer, dm.GetAliveMsg().Membership.PkiID) {
				isKnown = true
				break
			}
		}
		if !isKnown {
			deadPeers = append(deadPeers, dm)
			break
		}
	}

	return &proto.MembershipResponse{
		Alive: append(d.cachedMembership.Alive, aliveMsg),
		Dead:  deadPeers,
	}
}

func (d *gossipDiscoveryImpl) handleAliveMessage(m *proto.GossipMessage) {
	d.logger.Debug("Entering", m)
	defer d.logger.Debug("Exiting")

	if !d.crypt.ValidateAliveMsg(m) {
		d.logger.Warningf("Alive message isn't authentic, someone must be spoofing %s's identity", m.GetAliveMsg().Membership.Endpoint)
		return
	}

	pkiID := m.GetAliveMsg().Membership.PkiID
	if equalPKIid(pkiID, d.pkiID) {
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
		d.logger.Panicf("Member %s is both alive and dead at the same time", m.GetAliveMsg().Membership.Endpoint)
		return
	}

	if isDead {
		if before(lastDeadTS, ts) {
			// resurrect peer
			d.resurrectMember(m, *ts)
		} else if !same(lastDeadTS, ts) {
			d.logger.Debug(m.GetAliveMsg().Membership.Endpoint, "lastDeadTS:", lastDeadTS, "but got ts:", ts)
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
			d.logger.Debug(m.GetAliveMsg().Membership.Endpoint, "lastAliveTS:", lastAliveTS, "but got ts:", ts)
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
		Endpoint: member.Endpoint,
		Metadata: member.Metadata,
		PKIid:    member.PkiID,
	}
	delete(d.deadLastTS, string(pkiID))

	aliveMsgWithID := &proto.GossipMessage{
		Content: &proto.GossipMessage_AliveMsg{
			AliveMsg: &proto.AliveMessage{
				Membership: &proto.Member{PkiID: pkiID},
			},
		},
	}

	i := util.IndexInSlice(d.cachedMembership.Dead, aliveMsgWithID, samePKIidAliveMessage)
	if i != -1 {
		d.cachedMembership.Dead = append(d.cachedMembership.Dead[:i], d.cachedMembership.Dead[i+1:]...)
	}

	if util.IndexInSlice(d.cachedMembership.Alive, am, samePKIidAliveMessage) == -1 {
		d.cachedMembership.Alive = append(d.cachedMembership.Alive, am)
	}
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

		aliveMsgWithPKIid := &proto.GossipMessage{
			Content: &proto.GossipMessage_AliveMsg{
				AliveMsg: &proto.AliveMessage{
					Membership: &proto.Member{PkiID: pkiID},
				},
			},
		}
		aliveMemberIndex := util.IndexInSlice(d.cachedMembership.Alive, aliveMsgWithPKIid, samePKIidAliveMessage)
		if aliveMemberIndex != -1 {
			// Move the alive member to the dead members
			d.cachedMembership.Dead = append(d.cachedMembership.Dead, d.cachedMembership.Alive[aliveMemberIndex])
			// Delete the alive member from the cached membership
			d.cachedMembership.Alive = append(d.cachedMembership.Alive[:aliveMemberIndex], d.cachedMembership.Alive[aliveMemberIndex+1:]...)
		}
	}

	d.lock.Unlock()

	for _, member2Expire := range deadMembers2Expire {
		d.logger.Warning("Closing connection to", member2Expire.Endpoint)
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

	endpoint := d.endpoint
	meta := d.metadata
	pkiID := d.pkiID

	d.lock.Unlock()

	msg2Gossip := &proto.GossipMessage{
		Tag: proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_AliveMsg{
			AliveMsg: &proto.AliveMessage{
				Membership: &proto.Member{
					Endpoint: endpoint,
					Metadata: meta,
					PkiID:    pkiID,
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

		if _, isKnownAsDead := d.deadLastTS[string(am.Membership.PkiID)]; isKnownAsDead {
			d.logger.Warning(am.Membership.Endpoint, "has already expired")
			continue
		}

		if _, isKnownAsAlive := d.aliveLastTS[string(am.Membership.PkiID)]; !isKnownAsAlive {
			d.logger.Warning(am.Membership.Endpoint, "has already expired")
			continue
		} else {
			d.logger.Debug("Updating aliveness data:", am)
			// update existing aliveness data
			alive := d.aliveLastTS[string(am.Membership.PkiID)]
			alive.incTime = tsToTime(am.Timestamp.IncNumber)
			alive.lastSeen = time.Now()
			alive.seqNum = am.Timestamp.SeqNum

			i := util.IndexInSlice(d.cachedMembership.Alive, m, samePKIidAliveMessage)
			if i == -1 {
				d.logger.Debug("Appended", am, "to d.cachedMembership.Alive")
				d.cachedMembership.Alive = append(d.cachedMembership.Alive, m)
			} else {
				d.logger.Debug("Replaced", am, "in d.cachedMembership.Alive")
				d.cachedMembership.Alive[i] = m
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
		if equalPKIid(am.GetAliveMsg().Membership.PkiID, d.pkiID) {
			continue
		}
		d.aliveLastTS[string(am.GetAliveMsg().Membership.PkiID)] = &timestamp{
			incTime:  tsToTime(am.GetAliveMsg().Timestamp.IncNumber),
			lastSeen: time.Now(),
			seqNum:   am.GetAliveMsg().Timestamp.SeqNum,
		}

		d.cachedMembership.Alive = append(d.cachedMembership.Alive, am)
		d.logger.Infof("Learned about a new alive member: %v", am)
	}

	for _, dm := range deadMembers {
		if equalPKIid(dm.GetAliveMsg().Membership.PkiID, d.pkiID) {
			continue
		}
		d.deadLastTS[string(dm.GetAliveMsg().Membership.PkiID)] = &timestamp{
			incTime:  tsToTime(dm.GetAliveMsg().Timestamp.IncNumber),
			lastSeen: time.Now(),
			seqNum:   dm.GetAliveMsg().Timestamp.SeqNum,
		}

		d.cachedMembership.Dead = append(d.cachedMembership.Dead, dm)
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
				Endpoint: member.Membership.Endpoint,
				Metadata: member.Membership.Metadata,
				PKIid:    member.Membership.PkiID,
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
	for _, m := range d.cachedMembership.Alive {
		member := m.GetAliveMsg()
		response = append(response, NetworkMember{
			PKIid:    member.Membership.PkiID,
			Endpoint: member.Membership.Endpoint,
			Metadata: member.Membership.Metadata,
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
	d.metadata = md
}

func (d *gossipDiscoveryImpl) UpdateEndpoint(endpoint string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.endpoint = endpoint
}

func (d *gossipDiscoveryImpl) Self() NetworkMember {
	return NetworkMember{Endpoint: d.endpoint, Metadata: d.metadata, PKIid: d.pkiID}
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
	return (uint64(a.incTime.UnixNano()) == b.IncNumber && a.seqNum == b.SeqNum)
}

func before(a *timestamp, b *proto.PeerTime) bool {
	return (uint64(a.incTime.UnixNano()) == b.IncNumber && a.seqNum < b.SeqNum) ||
		uint64(a.incTime.UnixNano()) < b.IncNumber
}
