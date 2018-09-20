/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

type mutator func([]byte) []byte

func noopMutator(b []byte) []byte {
	return b
}

func corruptMutator(b []byte) []byte {
	b = append(b, 0)
	return b
}

func createX509Identity(t *testing.T, certFileName string) []byte {
	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", certFileName))
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: certBytes,
	}
	idBytes, err := proto.Marshal(sId)
	assert.NoError(t, err)
	return idBytes
}

func createIdemixIdentity(t *testing.T) []byte {
	idemixId := &msp.SerializedIdemixIdentity{
		NymX: []byte{1, 2, 3},
		NymY: []byte{1, 2, 3},
		Ou:   []byte("OU1"),
	}
	idemixBytes, err := proto.Marshal(idemixId)
	assert.NoError(t, err)
	sId := &msp.SerializedIdentity{
		IdBytes: idemixBytes,
	}
	idBytes, err := proto.Marshal(sId)
	assert.NoError(t, err)
	return idBytes
}

func createSignedProposal(t *testing.T, serializedIdentity []byte, corruptSigHdr mutator, corruptHdr mutator) *peer.SignedProposal {
	sHdr := utils.MakeSignatureHeader(serializedIdentity, nil)
	hdr := utils.MakePayloadHeader(&common.ChannelHeader{}, sHdr)
	hdr.SignatureHeader = corruptSigHdr(hdr.SignatureHeader)
	hdrBytes, err := proto.Marshal(hdr)
	assert.NoError(t, err)
	prop := &peer.Proposal{
		Header: hdrBytes,
	}
	prop.Header = corruptHdr(prop.Header)
	propBytes, err := proto.Marshal(prop)
	assert.NoError(t, err)
	return &peer.SignedProposal{
		ProposalBytes: propBytes,
	}
}

func createValidSignedProposal(t *testing.T, serializedIdentity []byte) *peer.SignedProposal {
	return createSignedProposal(t, serializedIdentity, noopMutator, noopMutator)
}

func createSignedProposalWithInvalidSigHeader(t *testing.T, serializedIdentity []byte) *peer.SignedProposal {
	return createSignedProposal(t, serializedIdentity, corruptMutator, noopMutator)
}

func createSignedProposalWithInvalidHeader(t *testing.T, serializedIdentity []byte) *peer.SignedProposal {
	return createSignedProposal(t, serializedIdentity, noopMutator, corruptMutator)
}

func TestExpirationCheckFilter(t *testing.T) {
	nextEndorser := &mockEndorserServer{}
	auth := NewExpirationCheckFilter()
	auth.Init(nextEndorser)

	// Scenario I: Expired x509 identity
	sp := createValidSignedProposal(t, createX509Identity(t, "expiredCert.pem"))
	_, err := auth.ProcessProposal(context.Background(), sp)
	assert.Equal(t, err.Error(), "identity expired")
	assert.False(t, nextEndorser.invoked)

	// Scenario II: Not expired x509 identity
	sp = createValidSignedProposal(t, createX509Identity(t, "notExpiredCert.pem"))
	_, err = auth.ProcessProposal(context.Background(), sp)
	assert.NoError(t, err)
	assert.True(t, nextEndorser.invoked)
	nextEndorser.invoked = false

	// Scenario III: Idemix identity
	sp = createValidSignedProposal(t, createIdemixIdentity(t))
	_, err = auth.ProcessProposal(context.Background(), sp)
	assert.NoError(t, err)
	assert.True(t, nextEndorser.invoked)
	nextEndorser.invoked = false

	// Scenario IV: Malformed proposal
	sp = createValidSignedProposal(t, createX509Identity(t, "notExpiredCert.pem"))
	sp.ProposalBytes = append(sp.ProposalBytes, 0)
	_, err = auth.ProcessProposal(context.Background(), sp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed parsing proposal")
	assert.False(t, nextEndorser.invoked)

	// Scenario V: Malformed signature header
	sp = createSignedProposalWithInvalidSigHeader(t, createX509Identity(t, "notExpiredCert.pem"))
	_, err = auth.ProcessProposal(context.Background(), sp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed parsing signature header")
	assert.False(t, nextEndorser.invoked)

	// Scenario VI: Malformed header
	sp = createSignedProposalWithInvalidHeader(t, createX509Identity(t, "notExpiredCert.pem"))
	_, err = auth.ProcessProposal(context.Background(), sp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed parsing header")
	assert.False(t, nextEndorser.invoked)
}
