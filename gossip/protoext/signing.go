/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"errors"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/protos/gossip"
)

// Signer signs a message, and returns (signature, nil)
// on success, and nil and an error on failure.
type Signer func(msg []byte) ([]byte, error)

// Verifier receives a peer identity, a signature and a message and returns nil
// if the signature on the message could be verified using the given identity.
type Verifier func(peerIdentity []byte, signature, message []byte) error

// SignSecret signs the secret payload and creates a secret envelope out of it.
func SignSecret(e *gossip.Envelope, signer Signer, secret *gossip.Secret) error {
	payload, err := proto.Marshal(secret)
	if err != nil {
		return err
	}
	sig, err := signer(payload)
	if err != nil {
		return err
	}
	e.SecretEnvelope = &gossip.SecretEnvelope{
		Payload:   payload,
		Signature: sig,
	}
	return nil
}

// NoopSign creates a SignedGossipMessage with a nil signature
func NoopSign(m *gossip.GossipMessage) (*SignedGossipMessage, error) {
	signer := func(msg []byte) ([]byte, error) {
		return nil, nil
	}
	sMsg := &SignedGossipMessage{
		GossipMessage: m,
	}
	_, err := sMsg.Sign(signer)
	return sMsg, err
}

// EnvelopeToGossipMessage un-marshals a given envelope and creates a
// SignedGossipMessage out of it.
// Returns an error if un-marshaling fails.
func EnvelopeToGossipMessage(e *gossip.Envelope) (*SignedGossipMessage, error) {
	if e == nil {
		return nil, errors.New("nil envelope")
	}
	msg := &gossip.GossipMessage{}
	err := proto.Unmarshal(e.Payload, msg)
	if err != nil {
		return nil, fmt.Errorf("Failed unmarshaling GossipMessage from envelope: %v", err)
	}
	return &SignedGossipMessage{
		GossipMessage: msg,
		Envelope:      e,
	}, nil
}

// SignedGossipMessage contains a GossipMessage and the Envelope from which it
// came from
type SignedGossipMessage struct {
	*gossip.Envelope
	*gossip.GossipMessage
}

// Sign signs a GossipMessage with given Signer.
// Returns an Envelope on success, panics on failure.
func (m *SignedGossipMessage) Sign(signer Signer) (*gossip.Envelope, error) {
	// If we have a secretEnvelope, don't override it.
	// Back it up, and restore it later
	var secretEnvelope *gossip.SecretEnvelope
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

	e := &gossip.Envelope{
		Payload:        payload,
		Signature:      sig,
		SecretEnvelope: secretEnvelope,
	}
	m.Envelope = e
	return e, nil
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
		} else if IsDataMsg(m.GossipMessage) && m.GetDataMsg().Payload != nil {
			gMsg = m.GetDataMsg().Payload.ToString()
		} else if IsDataUpdate(m.GossipMessage) {
			update := m.GetDataUpdate()
			gMsg = fmt.Sprintf("DataUpdate: %s", update.ToString())
		} else if m.GetMemRes() != nil {
			gMsg = m.GetMemRes().ToString()
		} else if IsStateInfoSnapshot(m.GossipMessage) {
			gMsg = m.GetStateSnapshot().ToString()
		} else if m.GetPrivateRes() != nil {
			gMsg = m.GetPrivateRes().ToString()
		} else if m.GetAliveMsg() != nil {
			gMsg = m.GetAliveMsg().ToString()
		} else if m.GetMemReq() != nil {
			gMsg = m.GetMemReq().ToString()
		} else if m.GetStateInfoPullReq() != nil {
			gMsg = m.GetStateInfoPullReq().ToString()
		} else if m.GetStateInfo() != nil {
			gMsg = m.GetStateInfo().ToString()
		} else if m.GetDataDig() != nil {
			gMsg = m.GetDataDig().ToString()
		} else if m.GetDataReq() != nil {
			gMsg = m.GetDataReq().ToString()
		} else if m.GetLeadershipMsg() != nil {
			gMsg = m.GetLeadershipMsg().ToString()
		} else {
			gMsg = m.GossipMessage.String()
			isSimpleMsg = true
		}
		if !isSimpleMsg {
			desc := fmt.Sprintf("Channel: %s, nonce: %d, tag: %s", string(m.Channel), m.Nonce, gossip.GossipMessage_Tag_name[int32(m.Tag)])
			gMsg = fmt.Sprintf("%s %s", desc, gMsg)
		}
	}
	return fmt.Sprintf("GossipMessage: %v, Envelope: %s", gMsg, env)
}
