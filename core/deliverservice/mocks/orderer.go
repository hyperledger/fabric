/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package mocks

import (
	"fmt"
	"net"

	"github.com/hyperledger/fabric/protos/orderer"
	"google.golang.org/grpc"
)

type Orderer struct {
	net.Listener
	*grpc.Server
}

func NewOrderer(port int) *Orderer {
	srv := grpc.NewServer()
	lsnr, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		panic(err)
	}
	go srv.Serve(lsnr)
	o := &Orderer{Server: srv, Listener: lsnr}
	orderer.RegisterAtomicBroadcastServer(srv, o)
	return o
}

func (o *Orderer) Shutdown() {
	o.Server.Stop()
	o.Listener.Close()
}

func (*Orderer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("Should not have ben called")
}

func (*Orderer) Deliver(orderer.AtomicBroadcast_DeliverServer) error {
	return nil
}
