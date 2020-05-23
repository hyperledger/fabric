/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"encoding/hex"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/msp"
)

// MemberToString prints Endpoint and PKI-id
func MemberToString(m *gossip.Member) string {
	return fmt.Sprint("Membership: Endpoint:", m.Endpoint, " PKI-id:", hex.EncodeToString(m.PkiId))
}

// MembershipResponseToString of MembershipResponse prints number of Alive and number of Dead
func MembershipResponseToString(mr *gossip.MembershipResponse) string {
	return fmt.Sprintf("MembershipResponse with Alive: %d, Dead: %d", len(mr.Alive), len(mr.Dead))
}

// AliveMessageToString of AliveMessage prints Alive Message, Identity and Timestamp
func AliveMessageToString(am *gossip.AliveMessage) string {
	if am.Membership == nil {
		return "nil Membership"
	}
	var sI string
	serializeIdentity := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(am.Identity, serializeIdentity); err == nil {
		sI = serializeIdentity.Mspid + string(serializeIdentity.IdBytes)
	}
	return fmt.Sprint("Alive Message:", MemberToString(am.Membership), "Identity:", sI, "Timestamp:", am.Timestamp)
}

// PayloadToString prints Block message: Data and seq
func PayloadToString(p *gossip.Payload) string {
	return fmt.Sprintf("Block message: {Data: %d bytes, seq: %d}", len(p.Data), p.SeqNum)
}

// DataUpdateToString prints Type, items and nonce
func DataUpdateToString(du *gossip.DataUpdate) string {
	mType := gossip.PullMsgType_name[int32(du.MsgType)]
	return fmt.Sprintf("Type: %s, items: %d, nonce: %d", mType, len(du.Data), du.Nonce)
}

// StateInfoSnapshotToString prints items
func StateInfoSnapshotToString(sis *gossip.StateInfoSnapshot) string {
	return fmt.Sprintf("StateInfoSnapshot with %d items", len(sis.Elements))
}

// MembershipRequestToString prints self information
func MembershipRequestToString(mr *gossip.MembershipRequest) string {
	if mr.SelfInformation == nil {
		return ""
	}
	signGM, err := EnvelopeToGossipMessage(mr.SelfInformation)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("Membership Request with self information of %s ", signGM.String())
}

// StateInfoPullRequestToString prints Channel MAC
func StateInfoPullRequestToString(sipr *gossip.StateInfoPullRequest) string {
	return fmt.Sprint("state_info_pull_req: Channel MAC:", hex.EncodeToString(sipr.Channel_MAC))
}

// StateInfoToString prints Timestamp and PKI-id
func StateInfoToString(si *gossip.StateInfo) string {
	return fmt.Sprint("state_info_message: Timestamp:", si.Timestamp, "PKI-id:", hex.EncodeToString(si.PkiId),
		" channel MAC:", hex.EncodeToString(si.Channel_MAC), " properties:", si.Properties)
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
	digests := formatDigests(dig.MsgType, dig.Digests)
	return fmt.Sprintf("data_dig: nonce: %d , Msg_type: %s, digests: %v", dig.Nonce, dig.MsgType, digests)
}

// DataRequestToString prints nonce, msg_type and digests
func DataRequestToString(dataReq *gossip.DataRequest) string {
	digests := formatDigests(dataReq.MsgType, dataReq.Digests)
	return fmt.Sprintf("data request: nonce: %d , Msg_type: %s, digests: %v", dataReq.Nonce, dataReq.MsgType, digests)
}

// LeadershipMessageToString prints PKI-id, Timestamp and Is Declaration
func LeadershipMessageToString(lm *gossip.LeadershipMessage) string {
	return fmt.Sprint("Leadership Message: PKI-id:", hex.EncodeToString(lm.PkiId), " Timestamp:", lm.Timestamp,
		"Is Declaration ", lm.IsDeclaration)
}

// RemovePvtDataResponseToString returns a string representation of this RemotePvtDataResponse
func RemovePvtDataResponseToString(res *gossip.RemotePvtDataResponse) string {
	a := make([]string, len(res.Elements))
	for i, el := range res.Elements {
		a[i] = fmt.Sprintf("%s with %d elements", el.Digest.String(), len(el.Payload))
	}
	return fmt.Sprintf("%v", a)
}
