/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/orderer"
)

//go:generate mockery --dir . --name Communicator --case underscore --output ./mocks/

// Communicator defines communication for a consenter
type Communicator interface {
	// Remote returns a RemoteContext for the given RemoteNode ID in the context
	// of the given channel, or error if connection cannot be established, or
	// the channel wasn't configured
	Remote(channel string, id uint64) (*RemoteContext, error)
	// Configure configures the communication to connect to all
	// given members, and disconnect from any members not among the given
	// members.
	Configure(channel string, members []RemoteNode)
	// Shutdown shuts down the communicator
	Shutdown()
}

// ChannelExtractor extracts the channel of a given message,
// or returns an empty string if that's not possible
type ChannelExtractor interface {
	TargetChannel(message proto.Message) string
}

//go:generate mockery --dir . --name Handler --case underscore --output ./mocks/

// Handler handles Step() and Submit() requests and returns a corresponding response
type Handler interface {
	OnConsensus(channel string, sender uint64, req *orderer.ConsensusRequest) error
	OnSubmit(channel string, sender uint64, req *orderer.SubmitRequest) error
}

type NodeCerts struct {
	// ServerTLSCert is the DER encoded TLS server certificate of the node
	ServerTLSCert []byte
	// ClientTLSCert is the DER encoded TLS client certificate of the node
	ClientTLSCert []byte
	// PEM-encoded X509 certificate authority to verify server certificates
	ServerRootCA [][]byte
	// PEM-encoded X509 certificate
	Identity []byte
}

type NodeAddress struct {
	// ID is unique among all members, and cannot be 0.
	ID uint64
	// Endpoint is the endpoint of the node, denoted in %s:%d format
	Endpoint string
}

// RemoteNode represents a cluster member
type RemoteNode struct {
	NodeAddress
	NodeCerts
}

// String returns a string representation of this RemoteNode
func (rm RemoteNode) String() string {
	return fmt.Sprintf("ID: %d,\nEndpoint: %s,\nServerTLSCert:%s,\nClientTLSCert:%s, ServerRootCA: %s",
		rm.ID, rm.Endpoint, DERtoPEM(rm.ServerTLSCert), DERtoPEM(rm.ClientTLSCert), rm.ServerRootCA)
}

// go:generate mockery --dir . --name StepClientStream --case underscore --output ./mocks/
// StepClientStream
type StepClientStream interface {
	Send(request *orderer.StepRequest) error
	Recv() (*orderer.StepResponse, error)
	Auth() error
	Context() context.Context
}
