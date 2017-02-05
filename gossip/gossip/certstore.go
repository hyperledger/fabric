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

package gossip

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
)

// certStore supports pull dissemination of identity messages
type certStore struct {
	sync.RWMutex
	selfIdentity api.PeerIdentityType
	idMapper     identity.Mapper
	pull         pull.Mediator
	logger       *logging.Logger
	mcs          api.MessageCryptoService
}

func newCertStore(puller pull.Mediator, idMapper identity.Mapper, selfIdentity api.PeerIdentityType, mcs api.MessageCryptoService) *certStore {
	selfPKIID := idMapper.GetPKIidOfCert(selfIdentity)
	logger := util.GetLogger(util.LoggingGossipModule, string(selfPKIID))
	if err := idMapper.Put(selfPKIID, selfIdentity); err != nil {
		logger.Error("Failed associating self PKIID to cert:", err)
		panic(fmt.Errorf("Failed associating self PKIID to cert: %v", err))
	}

	certStore := &certStore{
		mcs:          mcs,
		pull:         puller,
		idMapper:     idMapper,
		selfIdentity: selfIdentity,
		logger:       logger,
	}

	if err := certStore.idMapper.Put(selfPKIID, selfIdentity); err != nil {
		certStore.logger.Panic("Failed associating self PKIID to cert:", err)
	}

	puller.Add(certStore.createIdentityMessage())

	puller.RegisterMsgHook(pull.ResponseMsgType, func(_ []string, msgs []*proto.GossipMessage, _ comm.ReceivedMessage) {
		for _, msg := range msgs {
			pkiID := common.PKIidType(msg.GetPeerIdentity().PkiID)
			cert := api.PeerIdentityType(msg.GetPeerIdentity().Cert)
			if err := certStore.idMapper.Put(pkiID, cert); err != nil {
				certStore.logger.Warning("Failed adding identity", cert, ", reason:", err)
			}
		}
	})

	puller.Add(certStore.createIdentityMessage())

	return certStore
}

func (cs *certStore) handleMessage(msg comm.ReceivedMessage) {
	if update := msg.GetGossipMessage().GetDataUpdate(); update != nil {
		for _, m := range update.Data {
			if !m.IsIdentityMsg() {
				cs.logger.Warning("Got a non-identity message:", m, "aborting")
				return
			}
			if err := cs.validateIdentityMsg(m); err != nil {
				cs.logger.Warning("Failed validating identity message:", err)
				return
			}
		}
	}
	cs.pull.HandleMessage(msg)
}

func (cs *certStore) validateIdentityMsg(msg *proto.GossipMessage) error {
	idMsg := msg.GetPeerIdentity()
	if idMsg == nil {
		return fmt.Errorf("Identity empty: %+v", msg)
	}
	pkiID := idMsg.PkiID
	cert := idMsg.Cert
	calculatedPKIID := cs.mcs.GetPKIidOfCert(api.PeerIdentityType(cert))
	claimedPKIID := common.PKIidType(pkiID)
	if !bytes.Equal(calculatedPKIID, claimedPKIID) {
		return fmt.Errorf("Calculated pkiID doesn't match identity: calculated: %v, claimedPKI-ID: %v", calculatedPKIID, claimedPKIID)
	}

	verifier := func(peerIdentity []byte, signature, message []byte) error {
		return cs.mcs.Verify(api.PeerIdentityType(peerIdentity), signature, message)
	}

	err := msg.Verify(cert, verifier)
	if err != nil {
		return fmt.Errorf("Failed verifying message: %v", err)
	}

	return cs.mcs.ValidateIdentity(api.PeerIdentityType(idMsg.Cert))
}

func (cs *certStore) createIdentityMessage() *proto.GossipMessage {
	identity := &proto.PeerIdentity{
		Cert:     cs.selfIdentity,
		Metadata: nil,
		PkiID:    cs.idMapper.GetPKIidOfCert(cs.selfIdentity),
	}

	m := &proto.GossipMessage{
		Channel: nil,
		Nonce:   0,
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_PeerIdentity{
			PeerIdentity: identity,
		},
	}

	signer := func(msg []byte) ([]byte, error) {
		return cs.idMapper.Sign(msg)
	}
	if err := m.Sign(signer); err != nil {
		cs.logger.Warning("Failed signing identity message:", err)
		return nil
	}

	return m
}

func (cs *certStore) stop() {
	cs.pull.Stop()
}
