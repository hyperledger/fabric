/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"fmt"
	"strings"
	"time"

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type BroadcastClient interface {
	//Send data to orderer
	Send(env *cb.Envelope) error
	Close() error
}

type broadcastClient struct {
	conn   *grpc.ClientConn
	client ab.AtomicBroadcast_BroadcastClient
}

// GetBroadcastClient creates a simple instance of the BroadcastClient interface
func GetBroadcastClient(orderingEndpoint string, tlsEnabled bool, caFile string) (BroadcastClient, error) {

	if len(strings.Split(orderingEndpoint, ":")) != 2 {
		return nil, errors.Errorf("ordering service endpoint %s is not valid or missing", orderingEndpoint)
	}

	var opts []grpc.DialOption
	// check for TLS
	if tlsEnabled {
		if caFile != "" {
			creds, err := credentials.NewClientTLSFromFile(caFile, "")
			if err != nil {
				return nil, errors.WithMessage(err, fmt.Sprintf("error connecting to %s due to", orderingEndpoint))
			}
			opts = append(opts, grpc.WithTransportCredentials(creds))
		}
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	opts = append(opts, grpc.WithBlock())
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, 3*time.Second)
	conn, err := grpc.DialContext(ctx, orderingEndpoint, opts...)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error connecting to %s due to", orderingEndpoint))
	}
	client, err := ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
	if err != nil {
		conn.Close()
		return nil, errors.WithMessage(err, fmt.Sprintf("error connecting to %s due to", orderingEndpoint))
	}

	return &broadcastClient{conn: conn, client: client}, nil
}

func (s *broadcastClient) getAck() error {
	msg, err := s.client.Recv()
	if err != nil {
		return err
	}
	if msg.Status != cb.Status_SUCCESS {
		return errors.Errorf("got unexpected status: %v -- %s", msg.Status, msg.Info)
	}
	return nil
}

//Send data to orderer
func (s *broadcastClient) Send(env *cb.Envelope) error {
	if err := s.client.Send(env); err != nil {
		return errors.WithMessage(err, "could not send")
	}

	err := s.getAck()

	return err
}

func (s *broadcastClient) Close() error {
	return s.conn.Close()
}
