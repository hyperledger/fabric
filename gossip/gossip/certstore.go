/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"bytes"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/pkg/errors"
)

// certStore supports pull dissemination of identity messages
type certStore struct {
	selfIdentity api.PeerIdentityType
	idMapper     identity.Mapper
	pull         pull.Mediator
	logger       util.Logger
	mcs          api.MessageCryptoService
}

func newCertStore(puller pull.Mediator, idMapper identity.Mapper, selfIdentity api.PeerIdentityType, mcs api.MessageCryptoService) *certStore {
	selfPKIID := idMapper.GetPKIidOfCert(selfIdentity)
	logger := util.GetLogger(util.LoggingGossipModule, string(selfPKIID))

	certStore := &certStore{
		mcs:          mcs,
		pull:         puller,
		idMapper:     idMapper,
		selfIdentity: selfIdentity,
		logger:       logger,
	}

	if err := certStore.idMapper.Put(selfPKIID, selfIdentity); err != nil {
		certStore.logger.Panicf("Failed associating self PKIID to cert: %+v", errors.WithStack(err))
	}

	selfIDMsg, err := certStore.createIdentityMessage()
	if err != nil {
		certStore.logger.Panicf("Failed creating self identity message: %+v", errors.WithStack(err))
	}
	puller.Add(selfIDMsg)
	puller.RegisterMsgHook(pull.RequestMsgType, func(_ []string, msgs []*proto.SignedGossipMessage, _ proto.ReceivedMessage) {
		for _, msg := range msgs {
			pkiID := common.PKIidType(msg.GetPeerIdentity().PkiId)
			cert := api.PeerIdentityType(msg.GetPeerIdentity().Cert)
			if err := certStore.idMapper.Put(pkiID, cert); err != nil {
				certStore.logger.Warningf("Failed adding identity %v, reason %+v", cert, errors.WithStack(err))
			}
		}
	})
	return certStore
}

func (cs *certStore) handleMessage(msg proto.ReceivedMessage) {
	if update := msg.GetGossipMessage().GetDataUpdate(); update != nil {
		for _, env := range update.Data {
			m, err := env.ToGossipMessage()
			if err != nil {
				cs.logger.Warningf("Data update contains an invalid message: %+v", errors.WithStack(err))
				return
			}
			if !m.IsIdentityMsg() {
				cs.logger.Warning("Got a non-identity message:", m, "aborting")
				return
			}
			if err := cs.validateIdentityMsg(m); err != nil {
				cs.logger.Warningf("Failed validating identity message: %+v", errors.WithStack(err))
				return
			}
		}
	}
	cs.pull.HandleMessage(msg)
}

func (cs *certStore) validateIdentityMsg(msg *proto.SignedGossipMessage) error {
	idMsg := msg.GetPeerIdentity()
	if idMsg == nil {
		return errors.Errorf("Identity empty: %+v", msg)
	}
	pkiID := idMsg.PkiId
	cert := idMsg.Cert
	calculatedPKIID := cs.mcs.GetPKIidOfCert(api.PeerIdentityType(cert))
	claimedPKIID := common.PKIidType(pkiID)
	if !bytes.Equal(calculatedPKIID, claimedPKIID) {
		return errors.Errorf("Calculated pkiID doesn't match identity: calculated: %v, claimedPKI-ID: %v", calculatedPKIID, claimedPKIID)
	}

	verifier := func(peerIdentity []byte, signature, message []byte) error {
		return cs.mcs.Verify(api.PeerIdentityType(peerIdentity), signature, message)
	}

	err := msg.Verify(cert, verifier)
	if err != nil {
		return errors.Wrap(err, "Failed verifying message")
	}

	return cs.mcs.ValidateIdentity(api.PeerIdentityType(idMsg.Cert))
}

func (cs *certStore) createIdentityMessage() (*proto.SignedGossipMessage, error) {
	pi := &proto.PeerIdentity{
		Cert:     cs.selfIdentity,
		Metadata: nil,
		PkiId:    cs.idMapper.GetPKIidOfCert(cs.selfIdentity),
	}
	m := &proto.GossipMessage{
		Channel: nil,
		Nonce:   0,
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_PeerIdentity{
			PeerIdentity: pi,
		},
	}
	signer := func(msg []byte) ([]byte, error) {
		return cs.idMapper.Sign(msg)
	}
	sMsg := &proto.SignedGossipMessage{
		GossipMessage: m,
	}
	_, err := sMsg.Sign(signer)
	return sMsg, errors.WithStack(err)
}

func (cs *certStore) suspectPeers(isSuspected api.PeerSuspector) {
	cs.idMapper.SuspectPeers(isSuspected)
}

func (cs *certStore) stop() {
	cs.pull.Stop()
	cs.idMapper.Stop()
}
