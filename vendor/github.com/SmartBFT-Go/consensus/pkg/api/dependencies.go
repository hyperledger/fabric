// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package api

import (
	bft "github.com/SmartBFT-Go/consensus/pkg/types"
	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
)

// Application delivers the consented proposal and corresponding signatures.
type Application interface {
	// Deliver delivers the given proposal and signatures.
	// After the call returns we assume that this proposal is stored in persistent memory.
	// It returns whether this proposal was a reconfiguration and the current config.
	Deliver(proposal bft.Proposal, signature []bft.Signature) bft.Reconfig
}

// Comm enables the communications between the nodes.
type Comm interface {
	// SendConsensus sends the consensus protocol related message m to the node with id targetID.
	SendConsensus(targetID uint64, m *protos.Message)
	// SendTransaction sends the given client's request to the node with id targetID.
	SendTransaction(targetID uint64, request []byte)
	// Nodes returns a set of ids of participating nodes.
	// In case you need to change or keep this slice, create a copy.
	Nodes() []uint64
}

// Assembler creates proposals.
type Assembler interface {
	// AssembleProposal creates a proposal which includes
	// the given requests (when permitting) and metadata.
	AssembleProposal(metadata []byte, requests [][]byte) bft.Proposal
}

// WriteAheadLog is write ahead log.
type WriteAheadLog interface {
	// Append appends a data item to the end of the WAL
	// and indicate whether this entry is a truncation point.
	Append(entry []byte, truncateTo bool) error
}

// Signer signs on the given data.
type Signer interface {
	// Sign signs on the given data and returns the signature.
	Sign([]byte) []byte
	// SignProposal signs on the given proposal and returns a composite Signature.
	SignProposal(proposal bft.Proposal, auxiliaryInput []byte) *bft.Signature
}

// Verifier validates data and verifies signatures.
type Verifier interface {
	// VerifyProposal verifies the given proposal and returns the included requests' info.
	VerifyProposal(proposal bft.Proposal) ([]bft.RequestInfo, error)
	// VerifyRequest verifies the given request and returns its info.
	VerifyRequest(val []byte) (bft.RequestInfo, error)
	// VerifyConsenterSig verifies the signature for the given proposal.
	// It returns the auxiliary data in the signature.
	VerifyConsenterSig(signature bft.Signature, prop bft.Proposal) ([]byte, error)
	// VerifySignature verifies the signature.
	VerifySignature(signature bft.Signature) error
	// VerificationSequence returns the current verification sequence.
	VerificationSequence() uint64
	// RequestsFromProposal returns from the given proposal the included requests' info
	RequestsFromProposal(proposal bft.Proposal) []bft.RequestInfo
	// AuxiliaryData extracts the auxiliary data from a signature's message
	AuxiliaryData([]byte) []byte
}

// MembershipNotifier notifies if there was a membership change in the last proposal.
type MembershipNotifier interface {
	// MembershipChange returns true if there was a membership change in the last proposal.
	MembershipChange() bool
}

// RequestInspector extracts info (i.e. request id and client id) from a given request.
type RequestInspector interface {
	// RequestID returns info about the given request.
	RequestID(req []byte) bft.RequestInfo
}

// Synchronizer reaches the cluster nodes and fetches blocks in order to sync the replica's state.
type Synchronizer interface {
	// Sync blocks indefinitely until the replica's state is synchronized to the latest decision,
	// and returns it with info about reconfiguration.
	Sync() bft.SyncResponse
}

// Logger defines the contract for logging.
type Logger interface {
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Panicf(template string, args ...interface{})
}
