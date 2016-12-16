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

package node

import (
	"fmt"
	"io/ioutil"

	cutil "github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

// join-related variables.
var (
	genesisBlockPath string
)

// JoinCmdFactory holds the clients used by JoinCmd
type JoinCmdFactory struct {
	EndorserClient pb.EndorserClient
	Signer         msp.SigningIdentity
}

// initCmdFactory init the ChaincodeCmdFactory with default clients
func initCmdFactory() (*JoinCmdFactory, error) {
	endorserClient, err := common.GetEndorserClient()
	if err != nil {
		return nil, fmt.Errorf("Error getting endorser client : %s", err)
	}

	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, fmt.Errorf("Error getting default signer: %s", err)
	}

	return &JoinCmdFactory{
		EndorserClient: endorserClient,
		Signer:         signer,
	}, nil
}

func joinCmd() *cobra.Command {
	// Set the flags on the node start command.
	flags := nodeJoinCmd.Flags()
	flags.BoolVarP(&chaincodeDevMode, "peer-chaincodedev", "", false,
		"Whether peer in chaincode development mode")
	flags.StringVarP(&genesisBlockPath, "path", "b", common.UndefinedParamValue,
		"Path to file containing genesis block")

	return nodeJoinCmd
}

var nodeJoinCmd = &cobra.Command{
	Use:   "join",
	Short: "Joins the peer to a chain.",
	Long:  `Joins the peer to a chain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return join(args)
	},
}

func getJoinCCSpec() (*pb.ChaincodeSpec, error) {
	if genesisBlockPath == common.UndefinedParamValue {
		return nil, fmt.Errorf("Must supply genesis block file.\n")
	}

	gb, err := ioutil.ReadFile(genesisBlockPath)
	if err != nil {
		return nil, err
	}

	spec := &pb.ChaincodeSpec{}

	// Build the spec
	input := &pb.ChaincodeInput{Args: [][]byte{[]byte("JoinChain"), gb}}

	spec = &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]),
		ChaincodeID: &pb.ChaincodeID{Name: "cscc"},
		CtorMsg:     input,
	}

	return spec, nil
}

func executeJoin(cf *JoinCmdFactory) (err error) {
	spec, err := getJoinCCSpec()
	if err != nil {
		return err
	}

	// Build the ChaincodeInvocationSpec message
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("Error serializing identity for %s: %s\n", cf.Signer.GetIdentifier(), err)
	}

	uuid := cutil.GenerateUUID()

	var prop *pb.Proposal
	prop, err = putils.CreateProposalFromCIS(uuid, "", invocation, creator)
	if err != nil {
		return fmt.Errorf("Error creating proposal for join %s\n", err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = putils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return fmt.Errorf("Error creating signed proposal  %s\n", err)
	}

	var proposalResp *pb.ProposalResponse
	proposalResp, err = cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return fmt.Errorf("Error endorsing %s\n", err)
	}

	if proposalResp == nil {
		return fmt.Errorf("Error on join by endorsing: %s\n", err)
	}

	fmt.Printf("Join Result: %s\n", string(proposalResp.Response.Payload))

	return nil
}

func join(args []string) error {
	// Parameter overrides must be processed before any paramaters are
	// cached. Failures to cache cause the server to terminate immediately.
	if chaincodeDevMode {
		errStr := fmt.Sprintf("Chaincode development mode not implemented for join")
		logger.Info(errStr)
		return fmt.Errorf(errStr)
	}

	cf, err := initCmdFactory()
	if err != nil {
		return err
	}

	return executeJoin(cf)
}
