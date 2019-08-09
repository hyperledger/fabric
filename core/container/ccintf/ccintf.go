/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccintf

//This package defines the interfaces that support runtime and
//communication between chaincode and peer (chaincode support).
//Currently inproccontroller uses it. dockercontroller does not.

import (
	pb "github.com/hyperledger/fabric/protos/peer"
)

// ChaincodeStream interface for stream between Peer and chaincode instance.
type ChaincodeStream interface {
	Send(*pb.ChaincodeMessage) error
	Recv() (*pb.ChaincodeMessage, error)
}

// CCID encapsulates chaincode ID
type CCID string

// String returns a string version of the chaincode ID
func (c CCID) String() string {
	return string(c)
}

// New returns a chaincode ID given the supplied package ID
func New(packageID string) CCID {
	return CCID(packageID)
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
