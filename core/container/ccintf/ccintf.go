/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccintf

import (
	"github.com/hyperledger/fabric/internal/pkg/comm"

	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// These structures can/should probably be moved out of here.

// ChaincodeStream interface for stream between Peer and chaincode instance.
type ChaincodeStream interface {
	Send(*pb.ChaincodeMessage) error
	Recv() (*pb.ChaincodeMessage, error)
}

// PeerConnection instructs the chaincode how to connect back to the peer
type PeerConnection struct {
	Address   string
	TLSConfig *TLSConfig
}

// TLSConfig is used to pass the TLS context into the chaincode launch
type TLSConfig struct {
	ClientCert []byte
	ClientKey  []byte
	RootCert   []byte
}

// ChaincodeServerInfo provides chaincode connection information
type ChaincodeServerInfo struct {
	Address      string
	ClientConfig comm.ClientConfig
}
