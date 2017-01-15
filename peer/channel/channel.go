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

package channel

import (
	"fmt"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const channelFuncName = "channel"

var logger = logging.MustGetLogger("channelCmd")

var (
	// join related variables.
	genesisBlockPath string

	// create related variables
	chainID string
)

// Cmd returns the cobra command for Node
func Cmd(cf *ChannelCmdFactory) *cobra.Command {
	//the "peer.committer.enabled" flag should really go away...
	//basically we need the orderer for create and join
	if !viper.GetBool("peer.committer.enabled") || viper.GetString("peer.committer.ledger.orderer") == "" {
		panic("orderer not provided")
	}

	AddFlags(channelCmd)
	channelCmd.AddCommand(joinCmd(cf))
	channelCmd.AddCommand(createCmd(cf))

	return channelCmd
}

//AddFlags adds flags for create and join
func AddFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&genesisBlockPath, "blockpath", "b", common.UndefinedParamValue, "Path to file containing genesis block")
	flags.StringVarP(&chainID, "chain", "c", "mychain", "In case of a newChain command, the chain ID to create.")
}

var channelCmd = &cobra.Command{
	Use:   channelFuncName,
	Short: fmt.Sprintf("%s specific commands.", channelFuncName),
	Long:  fmt.Sprintf("%s specific commands.", channelFuncName),
}

// ChannelCmdFactory holds the clients used by ChannelCmdFactory
type ChannelCmdFactory struct {
	EndorserClient  pb.EndorserClient
	Signer          msp.SigningIdentity
	BroadcastClient common.BroadcastClient
	DeliverClient   deliverClientIntf
}

// InitCmdFactory init the ChannelCmdFactor with default clients
func InitCmdFactory(forJoin bool) (*ChannelCmdFactory, error) {
	var err error

	cmdFact := &ChannelCmdFactory{}

	cmdFact.Signer, err = common.GetDefaultSigner()
	if err != nil {
		return nil, fmt.Errorf("Error getting default signer: %s", err)
	}

	cmdFact.BroadcastClient, err = common.GetBroadcastClient()
	if err != nil {
		return nil, fmt.Errorf("Error getting broadcast client: %s", err)
	}

	//for join, we need the endorser as well
	if forJoin {
		cmdFact.EndorserClient, err = common.GetEndorserClient()
		if err != nil {
			return nil, fmt.Errorf("Error getting endorser client %s: %s", channelFuncName, err)
		}
	} else {
		orderer := viper.GetString("peer.committer.ledger.orderer")
		conn, err := grpc.Dial(orderer, grpc.WithInsecure())
		if err != nil {
			return nil, err
		}

		client, err := ab.NewAtomicBroadcastClient(conn).Deliver(context.TODO())
		if err != nil {
			fmt.Println("Error connecting:", err)
			return nil, err
		}

		cmdFact.DeliverClient = newDeliverClient(client, chainID)
	}

	return cmdFact, nil
}
