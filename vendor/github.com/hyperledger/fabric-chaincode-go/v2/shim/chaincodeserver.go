// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package shim

import (
	"crypto/tls"
	"errors"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim/internal"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"google.golang.org/grpc/keepalive"
)

// TLSProperties passed to ChaincodeServer
type TLSProperties struct {
	//Disabled forces default to be TLS enabled
	Disabled bool
	Key      []byte
	Cert     []byte
	// ClientCACerts set if connecting peer should be verified
	ClientCACerts []byte
}

// ChaincodeServer encapsulates basic properties needed for a chaincode server
type ChaincodeServer struct {
	// CCID should match chaincode's package name on peer
	CCID string
	// Addesss is the listen address of the chaincode server
	Address string
	// CC is the chaincode that handles Init and Invoke
	CC Chaincode
	// TLSProps is the TLS properties passed to chaincode server
	TLSProps TLSProperties
	// KaOpts keepalive options, sensible defaults provided if nil
	KaOpts *keepalive.ServerParameters
}

// Connect the bidi stream entry point called by chaincode to register with the Peer.
func (cs *ChaincodeServer) Connect(stream peer.Chaincode_ConnectServer) error {
	return chatWithPeer(cs.CCID, stream, cs.CC)
}

// Start the server
func (cs *ChaincodeServer) Start() error {
	if cs.CCID == "" {
		return errors.New("ccid must be specified")
	}

	if cs.Address == "" {
		return errors.New("address must be specified")
	}

	if cs.CC == nil {
		return errors.New("chaincode must be specified")
	}

	var tlsCfg *tls.Config
	var err error
	if !cs.TLSProps.Disabled {
		tlsCfg, err = internal.LoadTLSConfig(true, cs.TLSProps.Key, cs.TLSProps.Cert, cs.TLSProps.ClientCACerts)
		if err != nil {
			return err
		}
	}

	// create listener and grpc server
	server, err := internal.NewServer(cs.Address, tlsCfg, cs.KaOpts)
	if err != nil {
		return err
	}

	// register the server with grpc ...
	peer.RegisterChaincodeServer(server.Server, cs)

	// ... and start
	return server.Start()
}
