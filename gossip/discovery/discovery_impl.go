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
	"sync"
	"time"

	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/util"
	"sync/atomic"
)

const DEFAULT_HELLO_INTERVAL = time.Duration(5) * time.Second

var aliveTimeInterval = DEFAULT_HELLO_INTERVAL
var aliveExpirationTimeout = 5 * aliveTimeInterval
var aliveExpirationCheckInterval = time.Duration(aliveExpirationTimeout / 10)
var reconnectInterval = aliveExpirationTimeout

func sameIdAliveMessages(a interface{}, b interface{}) bool {
	return a.(*proto.AliveMessage).Membership.Id == b.(*proto.AliveMessage).Membership.Id
}

type timestamp struct {
	incTime  time.Time
	seqNum   uint64
	lastSeen time.Time
}

type gossipDiscoveryImpl struct {
	id       string
	endpoint string
	incTime  time.Time
	metadata []byte

	seqNum uint64

	deadLastTS       map[string]*timestamp     // H
	aliveLastTS      map[string]*timestamp     // V
	id2Member        map[string]*NetworkMember // all known members
	cachedMembership *proto.MembershipResponse

	bootstrapPeers []*NetworkMember

	comm   CommService
	crpypt CryptoService
	lock   *sync.RWMutex

	toDieChan  chan struct{}
	toDieFlag  int32
	stopSignal *sync.WaitGroup
	logger     *util.Logger
}

func NewDiscoveryService(bootstrapPeers []*NetworkMember, self NetworkMember, comm CommService, crypt CryptoService) DiscoveryService {
	d := &gossipDiscoveryImpl{
		id:       self.Id,
		endpoint: self.Endpoint,
		incTime:  time.Now(),
		metadata: self.Metadata,

		seqNum: uint64(0),

		deadLastTS:  make(map[string]*timestamp),
		aliveLastTS: make(map[string]*timestamp),
		id2Member:   make(map[string]*NetworkMember),
		cachedMembership: &proto.MembershipResponse{
			Alive: make([]*proto.AliveMessage, 0),
			Dead:  make([]*proto.AliveMessage, 0),
		},

		crpypt: crypt,

		bootstrapPeers: bootstrapPeers,

		comm: comm,
		lock: &sync.RWMutex{},

		toDieChan:  make(chan struct{}, 2),
		toDieFlag:  int32(0),
		stopSignal: &sync.WaitGroup{},
		logger:     util.GetLogger(util.LOGGING_DISCOVERY_MODULE, self.Id),
	}

	// Add bootstrap peers to the dead member set
	// The periodicalReconnectToDead() method immediately connects to all of them
	for _, bootPeer := range bootstrapPeers {
		if bootPeer.Id == d.id {
			continue
		}
		d.deadLastTS[bootPeer.Id] = getZeroTS()
		d.id2Member[bootPeer.Id] = bootPeer
	}

	go d.periodicalSendAlive()
	go d.periodicalCheckAlive()
	go d.handleMessages()
	go d.periodicalReconnectToDead()
	go d.handlePresumedDeadPeers()

	return d
}

func getZeroTS() *timestamp {
	return &timestamp{incTime: time.Unix(0, 0), lastSeen: time.Unix(0, 0), seqNum: 0}
}

func (d *gossipDiscoveryImpl) InitiateSync(peerNum int) {
	memReq := d.createMembershipRequest()
	d.lock.RLock()
	defer d.lock.RUnlock()

	n := len(d.cachedMembership.Alive)
	k := peerNum
	if k > n {
		k = n
	}

	for _, i := range util.GetRandomIndices(k, n-1) {
		pulledPeer := d.cachedMembership.Alive[i].Membership
		netMember := &NetworkMember{
			Id:       pulledPeer.Id,
			Endpoint: pulledPeer.Endpoint,
			Metadata: pulledPeer.Metadata,
		}
		d.comm.SendToPeer(netMember, memReq)
	}
}

func (d *gossipDiscoveryImpl) handlePresumedDeadPeers() {
	d.stopSignal.Add(1)
	defer d.stopSignal.Done()

	for !d.toDie() {
		select {
		case deadPeer := <-d.comm.PresumedDead():
			d.expireDeadMembers([]string{deadPeer})
		case <-d.toDieChan:
			return
		}
	}
}

func (d *gossipDiscoveryImpl) handleMessages() {
	d.stopSignal.Add(1)
	defer d.stopSignal.Done()
	for !d.toDie() {
		select {
		case <-d.toDieChan:
			return
		case m := <-d.comm.Accept():
			d.handleMsgFromComm(m.GetGossipMessage())
		}
	}
}

func (d *gossipDiscoveryImpl) handleMsgFromComm(m *proto.GossipMessage) {
	if m.GetAliveMsg() == nil && m.GetMemRes() == nil && m.GetMemReq() == nil {
		d.logger.Warning("Got message with wrong type (expected Alive or MembershipResponse or MembershipRequest message):", m.Content) // TODO: write only message type
		d.logger.Warning(m)
	}

	d.logger.Info("Got message:", m)
	defer d.logger.Info("Exiting")

	// TODO: make sure somehow that the membership request is "fresh"
	if memReq := m.GetMemReq(); memReq != nil {
		d.sendMemResponse(memReq.SelfInformation.Membership, memReq.Known)
		return
	}

	alive := m.GetAliveMsg()
	if alive != nil {
		d.handleAliveMessage(alive)
	}

	memResp := m.GetMemRes()
	if memResp != nil {
		for _, am := range memResp.Alive {
			d.handleAliveMessage(am)
		}

		for _, dm := range memResp.Dead {
			if !d.crpypt.ValidateAliveMsg(dm) {
				d.logger.Warningf("Alive message isn't authentic, someone spoofed %s's identity", dm.Membership.Id)
				continue
			}
			d.lock.RLock()
			if _, known := d.id2Member[dm.Membership.Id]; !known {
				d.learnNewMembers([]*proto.AliveMessage{}, []*proto.AliveMessage{dm})
			}
			d.lock.RUnlock()
		}
	}
}

func (d *gossipDiscoveryImpl) sendMemResponse(member *proto.Member, known []string) {
	d.logger.Info("Entering, id:", member.Id)

	defer d.logger.Info("Exiting")

	d.comm.SendToPeer(&NetworkMember{Endpoint: member.Endpoint, Id: member.Id, Metadata: member.Metadata}, &proto.GossipMessage{
		Nonce: uint64(0),
		Content: &proto.GossipMessage_MemRes{
			MemRes: d.createMembershipResponse(known),
		},
	})
}

func (d *gossipDiscoveryImpl) createMembershipResponse(known []string) *proto.MembershipResponse {
	aliveMsg := d.createAliveMessage()

	d.lock.RLock()
	defer d.lock.RUnlock()

	alivePeers := make([]*proto.AliveMessage, 0)
	deadPeers := make([]*proto.AliveMessage, 0)

	for _, am := range d.cachedMembership.Alive {
		isKnown := false
		for _, knownPeer := range known {
			if knownPeer == am.Membership.Id {
				isKnown = true
				break
			}
		}
		if isKnown {
			alivePeers = append(alivePeers, am)
			break
		}
	}

	for _, dm := range d.cachedMembership.Dead {
		isKnown := false
		for _, knownPeer := range known {
			if knownPeer == dm.Membership.Id {
				isKnown = true
				break
			}
		}
		if isKnown {
			deadPeers = append(deadPeers, dm)
			break
		}
	}

	return &proto.MembershipResponse{
		Alive: append(alivePeers, aliveMsg),
		Dead:  deadPeers,
	}
}

func (d *gossipDiscoveryImpl) handleAliveMessage(m *proto.AliveMessage) {
	d.logger.Info("Entering", m)
	defer d.logger.Info("Exiting")
	if !d.crpypt.ValidateAliveMsg(m) {
		d.logger.Warningf("Alive message isn't authentic, someone must be spoofing %s's identity", m.Membership.Id)
		return
	}

	id := m.Membership.Id
	if id == d.id {
		d.logger.Debug("Got alive message about ourselves,", m)
		return
	}
	ts := m.Timestamp

	d.lock.RLock()
	_, known := d.id2Member[id]
	d.lock.RUnlock()


	if !known {
		d.learnNewMembers([]*proto.AliveMessage{m}, []*proto.AliveMessage{})
		return
	}

	d.lock.RLock()
	lastAliveTS, isAlive := d.aliveLastTS[id]
	lastDeadTS, isDead := d.deadLastTS[id]
	d.lock.RUnlock()

	if !isAlive && !isDead {
		d.logger.Panicf("Member %s is known but not found neither in alive nor in dead lastTS maps, isAlive=%v, isDead=%v", id, isAlive, isDead)
		return
	}

	if isAlive && isDead {
		d.logger.Panicf("Member %s is both alive and dead at the same time", id)
		return
	}

	if !isAlive && uint64(lastDeadTS.incTime.Nanosecond()) <= ts.IncNumber && lastDeadTS.seqNum < ts.SeqNum {
		// resurrect peer
		d.resurrectMember(m, *ts)
		return
	}

	d.lock.RLock()
	lastAliveTS, isAlive = d.aliveLastTS[id]
	d.lock.RUnlock()

	if isAlive {
		if uint64(lastAliveTS.incTime.Nanosecond()) <= ts.IncNumber && lastAliveTS.seqNum < ts.SeqNum {
			d.learnExistingMembers([]*proto.AliveMessage{m})
		}
	}
	// else, ignore the message because it is too old
}

func (d *gossipDiscoveryImpl) resurrectMember(m *proto.AliveMessage, t proto.PeerTime) {
	d.logger.Info("Entering,", m, t)
	defer d.logger.Info("Exiting")
	d.lock.Lock()
	defer d.lock.Unlock()

	id := m.Membership.Id

	d.aliveLastTS[id] = &timestamp{
		lastSeen: time.Now(),
		seqNum:   t.SeqNum,
		incTime:  tsToTime(t.IncNumber),
	}

	d.id2Member[id] = &NetworkMember{
		Id: id,
		Endpoint: m.Membership.Endpoint,
		Metadata: m.Membership.Metadata,
	}
	delete(d.deadLastTS, id)
	aliveMsgWithId := &proto.AliveMessage{
		Membership: &proto.Member{Id: id},
	}

	// If the member is in the dead list, delete it from there
	i := util.IndexInSlice(d.cachedMembership.Dead, aliveMsgWithId, sameIdAliveMessages)
	if i != -1 {
		d.cachedMembership.Dead = append(d.cachedMembership.Dead[:i], d.cachedMembership.Dead[i+1:]...)
	}
	// add the member to the alive list
	d.cachedMembership.Alive = append(d.cachedMembership.Alive, m)
}

func (d *gossipDiscoveryImpl) periodicalReconnectToDead() {
	d.stopSignal.Add(1)
	defer d.stopSignal.Done()

	for !d.toDie() {
		wg := &sync.WaitGroup{}

		for _, member := range d.copyLastSeen(d.deadLastTS) {
			wg.Add(1)
			go func(member NetworkMember) {
				defer wg.Done()
				if d.comm.Ping(&member) {
					d.sendMembershipRequest(&member)
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
		Nonce: uint64(0),
		Content: &proto.GossipMessage_MemReq{
			MemReq: req,
		},
	}
}

func (d *gossipDiscoveryImpl) getKnownPeers() []string {
	d.lock.RLock()
	defer d.lock.RUnlock()

	peers := make([]string, 0)
	for id := range d.id2Member {
		peers = append(peers, id)
	}
	return peers
}

func (d *gossipDiscoveryImpl) copyLastSeen(lastSeenMap map[string]*timestamp) []NetworkMember {
	d.lock.RLock()
	defer d.lock.RUnlock()

	d.logger.Debug("Entering,", lastSeenMap)
	defer d.logger.Debug("Exiting,")

	res := make([]NetworkMember, 0)
	for id, last := range lastSeenMap {
		if _, exists := d.id2Member[id]; !exists {
			// Maybe it's a bootstrap peer?
			foundInBootPeers := false
			for _, bootPeer := range d.bootstrapPeers {
				if bootPeer.Id == id {
					res = append(res, NetworkMember{
						Id:       bootPeer.Id,
						Metadata: bootPeer.Metadata,
						Endpoint: bootPeer.Endpoint,
					})
					foundInBootPeers = true
					break
				}
			}
			if !foundInBootPeers {
				d.logger.Warning(id, " was last seen on", last, "but wasn't found in id2Member or in bootPeers")
			}
		} else {
			res = append(res, *(d.id2Member[id]))
		}
	}
	return res
}

func (d *gossipDiscoveryImpl) periodicalCheckAlive() {
	d.stopSignal.Add(1)
	defer d.stopSignal.Done()

	for !d.toDie() {
		d.logger.Debug("Sleeping", aliveExpirationCheckInterval)
		time.Sleep(aliveExpirationCheckInterval)
		dead := d.getDeadMembers()
		if len(dead) > 0 {
			d.logger.Debugf("Got %d dead members: %v", len(dead), dead)
			d.expireDeadMembers(dead)
		}
	}
}

func (d *gossipDiscoveryImpl) expireDeadMembers(dead []string) {
	d.logger.Infof("Entering", dead)
	defer d.logger.Infof("Exiting")

	d.lock.Lock()
	defer d.lock.Unlock()

	for _, id := range dead {
		d.comm.CloseConn(id)
		// move lastTS from alive to dead
		lastTS, hasLastTS := d.aliveLastTS[id]
		if hasLastTS {
			d.deadLastTS[id] = lastTS
			delete(d.aliveLastTS, id)
		}

		aliveMsgWithId := &proto.AliveMessage{
			Membership: &proto.Member{Id: id},
		}
		aliveMemberIndex := util.IndexInSlice(d.cachedMembership.Alive, aliveMsgWithId, sameIdAliveMessages)
		if aliveMemberIndex != -1 {
			// Move the alive member to the dead members
			d.cachedMembership.Dead = append(d.cachedMembership.Dead, d.cachedMembership.Alive[aliveMemberIndex])
			// Delete the alive member from the cached membership
			d.cachedMembership.Alive = append(d.cachedMembership.Alive[:aliveMemberIndex], d.cachedMembership.Alive[aliveMemberIndex+1:]...)
		}
	}
}

func (d *gossipDiscoveryImpl) getDeadMembers() []string {
	d.lock.RLock()
	defer d.lock.RUnlock()

	dead := make([]string, 0)
	for id, last := range d.aliveLastTS {
		elapsedNonAliveTime := time.Since(last.lastSeen)
		if elapsedNonAliveTime.Nanoseconds() > aliveExpirationTimeout.Nanoseconds() {
			d.logger.Warning("Haven't heard from", id, "for", elapsedNonAliveTime)
			dead = append(dead, id)
		}
	}
	return dead
}

func (d *gossipDiscoveryImpl) periodicalSendAlive() {
	d.stopSignal.Add(1)
	defer d.stopSignal.Done()
	for !d.toDie() {
		d.logger.Debug("Sleeping", aliveTimeInterval)
		time.Sleep(aliveTimeInterval)
		msg2Gossip := &proto.GossipMessage{
			Content: &proto.GossipMessage_AliveMsg{d.createAliveMessage()},
		}
		d.comm.Gossip(msg2Gossip)
	}
}

func (d *gossipDiscoveryImpl) createAliveMessage() *proto.AliveMessage {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.seqNum++
	return d.crpypt.SignMessage(&proto.AliveMessage{
		Membership: &proto.Member{
			Id:       d.id,
			Endpoint: d.endpoint,
			Metadata: d.metadata,
		},
		Signature: []byte{},
		Timestamp: &proto.PeerTime{
			IncNumber: uint64(d.incTime.Nanosecond()),
			SeqNum:    d.seqNum,
		},
	})
}

func (d *gossipDiscoveryImpl) learnExistingMembers(aliveArr []*proto.AliveMessage) {
	d.logger.Infof("Entering: learnedMembers={%v}", aliveArr)
	defer d.logger.Info("Exiting")

	d.lock.Lock()
	defer d.lock.Unlock()

	for _, am := range aliveArr {
		d.logger.Info("updating", am)
		// update member's data
		member := d.id2Member[am.Membership.Id]
		member.Endpoint = am.Membership.Endpoint
		member.Metadata = am.Membership.Metadata

		if _, isKnownAsDead := d.deadLastTS[am.Membership.Id]; isKnownAsDead {
			d.logger.Warning(am.Membership.Id, "has already expired")
			continue
		}

		if _, isKnownAsAlive := d.aliveLastTS[am.Membership.Id]; !isKnownAsAlive {
			d.logger.Warning(am.Membership.Id, "has already expired")
			continue
		} else {
			d.logger.Info("Updating aliveness data:", am)
			// update existing aliveness data
			alive := d.aliveLastTS[am.Membership.Id]
			alive.incTime = tsToTime(am.Timestamp.IncNumber)
			alive.lastSeen = time.Now()
			alive.seqNum = am.Timestamp.SeqNum

			i := util.IndexInSlice(d.cachedMembership.Alive, am, sameIdAliveMessages)
			if i == -1 {
				d.cachedMembership.Alive = append(d.cachedMembership.Alive, am)
			} else {
				d.cachedMembership.Alive[i] = am
			}
		}
	}
}

func (d *gossipDiscoveryImpl) learnNewMembers(aliveMembers []*proto.AliveMessage, deadMembers []*proto.AliveMessage) {
	d.logger.Debugf("Entering: learnedMembers={%v}, deadMembers={%v}", aliveMembers, deadMembers)
	defer d.logger.Debugf("Exiting")

	d.lock.Lock()
	defer d.lock.Unlock()

	for _, am := range aliveMembers {
		d.aliveLastTS[am.Membership.Id] = &timestamp{
			incTime:  tsToTime(am.Timestamp.IncNumber),
			lastSeen: time.Now(),
			seqNum:   am.Timestamp.SeqNum,
		}

		d.cachedMembership.Alive = append(d.cachedMembership.Alive, am)
		d.logger.Infof("Learned about a new alive member: %v", am)
	}

	for _, dm := range deadMembers {
		d.deadLastTS[dm.Membership.Id] = &timestamp{
			incTime:  tsToTime(dm.Timestamp.IncNumber),
			lastSeen: time.Now(),
			seqNum:   dm.Timestamp.SeqNum,
		}

		d.cachedMembership.Dead = append(d.cachedMembership.Dead, dm)
		d.logger.Infof("Learned about a new dead member: %v", dm)
	}

	// update the member in any case
	for _, a := range [][]*proto.AliveMessage{aliveMembers, deadMembers} {
		for _, member := range a {
			d.id2Member[member.Membership.Id] = &NetworkMember{
				Id:       member.Membership.Id,
				Endpoint: member.Membership.Endpoint,
				Metadata: member.Membership.Metadata,
			}
		}
	}
}

func (d *gossipDiscoveryImpl) GetMembership() []NetworkMember {
	d.lock.RLock()
	defer d.lock.RUnlock()

	response := make([]NetworkMember, 0)
	for _, member := range d.cachedMembership.Alive {
		response = append(response, NetworkMember{
			Id:       member.Membership.Id,
			Endpoint: member.Membership.Endpoint,
			Metadata: member.Membership.Metadata,
		})
	}
	d.logger.Debugf("Returning %d members", len(response))
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
	return NetworkMember{Id: d.id, Endpoint: d.endpoint, Metadata: d.metadata}
}

func (d *gossipDiscoveryImpl) toDie() bool {
	return atomic.LoadInt32(&d.toDieFlag) == 1
}

func (d *gossipDiscoveryImpl) Stop() {
	defer d.logger.Infof("Stopped")
	d.logger.Info("Stopping")
	atomic.StoreInt32(&d.toDieFlag, int32(1))
	d.toDieChan <- struct{}{}
	d.toDieChan <- struct{}{}
	d.stopSignal.Wait()
}
