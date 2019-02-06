/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"encoding/hex"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/msp"
)

func (p *Payload) ToString() string {
	return fmt.Sprintf("Block message: {Data: %d bytes, seq: %d}", len(p.Data), p.SeqNum)
}

// ToString of DataUpdate prints Type, items and nonce
func (du *DataUpdate) ToString() string {
	mType := PullMsgType_name[int32(du.MsgType)]
	return fmt.Sprintf("Type: %s, items: %d, nonce: %d", mType, len(du.Data), du.Nonce)
}

// ToString of MembershipResponse prints number of Alive and number of Dead
func (mr *MembershipResponse) ToString() string {
	return fmt.Sprintf("MembershipResponse with Alive: %d, Dead: %d", len(mr.Alive), len(mr.Dead))
}

// ToString of StateInfoSnapshot prints items
func (sis *StateInfoSnapshot) ToString() string {
	return fmt.Sprintf("StateInfoSnapshot with %d items", len(sis.Elements))
}

// ToString of MembershipRequest prints self information
func (mr *MembershipRequest) ToString() string {
	// if mr.SelfInformation == nil {
	// 	return ""
	// }
	// signGM, err := mr.SelfInformation.ToGossipMessage()
	// if err != nil {
	// 	return ""
	// }
	// return fmt.Sprintf("Membership Request with self information of %s ", signGM.String())
	return "implement me"
}

// ToString of Member prints Endpoint and PKI-id
func (member *Member) ToString() string {
	return fmt.Sprint("Membership: Endpoint:", member.Endpoint, " PKI-id:", hex.EncodeToString(member.PkiId))
}

// ToString of AliveMessage prints Alive Message, Identity and Timestamp
func (am *AliveMessage) ToString() string {
	if am.Membership == nil {
		return "nil Membership"
	}
	var sI string
	serializeIdentity := &msp.SerializedIdentity{}
	if err := proto.Unmarshal(am.Identity, serializeIdentity); err == nil {
		sI = serializeIdentity.Mspid + string(serializeIdentity.IdBytes)
	}
	return fmt.Sprint("Alive Message:", am.Membership.ToString(), "Identity:", sI, "Timestamp:", am.Timestamp)
}

// ToString of StateInfoPullRequest prints Channel MAC
func (sipr *StateInfoPullRequest) ToString() string {
	return fmt.Sprint("state_info_pull_req: Channel MAC:", hex.EncodeToString(sipr.Channel_MAC))
}

// ToString of StateInfo prints Timestamp and PKI-id
func (si *StateInfo) ToString() string {
	return fmt.Sprint("state_info_message: Timestamp:", si.Timestamp, "PKI-id:", hex.EncodeToString(si.PkiId),
		" channel MAC:", hex.EncodeToString(si.Channel_MAC), " properties:", si.Properties)
}

// formatDigests formats digest byte arrays into strings depending on the message type
func formatDigests(msgType PullMsgType, givenDigests [][]byte) []string {
	var digests []string
	switch msgType {
	case PullMsgType_BLOCK_MSG:
		for _, digest := range givenDigests {
			digests = append(digests, string(digest))
		}
	case PullMsgType_IDENTITY_MSG:
		for _, digest := range givenDigests {
			digests = append(digests, hex.EncodeToString(digest))
		}

	}
	return digests
}

// ToString of DataDigest prints nonce, msg_type and digests
func (dig *DataDigest) ToString() string {
	var digests []string
	digests = formatDigests(dig.MsgType, dig.Digests)
	return fmt.Sprintf("data_dig: nonce: %d , Msg_type: %s, digests: %v", dig.Nonce, dig.MsgType, digests)
}

// ToString of DataRequest prints nonce, msg_type and digests
func (dataReq *DataRequest) ToString() string {
	var digests []string
	digests = formatDigests(dataReq.MsgType, dataReq.Digests)
	return fmt.Sprintf("data request: nonce: %d , Msg_type: %s, digests: %v", dataReq.Nonce, dataReq.MsgType, digests)
}

// ToString of LeadershipMessage prints PKI-id, Timestamp and Is Declaration
func (lm *LeadershipMessage) ToString() string {
	return fmt.Sprint("Leadership Message: PKI-id:", hex.EncodeToString(lm.PkiId), " Timestamp:", lm.Timestamp,
		"Is Declaration ", lm.IsDeclaration)
}

// ToString returns a string representation of this RemotePvtDataResponse
func (res *RemotePvtDataResponse) ToString() string {
	a := make([]string, len(res.Elements))
	for i, el := range res.Elements {
		a[i] = fmt.Sprintf("%s with %d elements", el.Digest.String(), len(el.Payload))
	}
	return fmt.Sprintf("%v", a)
}
