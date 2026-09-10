/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"encoding/hex"
	"fmt"

	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"google.golang.org/protobuf/proto"
)

// MemberToString prints Endpoint and PKI-id
func MemberToString(m *gossip.Member) string {
	return fmt.Sprint("Membership: Endpoint:", m.GetEndpoint(), " PKI-id:", hex.EncodeToString(m.GetPkiId()))
}

// MembershipResponseToString of MembershipResponse prints number of Alive and number of Dead
func MembershipResponseToString(mr *gossip.MembershipResponse) string {
	return fmt.Sprintf("MembershipResponse with Alive: %d, Dead: %d", len(mr.GetAlive()), len(mr.GetDead()))
}

// AliveMessageToString of AliveMessage prints Alive Message, Identity and Timestamp
func AliveMessageToString(am *gossip.AliveMessage) string {
	if am.GetMembership() == nil {
		return "nil Membership"
	}
	var sI string
	serializeIdentity := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(am.GetIdentity(), serializeIdentity); err == nil {
		sI = serializeIdentity.GetMspid() + string(serializeIdentity.GetIdBytes())
	}
	return fmt.Sprint("Alive Message:", MemberToString(am.GetMembership()), "Identity:", sI, "Timestamp:", am.GetTimestamp())
}

// PayloadToString prints Block message: Data and seq
func PayloadToString(p *gossip.Payload) string {
	return fmt.Sprintf("Block message: {Data: %d bytes, seq: %d}", len(p.GetData()), p.GetSeqNum())
}

// DataUpdateToString prints Type, items and nonce
func DataUpdateToString(du *gossip.DataUpdate) string {
	mType := gossip.PullMsgType_name[int32(du.GetMsgType())]
	return fmt.Sprintf("Type: %s, items: %d, nonce: %d", mType, len(du.GetData()), du.GetNonce())
}

// StateInfoSnapshotToString prints items
func StateInfoSnapshotToString(sis *gossip.StateInfoSnapshot) string {
	return fmt.Sprintf("StateInfoSnapshot with %d items", len(sis.GetElements()))
}

// MembershipRequestToString prints self information
func MembershipRequestToString(mr *gossip.MembershipRequest) string {
	if mr.GetSelfInformation() == nil {
		return ""
	}
	signGM, err := EnvelopeToGossipMessage(mr.GetSelfInformation())
	if err != nil {
		return ""
	}
	return fmt.Sprintf("Membership Request with self information of %s ", signGM.String())
}

// StateInfoPullRequestToString prints Channel MAC
func StateInfoPullRequestToString(sipr *gossip.StateInfoPullRequest) string {
	return fmt.Sprint("state_info_pull_req: Channel MAC:", hex.EncodeToString(sipr.GetChannel_MAC()))
}

// StateInfoToString prints Timestamp and PKI-id
func StateInfoToString(si *gossip.StateInfo) string {
	return fmt.Sprint("state_info_message: Timestamp:", si.GetTimestamp(), "PKI-id:", hex.EncodeToString(si.GetPkiId()),
		" channel MAC:", hex.EncodeToString(si.GetChannel_MAC()), " properties:", si.GetProperties())
}

// formatDigests formats digest byte arrays into strings depending on the message type
func formatDigests(msgType gossip.PullMsgType, givenDigests [][]byte) []string {
	var digests []string
	switch msgType {
	case gossip.PullMsgType_BLOCK_MSG:
		for _, digest := range givenDigests {
			digests = append(digests, string(digest))
		}
	case gossip.PullMsgType_IDENTITY_MSG:
		for _, digest := range givenDigests {
			digests = append(digests, hex.EncodeToString(digest))
		}

	}
	return digests
}

// DataDigestToString prints nonce, msg_type and digests
func DataDigestToString(dig *gossip.DataDigest) string {
	digests := formatDigests(dig.GetMsgType(), dig.GetDigests())
	return fmt.Sprintf("data_dig: nonce: %d , Msg_type: %s, digests: %v", dig.GetNonce(), dig.GetMsgType(), digests)
}

// DataRequestToString prints nonce, msg_type and digests
func DataRequestToString(dataReq *gossip.DataRequest) string {
	digests := formatDigests(dataReq.GetMsgType(), dataReq.GetDigests())
	return fmt.Sprintf("data request: nonce: %d , Msg_type: %s, digests: %v", dataReq.GetNonce(), dataReq.GetMsgType(), digests)
}

// LeadershipMessageToString prints PKI-id, Timestamp and Is Declaration
func LeadershipMessageToString(lm *gossip.LeadershipMessage) string {
	return fmt.Sprint("Leadership Message: PKI-id:", hex.EncodeToString(lm.GetPkiId()), " Timestamp:", lm.GetTimestamp(),
		"Is Declaration ", lm.GetIsDeclaration())
}

// RemovePvtDataResponseToString returns a string representation of this RemotePvtDataResponse
func RemovePvtDataResponseToString(res *gossip.RemotePvtDataResponse) string {
	a := make([]string, len(res.GetElements()))
	for i, el := range res.GetElements() {
		a[i] = fmt.Sprintf("%s with %d elements", el.GetDigest().String(), len(el.GetPayload()))
	}
	return fmt.Sprintf("%v", a)
}
