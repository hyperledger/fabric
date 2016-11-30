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
	"sync"

	prot "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/util"
)

// certStore supports pull dissemination of identity messages
type certStore struct {
	sync.RWMutex
	selfIdentity api.PeerIdentityType
	idMapper     identity.Mapper
	pull         pull.Mediator
	logger       *util.Logger
}

func newCertStore(mcs api.MessageCryptoService, selfIdentity api.PeerIdentityType, pullMed pull.Mediator) *certStore {
	certStore := &certStore{
		idMapper:     identity.NewIdentityMapper(mcs),
		selfIdentity: selfIdentity,
		pull:         pullMed,
	}

	selfPKIID := certStore.idMapper.GetPKIidOfCert(selfIdentity)

	certStore.logger = util.GetLogger("certStore", string(selfPKIID))

	if err := certStore.idMapper.Put(selfPKIID, selfIdentity); err != nil {
		certStore.logger.Panic("Failed associating self PKIID to cert:", err)
	}

	pullMed.Add(certStore.createIdentityMessage())

	pullMed.RegisterMsgHook(pull.ResponseMsgType, func(_ []string, msgs []*proto.GossipMessage, _ comm.ReceivedMessage) {
		for _, msg := range msgs {
			pkiID := common.PKIidType(msg.GetPeerIdentity().PkiID)
			cert := api.PeerIdentityType(msg.GetPeerIdentity().Cert)
			if err := certStore.idMapper.Put(pkiID, cert); err != nil {
				certStore.logger.Warning("Failed adding identity", cert, ", reason:", err)
			}
		}
	})

	return certStore
}

func (cs *certStore) createIdentityMessage() *proto.GossipMessage {
	identity := &proto.PeerIdentity{
		Cert:     cs.selfIdentity,
		Metadata: nil,
		PkiID:    cs.idMapper.GetPKIidOfCert(cs.selfIdentity),
		Sig:      nil,
	}

	b, err := prot.Marshal(identity)
	if err != nil {
		cs.logger.Warning("Failed marshalling identity message:", err)
		return nil
	}

	sig, err := cs.idMapper.Sign(b)
	if err != nil {
		cs.logger.Warning("Failed signing identity message:", err)
		return nil
	}
	identity.Sig = sig

	return &proto.GossipMessage{
		Channel: nil,
		Nonce:   0,
		Tag:     proto.GossipMessage_EMPTY,
		Content: &proto.GossipMessage_PeerIdentity{
			PeerIdentity: identity,
		},
	}
}

func (cs *certStore) stop() {
	cs.pull.Stop()
}
