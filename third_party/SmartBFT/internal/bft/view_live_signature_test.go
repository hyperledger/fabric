// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"errors"
	"testing"

	"github.com/hyperledger/SmartBFT/pkg/api"
	"github.com/hyperledger/SmartBFT/pkg/types"
	protos "github.com/hyperledger/SmartBFT/smartbftprotos"
)

type liveVoteTestVerifier struct {
	api.Verifier
	err        error
	liveChecks int
}

func (v *liveVoteTestVerifier) VerifyLiveConsenterSig(types.Signature, types.Proposal) ([]byte, error) {
	v.liveChecks++
	return nil, v.err
}

type liveVoteTestLogger struct{ api.Logger }

func (liveVoteTestLogger) Warnf(string, ...any) {}

func TestLiveVoteUsesCurrentAuthorization(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "expired"}[expired], func(t *testing.T) {
			verifier := &liveVoteTestVerifier{}
			if expired {
				verifier.err = errors.New("certificate has expired")
			}
			vv := &voteVerifier{
				v:              &View{Verifier: verifier, Logger: liveVoteTestLogger{}},
				proposal:       &types.Proposal{},
				expectedDigest: "digest",
				validVotes:     make(chan types.Signature, 1),
			}
			vv.verifyVote(&protos.Message{Content: &protos.Message_Commit{
				Commit: &protos.Commit{Digest: "digest", Signature: &protos.Signature{Signer: 3}},
			}})
			if verifier.liveChecks != 1 {
				t.Fatalf("expected one live authorization check, got %d", verifier.liveChecks)
			}
			expectedVotes := 1
			if expired {
				expectedVotes = 0
			}
			if len(vv.validVotes) != expectedVotes {
				t.Fatalf("expected %d accepted votes, got %d", expectedVotes, len(vv.validVotes))
			}
		})
	}
}
