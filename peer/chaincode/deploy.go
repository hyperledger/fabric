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

package chaincode

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/cobra"
)

// Cmd returns the cobra command for Chaincode Deploy
func deployCmd() *cobra.Command {
	return chaincodeDeployCmd
}

var chaincodeDeployCmd = &cobra.Command{
	Use:       "deploy",
	Short:     fmt.Sprintf("Deploy the specified chaincode to the network."),
	Long:      fmt.Sprintf(`Deploy the specified chaincode to the network.`),
	ValidArgs: []string{"1"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return chaincodeDeploy(cmd, args)
	},
}

//deploy the command via Endorser
func deploy(cmd *cobra.Command) (*pb.ProposalResponse, error) {
	spec, err := getChaincodeSpecification(cmd)
	if err != nil {
		return nil, err
	}

	ctxt := context.Background()

	cds, err := core.GetChaincodeBytes(ctxt, spec)
	if err != nil {
		return nil, fmt.Errorf("Error getting chaincode code %s: %s", chainFuncName, err)
	}

	endorserClient, err := common.GetEndorserClient(cmd)
	if err != nil {
		return nil, fmt.Errorf("Error getting endorser client %s: %s", chainFuncName, err)
	}

	prop, err := getDeployProposal(cds)
	if err != nil {
		return nil, fmt.Errorf("Error creating proposal  %s: %s\n", chainFuncName, err)
	}

	proposalResponse, err := endorserClient.ProcessProposal(ctxt, prop)
	if err != nil {
		return nil, fmt.Errorf("Error endorsing %s: %s\n", chainFuncName, err)
	}

	logger.Infof("Deploy(endorser) result: %v", proposalResponse)
	return proposalResponse, nil
}

// chaincodeDeploy deploys the chaincode. On success, the chaincode name
// (hash) is printed to STDOUT for use by subsequent chaincode-related CLI
// commands.
func chaincodeDeploy(cmd *cobra.Command, args []string) error {
	presult, err := deploy(cmd)
	if err != nil {
		return err
	}

	if presult != nil {
		err = sendTransaction(presult)
	}

	return err
}
