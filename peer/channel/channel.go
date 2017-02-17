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
	chainID        string
	anchorPeerList string
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
	channelCmd.AddCommand(fetchCmd(cf))

	return channelCmd
}

// AddFlags adds flags for create and join
func AddFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&genesisBlockPath, "blockpath", "b", common.UndefinedParamValue, "Path to file containing genesis block")
	flags.StringVarP(&chainID, "chain", "c", "mychain", "In case of a newChain command, the chain ID to create.")
	flags.StringVarP(&anchorPeerList, "anchors", "a", "", anchorPeerUsage)
}

var channelCmd = &cobra.Command{
	Use:   channelFuncName,
	Short: fmt.Sprintf("%s specific commands.", channelFuncName),
	Long:  fmt.Sprintf("%s specific commands.", channelFuncName),
}

// ChannelCmdFactory holds the clients used by ChannelCmdFactory
type ChannelCmdFactory struct {
	EndorserClient   pb.EndorserClient
	Signer           msp.SigningIdentity
	BroadcastClient  common.BroadcastClient
	DeliverClient    deliverClientIntf
	AnchorPeerParser *common.AnchorPeerParser
}

// InitCmdFactory init the ChannelCmdFactor with default clients
func InitCmdFactory(isOrdererRequired bool) (*ChannelCmdFactory, error) {
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
	if isOrdererRequired {
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
		cmdFact.AnchorPeerParser = common.GetAnchorPeersParser(anchorPeerList)
	}

	return cmdFact, nil
}

const anchorPeerUsage = `In case of a newChain command, the list of anchor peer files, separated by commas.
	The files should be in the following format:
	anchorPeerHost
	anchorPeerPort
	PEM encoded certificate.

	In example:
	1.2.3.4
	7051
	-----BEGIN CERTIFICATE-----
	MIIDXTCCAkWgAwIBAgIJALRf63iSHa0BMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
	BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
	aWRnaXRzIFB0eSBMdGQwHhcNMTcwMTI2MjMyMzM1WhcNMTgwMTI2MjMyMzM1WjBF
	MQswCQYDVQQGEwJBVTETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
	ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
	CgKCAQEAzbph0SEHYb/tvNYATWfpl7oAFpw3Tcn2s0icJaScqs2RodjosIOBK6AB
	N6fkgGDHwYhYbMNfJzUYSYgXD4MPjDxzPw+/Hz02bjuxFB8pQnmln6b6pVHz79vL
	i3UQ8eaCe3zswpX0JJTlOs5wdJGOySNRNatbVKl9HDNWcNl6Ec5MrlK3/v6OGF03
	0ak7QYDNjyHaz3rMaOzJumRJeOxtjUO/+TbjN+bkcXSgQH9LjoeaZdkV/QWrCA1I
	qGowBOxYcyiX56bKKFvCZ76ZYA55d3HyI/H7S258CTdE6WUTDXNqmXnX5WbBuUiK
	dypI+KmGlzrRETahrJSJKdlxxtpPVwIDAQABo1AwTjAdBgNVHQ4EFgQUnK6ITmnz
	hfNKFr+57Bcayzio47EwHwYDVR0jBBgwFoAUnK6ITmnzhfNKFr+57Bcayzio47Ew
	DAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAvYFu4xQDE11C8wdK/5LE
	G61E9yjsDjFlhzgsG8+TqWI6LjHzm3hSNj7VMI7f0ckydxxOSQqKEkkQaL5GNS3B
	JOwsGtPjgQ2Sxx2KrEyaNozxznm1qZflQCis95NVvjHeiybbLfjQRVKde0+7kSKc
	cqBBE+IwxNofNyevlRyCBNsH6v2DLJoiFwvE5PqY6XvAcC17va/TKS16TVCqpxX0
	OrngleEKom1hiU1MzGZ29/nGpwP/oD8Lf+BqxipLf3BdiDR2+n5dbrV/ul1VczwQ
	F2ht++pZbdiqmv7CRAfvkSzrkwIeL+XfVR6ncFf4Nf92u6DJDnTzc/0K3pLaE+bo
	JQ==
	-----END CERTIFICATE-----
`
