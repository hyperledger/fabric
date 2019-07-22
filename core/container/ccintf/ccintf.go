/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccintf

//This package defines the interfaces that support runtime and
//communication between chaincode and peer (chaincode support).
//Currently inproccontroller uses it. dockercontroller does not.

import (
	persistence "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
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
func New(packageID persistence.PackageID) CCID {
	return CCID(packageID.String())
}

// TLSConfig is used to pass the TLS context into the chaincode launch
type TLSConfig struct {
	ClientCert []byte
	ClientKey  []byte
	RootCert   []byte
}
