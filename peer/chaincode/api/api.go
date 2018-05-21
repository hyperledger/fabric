/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	pcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

//go:generate counterfeiter -o ../mock/deliver.go -fake-name Deliver . Deliver

// Deliver defines the interface for delivering blocks
type Deliver interface {
	Send(*pcommon.Envelope) error
	Recv() (*pb.DeliverResponse, error)
	CloseSend() error
}
