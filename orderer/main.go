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

package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/hyperledger/fabric/orderer/solo"

	"google.golang.org/grpc"
)

func main() {

	address := os.Getenv("ORDERER_LISTEN_ADDRESS")
	if address == "" {
		address = "127.0.0.1"
	}

	port := os.Getenv("ORDERER_LISTEN_PORT")
	if port == "" {
		port = "5005"
	}

	lis, err := net.Listen("tcp", address+":"+port)
	if err != nil {
		fmt.Println("Failed to listen:", err)
		return
	}

	grpcServer := grpc.NewServer()

	solo.New(100, 10, 10, 10*time.Second, grpcServer)
	grpcServer.Serve(lis)
}
