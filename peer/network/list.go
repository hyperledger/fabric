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

package network

import (
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

func listCmd() *cobra.Command {
	return networkListCmd
}

var networkListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Lists all network peers.",
	Long: "Returns a list of all existing network connections for the " +
		"target peer node, includes both validating and non-validating peers.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return networkList()
	},
}

// Show a list of all existing network connections for the target peer node,
// includes both validating and non-validating peers
func networkList() (err error) {
	clientConn, err := peer.NewPeerClientConnection()
	if err != nil {
		err = fmt.Errorf("Error trying to connect to local peer: %s", err)
		return
	}
	openchainClient := pb.NewOpenchainClient(clientConn)
	peers, err := openchainClient.GetPeers(context.Background(), &empty.Empty{})

	if err != nil {
		err = fmt.Errorf("Error trying to get peers: %s", err)
		return
	}

	// The generated pb.PeersMessage struct will be added "omitempty" tag automatically.
	// But we still want to print it when pb.PeersMessage is empty.
	jsonOutput, _ := json.Marshal(struct{ Peers []*pb.PeerEndpoint }{append([]*pb.PeerEndpoint{}, peers.GetPeers()...)})
	fmt.Println(string(jsonOutput))
	return nil
}
