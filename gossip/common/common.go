/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
	"encoding/hex"
)

// PKIidType defines the type that holds the PKI-id
// which is the security identifier of a peer
type PKIidType []byte

func (p PKIidType) String() string {
	if p == nil {
		return "<nil>"
	}
	return hex.EncodeToString(p)
}

// IsNotSameFilter generate filter function which
// provides a predicate to identify whenever current id
// equals to another one.
func (id PKIidType) IsNotSameFilter(that PKIidType) bool {
	return !bytes.Equal(id, that)
}

// MessageAcceptor is a predicate that is used to
// determine in which messages the subscriber that created the
// instance of the MessageAcceptor is interested in.
type MessageAcceptor func(interface{}) bool

// Payload defines an object that contains a ledger block
type Payload struct {
	ChannelID ChannelID // The channel's ID of the block
	Data      []byte    // The content of the message, possibly encrypted or signed
	Hash      string    // The message hash
	SeqNum    uint64    // The message sequence number
}

// ChannelID defines the identity representation of a chain
type ChannelID []byte

func (c ChannelID) String() string {
	return string(c)
}

// MessageReplacingPolicy Returns:
// MESSAGE_INVALIDATES if this message invalidates that
// MESSAGE_INVALIDATED if this message is invalidated by that
// MESSAGE_NO_ACTION otherwise
type MessageReplacingPolicy func(this interface{}, that interface{}) InvalidationResult

// InvalidationResult determines how a message affects another message
// when it is put into gossip message store
type InvalidationResult int

const (
	// MessageNoAction means messages have no relation
	MessageNoAction InvalidationResult = iota
	// MessageInvalidates means message invalidates the other message
	MessageInvalidates
	// MessageInvalidated means message is invalidated by the other message
	MessageInvalidated
)
