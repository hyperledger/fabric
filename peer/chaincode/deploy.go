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

	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	protcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
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
func deploy(cmd *cobra.Command) (*protcommon.Envelope, error) {
	spec, err := getChaincodeSpecification(cmd)
	if err != nil {
		return nil, err
	}

	cds, err := getChaincodeBytes(spec)
	if err != nil {
		return nil, fmt.Errorf("Error getting chaincode code %s: %s", chainFuncName, err)
	}

	endorserClient, err := common.GetEndorserClient(cmd)
	if err != nil {
		return nil, fmt.Errorf("Error getting endorser client %s: %s", chainFuncName, err)
	}

	// TODO: how should we get signing ID from the command line?
	mspID := "DEFAULT"
	id := "PEER"
	signingIdentity := &msp.IdentityIdentifier{Mspid: msp.ProviderIdentifier{Value: mspID}, Value: id}

	// TODO: how should we obtain the config for the MSP from the command line? a hardcoded test config?
	signer, err := msp.GetManager().GetSigningIdentity(signingIdentity)
	if err != nil {
		return nil, fmt.Errorf("Error obtaining signing identity for %s: %s\n", signingIdentity, err)
	}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Error serializing identity for %s: %s\n", signingIdentity, err)
	}

	uuid := util.GenerateUUID()

	prop, err := utils.CreateProposalFromCDS(uuid, chainID, cds, creator)
	if err != nil {
		return nil, fmt.Errorf("Error creating proposal  %s: %s\n", chainFuncName, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, fmt.Errorf("Error creating signed proposal  %s: %s\n", chainFuncName, err)
	}

	proposalResponse, err := endorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("Error endorsing %s: %s\n", chainFuncName, err)
	}

	if proposalResponse != nil {
		// assemble a signed transaction (it's an Envelope message)
		env, err := utils.CreateSignedTx(prop, signer, proposalResponse)
		if err != nil {
			return nil, fmt.Errorf("Could not assemble transaction, err %s", err)
		}

		return env, nil
	}

	return nil, nil
}

// chaincodeDeploy deploys the chaincode. On success, the chaincode name
// (hash) is printed to STDOUT for use by subsequent chaincode-related CLI
// commands.
func chaincodeDeploy(cmd *cobra.Command, args []string) error {
	env, err := deploy(cmd)
	if err != nil {
		return err
	}

	if env != nil {
		err = sendTransaction(env)
	}

	return err
}
