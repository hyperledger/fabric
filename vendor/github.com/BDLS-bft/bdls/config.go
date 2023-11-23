
package bdls

import (
	"crypto/ecdsa"
	"time"
)

const (
	// ConfigMinimumParticipants is the minimum number of participant allow in consensus protocol
	ConfigMinimumParticipants = 4
)

// Config is to config the parameters of BDLS consensus protocol
type Config struct {
	// the starting time point for consensus
	Epoch time.Time
	// CurrentHeight
	CurrentHeight uint64
	// PrivateKey
	PrivateKey *ecdsa.PrivateKey
	// Consensus Group
	Participants []Identity
	// EnableCommitUnicast sets to true to enable <commit> message to be delivered via unicast
	// if not(by default), <commit> message will be broadcasted
	EnableCommitUnicast bool

	// StateCompare is a function from user to compare states,
	// The result will be 0 if a==b, -1 if a < b, and +1 if a > b.
	// Usually this will lead to block header comparsion in blockchain, or replication log in database,
	// users should check fields in block header to make comparison.
	StateCompare func(a State, b State) int

	// StateValidate is a function from user to validate the integrity of
	// state data.
	StateValidate func(State) bool

	// MessageValidator is an external validator to be called when a message inputs into ReceiveMessage
	MessageValidator func(c *Consensus, m *Message, signed *SignedProto) bool

	// MessageOutCallback will be called if not nil before a message send out
	MessageOutCallback func(m *Message, signed *SignedProto)

	// Identity derviation from ecdsa.PublicKey
	// (optional). Default to DefaultPubKeyToIdentity
	PubKeyToIdentity func(pubkey *ecdsa.PublicKey) (ret Identity)
}

// VerifyConfig verifies the integrity of this config when creating new consensus object
func VerifyConfig(c *Config) error {
	if c.Epoch.IsZero() {
		return ErrConfigEpoch
	}

	if c.StateCompare == nil {
		return ErrConfigStateCompare
	}

	if c.StateValidate == nil {
		return ErrConfigStateValidate
	}

	if c.PrivateKey == nil {
		return ErrConfigPrivateKey
	}

	if len(c.Participants) < ConfigMinimumParticipants {
		return ErrConfigParticipants
	}

	return nil
}
