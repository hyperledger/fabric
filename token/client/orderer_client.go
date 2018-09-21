/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

//go:generate counterfeiter -o mock/broadcast.go -fake-name Broadcast . Broadcast

// Broadcast defines the interface that abstracts grpc calls to broadcast transactions to orderer
type Broadcast interface {
	Send(m *common.Envelope) error
	Recv() (*ab.BroadcastResponse, error)
	CloseSend() error
}

//go:generate counterfeiter -o mock/orderer_client.go -fake-name OrdererClient . OrdererClient

// OrdererClient defines the interface to create a Broadcast
type OrdererClient interface {
	// NewBroadcast returns a Broadcast
	NewBroadcast(ctx context.Context, opts ...grpc.CallOption) (Broadcast, error)

	// Certificate returns tls certificate for the orderer client
	Certificate() *tls.Certificate
}

// ordererClient implements OrdererClient interface
type ordererClient struct {
	ordererAddr        string
	serverNameOverride string
	grpcClient         *comm.GRPCClient
	conn               *grpc.ClientConn
}

func NewOrdererClient(config *ClientConfig) (OrdererClient, error) {
	grpcClient, err := createGrpcClient(&config.OrdererCfg, config.TlsEnabled)
	if err != nil {
		err = errors.WithMessage(err, fmt.Sprintf("failed to create a GRPCClient to orderer %s", config.OrdererCfg.Address))
		logger.Errorf("%s", err)
		return nil, err
	}
	conn, err := grpcClient.NewConnection(config.OrdererCfg.Address, config.OrdererCfg.ServerNameOverride)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("failed to connect to orderer %s", config.OrdererCfg.Address))
	}

	return &ordererClient{
		ordererAddr:        config.OrdererCfg.Address,
		serverNameOverride: config.OrdererCfg.ServerNameOverride,
		grpcClient:         grpcClient,
		conn:               conn,
	}, nil
}

// NewBroadcast creates a Broadcast
func (oc *ordererClient) NewBroadcast(ctx context.Context, opts ...grpc.CallOption) (Broadcast, error) {
	// reuse the existing connection to create Broadcast client
	broadcast, err := ab.NewAtomicBroadcastClient(oc.conn).Broadcast(ctx)
	if err == nil {
		return broadcast, nil
	}

	// error occurred with the existing connection, so create a new connection to orderer
	oc.conn, err = oc.grpcClient.NewConnection(oc.ordererAddr, oc.serverNameOverride)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("failed to connect to orderer %s", oc.ordererAddr))
	}

	// create a new Broadcast
	broadcast, err = ab.NewAtomicBroadcastClient(oc.conn).Broadcast(ctx)
	if err != nil {
		rpcStatus, _ := status.FromError(err)
		return nil, errors.Wrapf(err, "failed to new a broadcast, rpcStatus=%+v", rpcStatus)
	}
	return broadcast, nil
}

func (oc *ordererClient) Certificate() *tls.Certificate {
	cert := oc.grpcClient.Certificate()
	return &cert
}

// broadcastSend sends transaction envelope to orderer service
func BroadcastSend(broadcast Broadcast, addr string, envelope *common.Envelope) error {
	err := broadcast.Send(envelope)
	broadcast.CloseSend()

	if err != nil {
		return errors.Wrapf(err, "failed to send transaction to orderer %s", addr)
	}
	return nil
}

// broadReceive waits until it receives the response from broadcast stream
func BroadcastReceive(broadcast Broadcast, addr string, responses chan common.Status, errs chan error) {
	logger.Infof("calling OrdererClient.broadcastReceive")
	for {
		broadcastResponse, err := broadcast.Recv()
		if err == io.EOF {
			close(responses)
			return
		}

		if err != nil {
			rpcStatus, _ := status.FromError(err)
			errs <- errors.Wrapf(err, "broadcast recv error from orderer %s, rpcStatus=%+v", addr, rpcStatus)
			close(responses)
			return
		}

		if broadcastResponse.Status == common.Status_SUCCESS {
			responses <- broadcastResponse.Status
		} else {
			errs <- errors.Errorf("broadcast response error %d from orderer %s", int32(broadcastResponse.Status), addr)
		}
	}
}

// broadcastWaitForResponse reads from response and errs chans until responses chan is closed
func BroadcastWaitForResponse(responses chan common.Status, errs chan error) (common.Status, error) {
	var status common.Status
	allErrs := make([]error, 0)

read:
	for {
		select {
		case s, ok := <-responses:
			if !ok {
				break read
			}
			status = s
		case e := <-errs:
			allErrs = append(allErrs, e)
		}
	}

	// drain remaining errors
	for i := 0; i < len(errs); i++ {
		e := <-errs
		allErrs = append(allErrs, e)
	}
	// close errs channel since we have read all of them
	close(errs)
	return status, toError(allErrs)
}

// toError converts []error to error
func toError(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errmsgs := []string{fmt.Sprint("Multiple errors occurred in order broadcast stream: ")}
	for _, err := range errs {
		errmsgs = append(errmsgs, err.Error())
	}
	return errors.New(strings.Join(errmsgs, "\n"))
}
