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

	protcommon "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

var chaincodeUpgradeCmd *cobra.Command

const upgradeCmdName = "upgrade"

// upgradeCmd returns the cobra command for Chaincode Upgrade
func upgradeCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeUpgradeCmd = &cobra.Command{
		Use:       upgradeCmdName,
		Short:     "Upgrade chaincode.",
		Long:      "Upgrade an existing chaincode with the specified one. The new chaincode will immediately replace the existing chaincode upon the transaction committed.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cf1 := cf
			return chaincodeUpgrade(cf1, func() error {
				var err error
				if cf1 == nil {
					cf1, err = InitCmdFactory(true, true)
					if err != nil {
						return err
					}
				}
				env, err := upgrade(cmd, cf1)
				if err != nil {
					return err
				}

				if env != nil {
					logger.Debug("Send signed envelope to orderer")
					err = cf1.BroadcastClient.Send(env)
					return err
				}
				return nil
			})
		},
	}
	flagList := []string{
		"lang",
		"ctor",
		"path",
		"name",
		"channelID",
		"version",
		"policy",
		"escc",
		"vscc",
		"resourceEnvelopeSavePath",
		"resourceEnvelopeLoadPath",
	}
	attachFlags(chaincodeUpgradeCmd, flagList)

	return chaincodeUpgradeCmd
}

//upgrade the command via Endorser
func upgrade(cmd *cobra.Command, cf *ChaincodeCmdFactory) (*protcommon.Envelope, error) {
	if channelID == "" {
		return nil, errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	spec, err := getChaincodeSpec(cmd)
	if err != nil {
		return nil, err
	}

	cds, err := getChaincodeDeploymentSpec(spec, false)
	if err != nil {
		return nil, fmt.Errorf("Error getting chaincode code %s: %s", chainFuncName, err)
	}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Error serializing identity for %s: %s", cf.Signer.GetIdentifier(), err)
	}

	prop, _, err := utils.CreateUpgradeProposalFromCDS(channelID, cds, creator, policyMarshalled, []byte(escc), []byte(vscc))
	if err != nil {
		return nil, fmt.Errorf("Error creating proposal %s: %s", chainFuncName, err)
	}
	logger.Debugf("Get upgrade proposal for chaincode <%v>", spec.ChaincodeId)

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return nil, fmt.Errorf("Error creating signed proposal  %s: %s", chainFuncName, err)
	}

	proposalResponse, err := cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("Error endorsing %s: %s", chainFuncName, err)
	}
	logger.Debugf("endorse upgrade proposal, get response <%v>", proposalResponse.Response)

	if proposalResponse != nil {
		// assemble a signed transaction (it's an Envelope message)
		env, err := utils.CreateSignedTx(prop, cf.Signer, proposalResponse)
		if err != nil {
			return nil, fmt.Errorf("Could not assemble transaction, err %s", err)
		}
		logger.Debug("Get Signed envelope")
		return env, nil
	}

	return nil, nil
}

// chaincodeUpgrade upgrades the chaincode. On success, the new chaincode
// version is printed to STDOUT
func chaincodeUpgrade(cf *ChaincodeCmdFactory, sendInit sendInitTransaction) error {
	if channelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	var err error
	defer cf.BroadcastClient.Close()

	ss := &sigSupport{cf.Signer}
	version, config, err := fetchResourceConfig(cf.EndorserClient, ss, channelID)
	if err != nil {
		return errors.Wrap(err, "failed probing channel version")
	}
	if version == v11 {
		return configBasedLifecycleUpdate(ss, cf, config, sendInit)
	}
	return sendInit()
}
