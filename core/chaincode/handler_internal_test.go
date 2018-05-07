/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Helpers to access unexported state.

func SetHandlerChaincodeID(h *Handler, chaincodeID *pb.ChaincodeID) {
	h.chaincodeID = chaincodeID
}

func SetHandlerChatStream(h *Handler, chatStream ccintf.ChaincodeStream) {
	h.chatStream = chatStream
}

func SetHandlerCCInstance(h *Handler, ccInstance *sysccprovider.ChaincodeInstance) {
	h.ccInstance = ccInstance
}
