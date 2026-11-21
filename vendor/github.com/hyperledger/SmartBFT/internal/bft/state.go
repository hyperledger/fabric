// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hyperledger/SmartBFT/pkg/api"
	"github.com/hyperledger/SmartBFT/pkg/types"
	protos "github.com/hyperledger/SmartBFT/smartbftprotos"
	"google.golang.org/protobuf/proto"
)

type StateRecorder struct {
	SavedMessages []*protos.SavedMessage
	TruncateTo    []bool
}

func (sr *StateRecorder) Save(message *protos.SavedMessage, truncateTo bool) error {
	sr.SavedMessages = append(sr.SavedMessages, message)
	sr.TruncateTo = append(sr.TruncateTo, truncateTo)
	return nil
}

func (*StateRecorder) Restore(_ *View) error {
	panic("should not be used")
}

type PersistedState struct {
	InFlightProposal *InFlightData
	Entries          [][]byte
	Logger           api.Logger
	WAL              api.WriteAheadLog
}

func (ps *PersistedState) Save(msgToSave *protos.SavedMessage, truncateTo bool) error {
	if proposed := msgToSave.GetProposedRecord(); proposed != nil {
		ps.storeProposal(proposed)
	}
	if prepared := msgToSave.GetCommit(); prepared != nil {
		ps.storePrepared(prepared)
	}

	b, err := proto.Marshal(msgToSave)
	if err != nil {
		ps.Logger.Panicf("Failed marshaling message: %v", err)
	}
	// The caller decides whether this record is a truncation point and passes the
	// flag explicitly. Save stores the flag as given and never infers it from the
	// message type or from shared state.
	return ps.WAL.Append(b, truncateTo)
}

func (ps *PersistedState) storeProposal(proposed *protos.ProposedRecord) {
	proposal := proposed.GetPrePrepare().GetProposal()
	proposalToStore := types.Proposal{
		VerificationSequence: int64(proposal.GetVerificationSequence()),
		Header:               proposal.GetHeader(),
		Payload:              proposal.GetPayload(),
		Metadata:             proposal.GetMetadata(),
	}
	ps.InFlightProposal.StoreProposal(proposalToStore)
}

func (ps *PersistedState) storePrepared(commitMsg *protos.Message) {
	cmt := commitMsg.GetCommit()
	ps.InFlightProposal.StorePrepares(cmt.GetView(), cmt.GetSeq())
}

func (ps *PersistedState) LoadNewViewIfApplicable() (*types.ViewAndSeq, error) {
	entries := ps.Entries
	if len(entries) == 0 {
		ps.Logger.Infof("Nothing to restore")
		return nil, nil
	}

	// A proposal or a commit can be appended after the new view record, so the
	// new view is not necessarily the last entry. Walk backwards to the latest
	// view transition.
	for i := range slices.Backward(entries) {
		persistedMessage := &protos.SavedMessage{}
		if err := proto.Unmarshal(entries[i], persistedMessage); err != nil {
			ps.Logger.Errorf("Failed unmarshaling entry %d from WAL: %v", i, err)
			return nil, fmt.Errorf("failed unmarshaling entry %d from WAL: %w", i, err)
		}
		if newViewMsg := persistedMessage.GetNewView(); newViewMsg != nil {
			ps.Logger.Infof("found a newView record in WAL")
			return &types.ViewAndSeq{View: newViewMsg.GetViewId(), Seq: newViewMsg.GetLatestSequence()}, nil
		}
		if persistedMessage.GetViewChange() != nil {
			// A later view change supersedes any new view before it.
			// LoadViewChangeIfApplicable sets the starting view in that case.
			ps.Logger.Infof("found a viewChange record after the last newView record")
			return nil, nil
		}
	}
	return nil, nil
}

// LoadLastProposalView returns the metadata of the latest proposal record.
// The built-in WAL drops a NewView record when the next pre-prepare is saved
// with truncateTo=true, so after a restart the proposal can be the only record
// that defines the view. A later NewView or ViewChange record takes precedence
// and makes this method return nil: LoadNewViewIfApplicable and
// LoadViewChangeIfApplicable handle those records.
func (ps *PersistedState) LoadLastProposalView() (*protos.ViewMetadata, error) {
	for i := range slices.Backward(ps.Entries) {
		persistedMessage := &protos.SavedMessage{}
		if err := proto.Unmarshal(ps.Entries[i], persistedMessage); err != nil {
			ps.Logger.Errorf("Failed unmarshaling entry %d from WAL: %v", i, err)
			return nil, fmt.Errorf("failed unmarshaling entry %d from WAL: %w", i, err)
		}
		if persistedMessage.GetNewView() != nil || persistedMessage.GetViewChange() != nil {
			return nil, nil
		}
		proposed := persistedMessage.GetProposedRecord()
		if proposed == nil {
			// A commit record always follows its proposal record, so keep
			// walking backwards to find the proposal that carries the metadata.
			continue
		}
		prePrepare := proposed.GetPrePrepare()
		if prePrepare == nil {
			return nil, errors.New("proposed record has no pre-prepare")
		}

		metadata := &protos.ViewMetadata{}
		if err := proto.Unmarshal(prePrepare.GetProposal().GetMetadata(), metadata); err != nil {
			return nil, fmt.Errorf("failed unmarshaling proposal metadata: %w", err)
		}
		return metadata, nil
	}
	return nil, nil
}

func (ps *PersistedState) LoadViewChangeIfApplicable() (*protos.ViewChange, error) {
	entries := ps.Entries
	if len(entries) == 0 {
		ps.Logger.Infof("Nothing to restore")
		return nil, nil
	}
	lastEntry := entries[len(entries)-1]
	lastPersistedMessage := &protos.SavedMessage{}
	if err := proto.Unmarshal(lastEntry, lastPersistedMessage); err != nil {
		ps.Logger.Errorf("Failed unmarshaling last entry from WAL: %v", err)
		return nil, fmt.Errorf("failed unmarshaling last entry from WAL: %w", err)
	}
	if viewChangeMsg := lastPersistedMessage.GetViewChange(); viewChangeMsg != nil {
		ps.Logger.Infof("last entry in WAL is a viewChange message")
		return viewChangeMsg, nil
	}
	return nil, nil
}

func (ps *PersistedState) Restore(v *View) error {
	// Unless we conclude otherwise, we're in a COMMITTED state
	v.Phase = COMMITTED

	entries := ps.Entries
	if len(entries) == 0 {
		ps.Logger.Infof("Nothing to restore")
		return nil
	}

	ps.Logger.Infof("WAL contains %d entries", len(entries))

	lastEntry := entries[len(entries)-1]
	lastPersistedMessage := &protos.SavedMessage{}
	if err := proto.Unmarshal(lastEntry, lastPersistedMessage); err != nil {
		ps.Logger.Errorf("Failed unmarshaling last entry from WAL: %v", err)
		return fmt.Errorf("failed unmarshaling last entry from WAL: %w", err)
	}

	if proposed := lastPersistedMessage.GetProposedRecord(); proposed != nil {
		return ps.recoverProposed(proposed, v)
	}

	if commitMsg := lastPersistedMessage.GetCommit(); commitMsg != nil {
		return ps.recoverPrepared(commitMsg, v, entries)
	}

	if newViewMsg := lastPersistedMessage.GetNewView(); newViewMsg != nil {
		ps.Logger.Infof("last entry in WAL is a newView record")
		return ps.restoreInFlightBeforeViewRecord(v)
	}

	if viewChangeMsg := lastPersistedMessage.GetViewChange(); viewChangeMsg != nil {
		ps.Logger.Infof("last entry in WAL is a viewChange message")
		return ps.restoreInFlightBeforeViewRecord(v)
	}

	return fmt.Errorf("unrecognized record: %v", lastPersistedMessage)
}

// restoreInFlightBeforeViewRecord restores the proposal that a trailing NewView or
// ViewChange record kept in the WAL because it was still needed for a view change.
// Only the in-flight data is restored: the view record defines the current view, so
// the view phase, number and sequence stay as they are.
func (ps *PersistedState) restoreInFlightBeforeViewRecord(v *View) error {
	for i := len(ps.Entries) - 2; i >= 0; i-- {
		record := &protos.SavedMessage{}
		if err := proto.Unmarshal(ps.Entries[i], record); err != nil {
			return fmt.Errorf("failed unmarshaling record %d from WAL: %w", i, err)
		}

		if commitMsg := record.GetCommit(); commitMsg != nil {
			if i == 0 {
				return errors.New("commit record has no preceding pre-prepare")
			}

			prePrepareRecord := &protos.SavedMessage{}
			if err := proto.Unmarshal(ps.Entries[i-1], prePrepareRecord); err != nil {
				return fmt.Errorf("failed unmarshaling record %d from WAL: %w", i-1, err)
			}

			proposed := prePrepareRecord.GetProposedRecord()
			if proposed == nil {
				return fmt.Errorf("expected the record before the commit to be a pre-prepare, but got '%v' instead", prePrepareRecord)
			}

			return ps.restoreInFlightProposal(proposed, commitMsg, v)
		}

		if proposed := record.GetProposedRecord(); proposed != nil {
			return ps.restoreInFlightProposal(proposed, nil, v)
		}

		// NewView and ViewChange records can sit between the proposal pair and the
		// end of the WAL; keep looking for the pair they kept alive.
	}

	return nil
}

func (ps *PersistedState) restoreInFlightProposal(proposed *protos.ProposedRecord, commitMsg *protos.Message, v *View) error {
	prePrepare := proposed.GetPrePrepare()
	if prePrepare == nil {
		return errors.New("proposed record has no pre-prepare")
	}

	seq := prePrepare.GetSeq()
	if v.ProposalSequence < seq {
		return fmt.Errorf("last proposal sequence persisted into WAL is %d which is greater than last committed sequence is %d", seq, v.ProposalSequence)
	}
	if v.ProposalSequence > seq {
		ps.Logger.Infof("Last proposal with sequence %d has been safely committed", seq)
		return nil
	}

	ps.storeProposal(proposed)
	if commitMsg != nil {
		ps.storePrepared(commitMsg)
	}

	return nil
}

func (ps *PersistedState) recoverProposed(lastPersistedMessage *protos.ProposedRecord, v *View) error {
	// A sync can deliver the proposal without writing to the WAL, so the record
	// may be older than the sequence the view starts at.
	if seq := lastPersistedMessage.GetPrePrepare().GetSeq(); v.ProposalSequence > seq {
		ps.Logger.Infof("Last proposal with sequence %d has been safely committed", seq)
		return nil
	}

	prop := lastPersistedMessage.GetPrePrepare().GetProposal()
	v.inFlightProposal = &types.Proposal{
		VerificationSequence: int64(prop.GetVerificationSequence()),
		Metadata:             prop.GetMetadata(),
		Payload:              prop.GetPayload(),
		Header:               prop.GetHeader(),
	}
	restoreInFlightRequests(v)
	ps.storeProposal(lastPersistedMessage)
	// Reconstruct the prepare message we shall next broadcast
	// after the recovery.
	prp := lastPersistedMessage.GetPrePrepare()
	v.lastBroadcastSent = &protos.Message{
		Content: &protos.Message_Prepare{
			Prepare: lastPersistedMessage.GetPrepare(),
		},
	}
	v.Phase = PROPOSED
	v.Number = prp.GetView()
	v.ProposalSequence = prp.GetSeq()
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prop.GetMetadata(), md); err != nil {
		ps.Logger.Panicf("Failed to unmarshal proposal metadata, error: %v", err)
	}
	v.DecisionsInView = md.GetDecisionsInView()
	ps.Logger.Infof("Restored proposal with sequence %d", lastPersistedMessage.GetPrePrepare().GetSeq())
	return nil
}

func (ps *PersistedState) recoverPrepared(lastPersistedMessage *protos.Message, v *View, entries [][]byte) error {
	// Last entry is a commit, so we should have not pruned the previous pre-prepare
	if len(entries) < 2 {
		return errors.New("last message is a commit, but expected to also have a matching pre-prepare")
	}
	prePrepareMsg := &protos.SavedMessage{}
	if err := proto.Unmarshal(entries[len(entries)-2], prePrepareMsg); err != nil {
		ps.Logger.Errorf("Failed unmarshaling second last entry from WAL: %v", err)
		return fmt.Errorf("failed unmarshaling last entry from WAL: %w", err)
	}

	prePrepareFromWAL := prePrepareMsg.GetProposedRecord().GetPrePrepare()

	if prePrepareFromWAL == nil {
		return fmt.Errorf("expected second last message to be a pre-prepare, but got '%v' instead", prePrepareMsg)
	}

	if v.ProposalSequence < prePrepareFromWAL.GetSeq() {
		err := fmt.Errorf("last proposal sequence persisted into WAL is %d which is greater than last committed sequence is %d", prePrepareFromWAL.GetSeq(), v.ProposalSequence)
		ps.Logger.Errorf("Failed recovery: %s", err)
		return err
	}

	// Check if the WAL's last sequence has been persisted into the application layer.
	if v.ProposalSequence > prePrepareFromWAL.GetSeq() {
		ps.Logger.Infof("Last proposal with sequence %d has been safely committed", v.ProposalSequence)
		return nil
	}

	// Else, v.ProposalSequence == prePrepareFromWAL.Seq

	prop := prePrepareFromWAL.GetProposal()
	v.inFlightProposal = &types.Proposal{
		VerificationSequence: int64(prop.GetVerificationSequence()),
		Metadata:             prop.GetMetadata(),
		Payload:              prop.GetPayload(),
		Header:               prop.GetHeader(),
	}
	restoreInFlightRequests(v)
	ps.storeProposal(prePrepareMsg.GetProposedRecord())
	ps.storePrepared(lastPersistedMessage)

	// Reconstruct the commit message we shall next broadcast
	// after the recovery.
	v.lastBroadcastSent = lastPersistedMessage
	v.Phase = PREPARED
	v.Number = prePrepareFromWAL.GetView()
	v.ProposalSequence = prePrepareFromWAL.GetSeq()
	md := &protos.ViewMetadata{}
	if err := proto.Unmarshal(prop.GetMetadata(), md); err != nil {
		ps.Logger.Panicf("Failed to unmarshal proposal metadata, error: %v", err)
	}
	v.DecisionsInView = md.GetDecisionsInView()

	// Restore signature
	signatureInLastSentCommit := v.lastBroadcastSent.GetCommit().GetSignature()
	v.myProposalSig = &types.Signature{
		ID:    signatureInLastSentCommit.GetSigner(),
		Msg:   signatureInLastSentCommit.GetMsg(),
		Value: signatureInLastSentCommit.GetValue(),
	}

	ps.Logger.Infof("Restored proposal with sequence %d", prePrepareFromWAL.GetSeq())
	return nil
}

// restoreInFlightRequests sets what a view sets when it accepts a proposal and that is not persisted in the WAL,
// so that once the restored proposal is decided its requests are removed from the pool,
// and the time it took to process it is measured from the restore.
func restoreInFlightRequests(v *View) {
	v.inFlightRequests = v.Verifier.RequestsFromProposal(*v.inFlightProposal)
	v.beginPrePrepare = time.Now()
}
