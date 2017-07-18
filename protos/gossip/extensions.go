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
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
)

// NewGossipMessageComparator creates a MessageReplacingPolicy given a maximum number of blocks to hold
func NewGossipMessageComparator(dataBlockStorageSize int) common.MessageReplacingPolicy {
	return (&msgComparator{dataBlockStorageSize: dataBlockStorageSize}).getMsgReplacingPolicy()
}

type msgComparator struct {
	dataBlockStorageSize int
}

func (mc *msgComparator) getMsgReplacingPolicy() common.MessageReplacingPolicy {
	return func(this interface{}, that interface{}) common.InvalidationResult {
		return mc.invalidationPolicy(this, that)
	}
}

func (mc *msgComparator) invalidationPolicy(this interface{}, that interface{}) common.InvalidationResult {
	thisMsg := this.(*SignedGossipMessage)
	thatMsg := that.(*SignedGossipMessage)

	if thisMsg.IsAliveMsg() && thatMsg.IsAliveMsg() {
		return aliveInvalidationPolicy(thisMsg.GetAliveMsg(), thatMsg.GetAliveMsg())
	}

	if thisMsg.IsDataMsg() && thatMsg.IsDataMsg() {
		return mc.dataInvalidationPolicy(thisMsg.GetDataMsg(), thatMsg.GetDataMsg())
	}

	if thisMsg.IsStateInfoMsg() && thatMsg.IsStateInfoMsg() {
		return mc.stateInvalidationPolicy(thisMsg.GetStateInfo(), thatMsg.GetStateInfo())
	}

	if thisMsg.IsIdentityMsg() && thatMsg.IsIdentityMsg() {
		return mc.identityInvalidationPolicy(thisMsg.GetPeerIdentity(), thatMsg.GetPeerIdentity())
	}

	if thisMsg.IsLeadershipMsg() && thatMsg.IsLeadershipMsg() {
		return leaderInvalidationPolicy(thisMsg.GetLeadershipMsg(), thatMsg.GetLeadershipMsg())
	}

	return common.MessageNoAction
}

func (mc *msgComparator) stateInvalidationPolicy(thisStateMsg *StateInfo, thatStateMsg *StateInfo) common.InvalidationResult {
	if !bytes.Equal(thisStateMsg.PkiId, thatStateMsg.PkiId) {
		return common.MessageNoAction
	}
	return compareTimestamps(thisStateMsg.Timestamp, thatStateMsg.Timestamp)
}

func (mc *msgComparator) identityInvalidationPolicy(thisIdentityMsg *PeerIdentity, thatIdentityMsg *PeerIdentity) common.InvalidationResult {
	if bytes.Equal(thisIdentityMsg.PkiId, thatIdentityMsg.PkiId) {
		return common.MessageInvalidated
	}

	return common.MessageNoAction
}

func (mc *msgComparator) dataInvalidationPolicy(thisDataMsg *DataMessage, thatDataMsg *DataMessage) common.InvalidationResult {
	if thisDataMsg.Payload.SeqNum == thatDataMsg.Payload.SeqNum {
		return common.MessageInvalidated
	}

	diff := abs(thisDataMsg.Payload.SeqNum, thatDataMsg.Payload.SeqNum)
	if diff <= uint64(mc.dataBlockStorageSize) {
		return common.MessageNoAction
	}

	if thisDataMsg.Payload.SeqNum > thatDataMsg.Payload.SeqNum {
		return common.MessageInvalidates
	}
	return common.MessageInvalidated
}

func aliveInvalidationPolicy(thisMsg *AliveMessage, thatMsg *AliveMessage) common.InvalidationResult {
	if !bytes.Equal(thisMsg.Membership.PkiId, thatMsg.Membership.PkiId) {
		return common.MessageNoAction
	}

	return compareTimestamps(thisMsg.Timestamp, thatMsg.Timestamp)
}

func leaderInvalidationPolicy(thisMsg *LeadershipMessage, thatMsg *LeadershipMessage) common.InvalidationResult {
	if !bytes.Equal(thisMsg.PkiId, thatMsg.PkiId) {
		return common.MessageNoAction
	}

	return compareTimestamps(thisMsg.Timestamp, thatMsg.Timestamp)
}

func compareTimestamps(thisTS *PeerTime, thatTS *PeerTime) common.InvalidationResult {
	if thisTS.IncNum == thatTS.IncNum {
		if thisTS.SeqNum > thatTS.SeqNum {
			return common.MessageInvalidates
		}

		return common.MessageInvalidated
	}
	if thisTS.IncNum < thatTS.IncNum {
		return common.MessageInvalidated
	}
	return common.MessageInvalidates
}

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

// MsgConsumer invokes code given a SignedGossipMessage
type MsgConsumer func(message *SignedGossipMessage)

// IdentifierExtractor extracts from a SignedGossipMessage an identifier
type IdentifierExtractor func(*SignedGossipMessage) string

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

// ReceivedMessage is a GossipMessage wrapper that
// enables the user to send a message to the origin from which
// the ReceivedMessage was sent from.
// It also allows to know the identity of the sender,
// to obtain the raw bytes the GossipMessage was un-marshaled from,
// and the signature over these raw bytes.
type ReceivedMessage interface {

	// Respond sends a GossipMessage to the origin from which this ReceivedMessage was sent from
	Respond(msg *GossipMessage)

	// GetGossipMessage returns the underlying GossipMessage
	GetGossipMessage() *SignedGossipMessage

	// GetSourceMessage Returns the Envelope the ReceivedMessage was
	// constructed with
	GetSourceEnvelope() *Envelope

	// GetConnectionInfo returns information about the remote peer
	// that sent the message
	GetConnectionInfo() *ConnectionInfo
}

// ConnectionInfo represents information about
// the remote peer that sent a certain ReceivedMessage
type ConnectionInfo struct {
	ID       common.PKIidType
	Auth     *AuthInfo
	Identity api.PeerIdentityType
	Endpoint string
}

// String returns a string representation of this ConnectionInfo
func (c *ConnectionInfo) String() string {
	return fmt.Sprintf("%s %v", c.Endpoint, c.ID)
}

// IsAuthenticated returns whether the connection to the remote peer
// was authenticated when the handshake took place
func (c *ConnectionInfo) IsAuthenticated() bool {
	return c.Auth != nil
}

// AuthInfo represents the authentication
// data that was provided by the remote peer
// at the connection time
type AuthInfo struct {
	SignedData []byte
	Signature  []byte
}

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

func (p *Payload) toString() string {
	return fmt.Sprintf("Block message: {Data: %d bytes, seq: %d}", len(p.Data), p.SeqNum)
}

func (du *DataUpdate) toString() string {
	mType := PullMsgType_name[int32(du.MsgType)]
	return fmt.Sprintf("Type: %s, items: %d, nonce: %d", mType, len(du.Data), du.Nonce)
}

func (mr *MembershipResponse) toString() string {
	return fmt.Sprintf("MembershipResponse with Alive: %d, Dead: %d", len(mr.Alive), len(mr.Dead))
}

func (sis *StateInfoSnapshot) toString() string {
	return fmt.Sprintf("StateInfoSnapshot with %d items", len(sis.Elements))
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
			gMsg = m.GetMemRes().toString()
		} else if m.IsStateInfoSnapshot() {
			gMsg = m.GetStateSnapshot().toString()
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

// Abs returns abs(a-b)
func abs(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}
