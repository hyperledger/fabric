/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
)

// IsAliveMsg returns whether this GossipMessage is an AliveMessage
func (m *GossipMessage) IsAliveMsg() bool {
	return m.GetAliveMsg() != nil
}

// IsDataMsg returns whether this GossipMessage is a data message
func (m *GossipMessage) IsDataMsg() bool {
	return m.GetDataMsg() != nil
}

// IsStateInfoPullRequestMsg returns whether this GossipMessage is a stateInfoPullRequest
func (m *GossipMessage) IsStateInfoPullRequestMsg() bool {
	return m.GetStateInfoPullReq() != nil
}

// IsStateInfoSnapshot returns whether this GossipMessage is a stateInfo snapshot
func (m *GossipMessage) IsStateInfoSnapshot() bool {
	return m.GetStateSnapshot() != nil
}

// IsStateInfoMsg returns whether this GossipMessage is a stateInfo message
func (m *GossipMessage) IsStateInfoMsg() bool {
	return m.GetStateInfo() != nil
}

// IsPullMsg returns whether this GossipMessage is a message that belongs
// to the pull mechanism
func (m *GossipMessage) IsPullMsg() bool {
	return m.GetDataReq() != nil || m.GetDataUpdate() != nil ||
		m.GetHello() != nil || m.GetDataDig() != nil
}

// IsRemoteStateMessage returns whether this GossipMessage is related to state synchronization
func (m *GossipMessage) IsRemoteStateMessage() bool {
	return m.GetStateRequest() != nil || m.GetStateResponse() != nil
}

// GetPullMsgType returns the phase of the pull mechanism this GossipMessage belongs to
// for example: Hello, Digest, etc.
// If this isn't a pull message, PullMsgType_UNDEFINED is returned.
func (m *GossipMessage) GetPullMsgType() PullMsgType {
	if helloMsg := m.GetHello(); helloMsg != nil {
		return helloMsg.MsgType
	}

	if digMsg := m.GetDataDig(); digMsg != nil {
		return digMsg.MsgType
	}

	if reqMsg := m.GetDataReq(); reqMsg != nil {
		return reqMsg.MsgType
	}

	if resMsg := m.GetDataUpdate(); resMsg != nil {
		return resMsg.MsgType
	}

	return PullMsgType_UNDEFINED
}

// IsChannelRestricted returns whether this GossipMessage should be routed
// only in its channel
func (m *GossipMessage) IsChannelRestricted() bool {
	return m.Tag == GossipMessage_CHAN_AND_ORG || m.Tag == GossipMessage_CHAN_ONLY || m.Tag == GossipMessage_CHAN_OR_ORG
}

// IsOrgRestricted returns whether this GossipMessage should be routed only
// inside the organization
func (m *GossipMessage) IsOrgRestricted() bool {
	return m.Tag == GossipMessage_CHAN_AND_ORG || m.Tag == GossipMessage_ORG_ONLY
}

// IsIdentityMsg returns whether this GossipMessage is an identity message
func (m *GossipMessage) IsIdentityMsg() bool {
	return m.GetPeerIdentity() != nil
}

// IsDataReq returns whether this GossipMessage is a data request message
func (m *GossipMessage) IsDataReq() bool {
	return m.GetDataReq() != nil
}

// IsPrivateDataMsg returns whether this message is related to private data
func (m *GossipMessage) IsPrivateDataMsg() bool {
	return m.GetPrivateReq() != nil || m.GetPrivateRes() != nil || m.GetPrivateData() != nil
}

// IsAck returns whether this GossipMessage is an acknowledgement
func (m *GossipMessage) IsAck() bool {
	return m.GetAck() != nil
}

// IsDataUpdate returns whether this GossipMessage is a data update message
func (m *GossipMessage) IsDataUpdate() bool {
	return m.GetDataUpdate() != nil
}

// IsHelloMsg returns whether this GossipMessage is a hello message
func (m *GossipMessage) IsHelloMsg() bool {
	return m.GetHello() != nil
}

// IsDigestMsg returns whether this GossipMessage is a digest message
func (m *GossipMessage) IsDigestMsg() bool {
	return m.GetDataDig() != nil
}

// IsLeadershipMsg returns whether this GossipMessage is a leadership (leader election) message
func (m *GossipMessage) IsLeadershipMsg() bool {
	return m.GetLeadershipMsg() != nil
}

// IsTagLegal checks the GossipMessage tags and inner type
// and returns an error if the tag doesn't match the type.
func (m *GossipMessage) IsTagLegal() error {
	if m.Tag == GossipMessage_UNDEFINED {
		return fmt.Errorf("Undefined tag")
	}
	if m.IsDataMsg() {
		if m.Tag != GossipMessage_CHAN_AND_ORG {
			return fmt.Errorf("Tag should be %s", GossipMessage_Tag_name[int32(GossipMessage_CHAN_AND_ORG)])
		}
		return nil
	}

	if m.IsAliveMsg() || m.GetMemReq() != nil || m.GetMemRes() != nil {
		if m.Tag != GossipMessage_EMPTY {
			return fmt.Errorf("Tag should be %s", GossipMessage_Tag_name[int32(GossipMessage_EMPTY)])
		}
		return nil
	}

	if m.IsIdentityMsg() {
		if m.Tag != GossipMessage_ORG_ONLY {
			return fmt.Errorf("Tag should be %s", GossipMessage_Tag_name[int32(GossipMessage_ORG_ONLY)])
		}
		return nil
	}

	if m.IsPullMsg() {
		switch m.GetPullMsgType() {
		case PullMsgType_BLOCK_MSG:
			if m.Tag != GossipMessage_CHAN_AND_ORG {
				return fmt.Errorf("Tag should be %s", GossipMessage_Tag_name[int32(GossipMessage_CHAN_AND_ORG)])
			}
			return nil
		case PullMsgType_IDENTITY_MSG:
			if m.Tag != GossipMessage_EMPTY {
				return fmt.Errorf("Tag should be %s", GossipMessage_Tag_name[int32(GossipMessage_EMPTY)])
			}
			return nil
		default:
			return fmt.Errorf("Invalid PullMsgType: %s", PullMsgType_name[int32(m.GetPullMsgType())])
		}
	}

	if m.IsStateInfoMsg() || m.IsStateInfoPullRequestMsg() || m.IsStateInfoSnapshot() || m.IsRemoteStateMessage() {
		if m.Tag != GossipMessage_CHAN_OR_ORG {
			return fmt.Errorf("Tag should be %s", GossipMessage_Tag_name[int32(GossipMessage_CHAN_OR_ORG)])
		}
		return nil
	}

	if m.IsLeadershipMsg() {
		if m.Tag != GossipMessage_CHAN_AND_ORG {
			return fmt.Errorf("Tag should be %s", GossipMessage_Tag_name[int32(GossipMessage_CHAN_AND_ORG)])
		}
		return nil
	}

	return fmt.Errorf("Unknown message type: %v", m)
}

// Verifier receives a peer identity, a signature and a message
// and returns nil if the signature on the message could be verified
// using the given identity.
type Verifier func(peerIdentity []byte, signature, message []byte) error

// Signer signs a message, and returns (signature, nil)
// on success, and nil and an error on failure.
type Signer func(msg []byte) ([]byte, error)

// Sign signs a GossipMessage with given Signer.
// Returns an Envelope on success,
// panics on failure.
func (m *SignedGossipMessage) Sign(signer Signer) (*Envelope, error) {
	// If we have a secretEnvelope, don't override it.
	// Back it up, and restore it later
	var secretEnvelope *SecretEnvelope
	if m.Envelope != nil {
		secretEnvelope = m.Envelope.SecretEnvelope
	}
	m.Envelope = nil
	payload, err := proto.Marshal(m.GossipMessage)
	if err != nil {
		return nil, err
	}
	sig, err := signer(payload)
	if err != nil {
		return nil, err
	}

	e := &Envelope{
		Payload:        payload,
		Signature:      sig,
		SecretEnvelope: secretEnvelope,
	}
	m.Envelope = e
	return e, nil
}

// NoopSign creates a SignedGossipMessage with a nil signature
func (m *GossipMessage) NoopSign() (*SignedGossipMessage, error) {
	signer := func(msg []byte) ([]byte, error) {
		return nil, nil
	}
	sMsg := &SignedGossipMessage{
		GossipMessage: m,
	}
	_, err := sMsg.Sign(signer)
	return sMsg, err
}

// Verify verifies a signed GossipMessage with a given Verifier.
// Returns nil on success, error on failure.
func (m *SignedGossipMessage) Verify(peerIdentity []byte, verify Verifier) error {
	if m.Envelope == nil {
		return errors.New("Missing envelope")
	}
	if len(m.Envelope.Payload) == 0 {
		return errors.New("Empty payload")
	}
	if len(m.Envelope.Signature) == 0 {
		return errors.New("Empty signature")
	}
	payloadSigVerificationErr := verify(peerIdentity, m.Envelope.Signature, m.Envelope.Payload)
	if payloadSigVerificationErr != nil {
		return payloadSigVerificationErr
	}
	if m.Envelope.SecretEnvelope != nil {
		payload := m.Envelope.SecretEnvelope.Payload
		sig := m.Envelope.SecretEnvelope.Signature
		if len(payload) == 0 {
			return errors.New("Empty payload")
		}
		if len(sig) == 0 {
			return errors.New("Empty signature")
		}
		return verify(peerIdentity, sig, payload)
	}
	return nil
}

// IsSigned returns whether the message
// has a signature in the envelope.
func (m *SignedGossipMessage) IsSigned() bool {
	return m.Envelope != nil && m.Envelope.Payload != nil && m.Envelope.Signature != nil
}

// ToGossipMessage un-marshals a given envelope and creates a
// SignedGossipMessage out of it.
// Returns an error if un-marshaling fails.
func (e *Envelope) ToGossipMessage() (*SignedGossipMessage, error) {
	if e == nil {
		return nil, errors.New("nil envelope")
	}
	msg := &GossipMessage{}
	err := proto.Unmarshal(e.Payload, msg)
	if err != nil {
		return nil, fmt.Errorf("Failed unmarshaling GossipMessage from envelope: %v", err)
	}
	return &SignedGossipMessage{
		GossipMessage: msg,
		Envelope:      e,
	}, nil
}

// SignSecret signs the secret payload and creates
// a secret envelope out of it.
func (e *Envelope) SignSecret(signer Signer, secret *Secret) error {
	payload, err := proto.Marshal(secret)
	if err != nil {
		return err
	}
	sig, err := signer(payload)
	if err != nil {
		return err
	}
	e.SecretEnvelope = &SecretEnvelope{
		Payload:   payload,
		Signature: sig,
	}
	return nil
}

// InternalEndpoint returns the internal endpoint
// in the secret envelope, or an empty string
// if a failure occurs.
func (s *SecretEnvelope) InternalEndpoint() string {
	secret := &Secret{}
	if err := proto.Unmarshal(s.Payload, secret); err != nil {
		return ""
	}
	return secret.GetInternalEndpoint()
}

// SignedGossipMessage contains a GossipMessage
// and the Envelope from which it came from
type SignedGossipMessage struct {
	*Envelope
	*GossipMessage
}

// toString of Payload prints Block message: Data and seq
func (p *Payload) toString() string {
	return fmt.Sprintf("Block message: {Data: %d bytes, seq: %d}", len(p.Data), p.SeqNum)
}

// toString of DataUpdate prints Type, items and nonce
func (du *DataUpdate) toString() string {
	mType := PullMsgType_name[int32(du.MsgType)]
	return fmt.Sprintf("Type: %s, items: %d, nonce: %d", mType, len(du.Data), du.Nonce)
}

// ToString of MembershipResponse prints number of Alive and number of Dead
func (mr *MembershipResponse) ToString() string {
	return fmt.Sprintf("MembershipResponse with Alive: %d, Dead: %d", len(mr.Alive), len(mr.Dead))
}

// toString of StateInfoSnapshot prints items
func (sis *StateInfoSnapshot) toString() string {
	return fmt.Sprintf("StateInfoSnapshot with %d items", len(sis.Elements))
}

// toString of MembershipRequest prints self information
func (mr *MembershipRequest) toString() string {
	if mr.SelfInformation == nil {
		return ""
	}
	signGM, err := mr.SelfInformation.ToGossipMessage()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("Membership Request with self information of %s ", signGM.String())
}

// ToString of Member prints Endpoint and PKI-id
func (member *Member) ToString() string {
	return fmt.Sprint("Membership: Endpoint:", member.Endpoint, " PKI-id:", hex.EncodeToString(member.PkiId))
}

// ToString of AliveMessage prints Alive Message, Identity and Timestamp
func (am *AliveMessage) ToString() string {
	if am.Membership == nil {
		return "nil Membership"
	}
	var sI string
	serializeIdentity := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(am.Identity, serializeIdentity); err == nil {
		sI = serializeIdentity.Mspid + string(serializeIdentity.IdBytes)
	}
	return fmt.Sprint("Alive Message:", am.Membership.ToString(), "Identity:", sI, "Timestamp:", am.Timestamp)
}

// toString of StateInfoPullRequest prints Channel MAC
func (sipr *StateInfoPullRequest) toString() string {
	return fmt.Sprint("state_info_pull_req: Channel MAC:", hex.EncodeToString(sipr.Channel_MAC))
}

// toString of StateInfo prints Timestamp and PKI-id
func (si *StateInfo) toString() string {
	return fmt.Sprint("state_info_message: Timestamp:", si.Timestamp, "PKI-id:", hex.EncodeToString(si.PkiId),
		" channel MAC:", hex.EncodeToString(si.Channel_MAC), " properties:", si.Properties)
}

// formatDigests formats digest byte arrays into strings depending on the message type
func formatDigests(msgType PullMsgType, givenDigests [][]byte) []string {
	var digests []string
	switch msgType {
	case PullMsgType_BLOCK_MSG:
		for _, digest := range givenDigests {
			digests = append(digests, string(digest))
		}
	case PullMsgType_IDENTITY_MSG:
		for _, digest := range givenDigests {
			digests = append(digests, hex.EncodeToString(digest))
		}

	}
	return digests
}

// toString of DataDigest prints nonce, msg_type and digests
func (dig *DataDigest) toString() string {
	var digests []string
	digests = formatDigests(dig.MsgType, dig.Digests)
	return fmt.Sprintf("data_dig: nonce: %d , Msg_type: %s, digests: %v", dig.Nonce, dig.MsgType, digests)
}

// toString of DataRequest prints nonce, msg_type and digests
func (dataReq *DataRequest) toString() string {
	var digests []string
	digests = formatDigests(dataReq.MsgType, dataReq.Digests)
	return fmt.Sprintf("data request: nonce: %d , Msg_type: %s, digests: %v", dataReq.Nonce, dataReq.MsgType, digests)
}

// toString of LeadershipMessage prints PKI-id, Timestamp and Is Declaration
func (lm *LeadershipMessage) toString() string {
	return fmt.Sprint("Leadership Message: PKI-id:", hex.EncodeToString(lm.PkiId), " Timestamp:", lm.Timestamp,
		"Is Declaration ", lm.IsDeclaration)
}

// String returns a string representation
// of a SignedGossipMessage
func (m *SignedGossipMessage) String() string {
	env := "No envelope"
	if m.Envelope != nil {
		var secretEnv string
		if m.SecretEnvelope != nil {
			pl := len(m.SecretEnvelope.Payload)
			sl := len(m.SecretEnvelope.Signature)
			secretEnv = fmt.Sprintf(" Secret payload: %d bytes, Secret Signature: %d bytes", pl, sl)
		}
		env = fmt.Sprintf("%d bytes, Signature: %d bytes%s", len(m.Envelope.Payload), len(m.Envelope.Signature), secretEnv)
	}
	gMsg := "No gossipMessage"
	if m.GossipMessage != nil {
		var isSimpleMsg bool
		if m.GetStateResponse() != nil {
			gMsg = fmt.Sprintf("StateResponse with %d items", len(m.GetStateResponse().Payloads))
		} else if m.IsDataMsg() && m.GetDataMsg().Payload != nil {
			gMsg = m.GetDataMsg().Payload.toString()
		} else if m.IsDataUpdate() {
			update := m.GetDataUpdate()
			gMsg = fmt.Sprintf("DataUpdate: %s", update.toString())
		} else if m.GetMemRes() != nil {
			gMsg = m.GetMemRes().ToString()
		} else if m.IsStateInfoSnapshot() {
			gMsg = m.GetStateSnapshot().toString()
		} else if m.GetPrivateRes() != nil {
			gMsg = m.GetPrivateRes().ToString()
		} else if m.GetAliveMsg() != nil {
			gMsg = m.GetAliveMsg().ToString()
		} else if m.GetMemReq() != nil {
			gMsg = m.GetMemReq().toString()
		} else if m.GetStateInfoPullReq() != nil {
			gMsg = m.GetStateInfoPullReq().toString()
		} else if m.GetStateInfo() != nil {
			gMsg = m.GetStateInfo().toString()
		} else if m.GetDataDig() != nil {
			gMsg = m.GetDataDig().toString()
		} else if m.GetDataReq() != nil {
			gMsg = m.GetDataReq().toString()
		} else if m.GetLeadershipMsg() != nil {
			gMsg = m.GetLeadershipMsg().toString()
		} else {
			gMsg = m.GossipMessage.String()
			isSimpleMsg = true
		}
		if !isSimpleMsg {
			desc := fmt.Sprintf("Channel: %s, nonce: %d, tag: %s", string(m.Channel), m.Nonce, GossipMessage_Tag_name[int32(m.Tag)])
			gMsg = fmt.Sprintf("%s %s", desc, gMsg)
		}
	}
	return fmt.Sprintf("GossipMessage: %v, Envelope: %s", gMsg, env)
}

// ToString returns a string representation of this RemotePvtDataResponse
func (res *RemotePvtDataResponse) ToString() string {
	a := make([]string, len(res.Elements))
	for i, el := range res.Elements {
		a[i] = fmt.Sprintf("%s with %d elements", el.Digest.String(), len(el.Payload))
	}
	return fmt.Sprintf("%v", a)
}
