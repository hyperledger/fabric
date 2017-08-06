/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

import (
	"fmt"

	pb "github.com/hyperledger/fabric/protos/peer"
	"google.golang.org/grpc"
)

type interceptor struct {
	next pb.ChaincodeSupportServer
	auth authorization
}

// ChaincodeStream defines a gRPC stream for sending
// and receiving chaincode messages
type ChaincodeStream interface {
	// Send sends a chaincode message
	Send(*pb.ChaincodeMessage) error
	// Recv receives a chaincode message
	Recv() (*pb.ChaincodeMessage, error)
}

type authorization func(message *pb.ChaincodeMessage, stream grpc.ServerStream) error

func newInterceptor(srv pb.ChaincodeSupportServer, auth authorization) pb.ChaincodeSupportServer {
	return &interceptor{
		next: srv,
		auth: auth,
	}
}

// Register makes the interceptor implement ChaincodeSupportServer
func (i *interceptor) Register(stream pb.ChaincodeSupport_RegisterServer) error {
	is := &interceptedStream{
		incMessages:  make(chan *pb.ChaincodeMessage, 1),
		stream:       stream,
		ServerStream: stream,
		auth:         i.auth,
	}
	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("Recv() error: %v, closing connection", err)
	}
	err = is.auth(msg, is.ServerStream)
	if err != nil {
		return err
	}
	is.incMessages <- msg
	close(is.incMessages)
	return i.next.Register(is)
}

type interceptedStream struct {
	incMessages chan *pb.ChaincodeMessage
	stream      ChaincodeStream
	grpc.ServerStream
	auth authorization
}

// Send sends a chaincode message
func (is *interceptedStream) Send(msg *pb.ChaincodeMessage) error {
	return is.stream.Send(msg)
}

// Recv receives a chaincode message
func (is *interceptedStream) Recv() (*pb.ChaincodeMessage, error) {
	msg, ok := <-is.incMessages
	if !ok {
		return is.stream.Recv()
	}
	return msg, nil
}
