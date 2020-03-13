/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"errors"
	"fmt"

	protcommon "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/cobra"
)

var chaincodeUpgradeCmd *cobra.Command

const upgradeCmdName = "upgrade"

// upgradeCmd returns the cobra command for Chaincode Upgrade
func upgradeCmd(cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeUpgradeCmd = &cobra.Command{
		Use:       upgradeCmdName,
		Short:     "Upgrade chaincode.",
		Long:      "Upgrade an existing chaincode with the specified one. The new chaincode will immediately replace the existing chaincode upon the transaction committed.",
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return chaincodeUpgrade(cmd, args, cf, cryptoProvider)
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
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"collections-config",
	}
	attachFlags(chaincodeUpgradeCmd, flagList)

	return chaincodeUpgradeCmd
}

//upgrade the command via Endorser
func upgrade(cmd *cobra.Command, cf *ChaincodeCmdFactory) (*protcommon.Envelope, error) {
	spec, err := getChaincodeSpec(cmd)
	if err != nil {
		return nil, err
	}

	cds, err := getChaincodeDeploymentSpec(spec, false)
	if err != nil {
		return nil, fmt.Errorf("error getting chaincode code %s: %s", chaincodeName, err)
	}

	creator, err := cf.Signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("error serializing identity: %s", err)
	}

	prop, _, err := protoutil.CreateUpgradeProposalFromCDS(channelID, cds, creator, policyMarshalled, []byte(escc), []byte(vscc), collectionConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("error creating proposal %s: %s", chainFuncName, err)
	}
	logger.Debugf("Get upgrade proposal for chaincode <%v>", spec.ChaincodeId)

	var signedProp *pb.SignedProposal
	signedProp, err = protoutil.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return nil, fmt.Errorf("error creating signed proposal  %s: %s", chainFuncName, err)
	}

	// upgrade is currently only supported for one peer
	proposalResponse, err := cf.EndorserClients[0].ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("error endorsing %s: %s", chainFuncName, err)
	}
	logger.Debugf("endorse upgrade proposal, get response <%v>", proposalResponse.Response)

	if proposalResponse != nil {
		// assemble a signed transaction (it's an Envelope message)
		env, err := protoutil.CreateSignedTx(prop, cf.Signer, proposalResponse)
		if err != nil {
			return nil, fmt.Errorf("could not assemble transaction, err %s", err)
		}
		logger.Debug("Get Signed envelope")
		return env, nil
	}

	return nil, nil
}

// chaincodeUpgrade upgrades the chaincode. On success, the new chaincode
// version is printed to STDOUT
func chaincodeUpgrade(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) error {
	if channelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), true, true, cryptoProvider)
		if err != nil {
			return err
		}
	}
	defer cf.BroadcastClient.Close()

	env, err := upgrade(cmd, cf)
	if err != nil {
		return err
	}

	if env != nil {
		logger.Debug("Send signed envelope to orderer")
		err = cf.BroadcastClient.Send(env)
		return err
	}

	return nil
}
