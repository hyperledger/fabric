/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

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

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/spf13/cobra"
)

var chaincodeInstallCmd *cobra.Command

const install_cmdname = "install"

const install_desc = "Package the specified chaincode into a deployment spec and save it on the peer's path."

// installCmd returns the cobra command for Chaincode Deploy
func installCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeInstallCmd = &cobra.Command{
		Use:       "install",
		Short:     fmt.Sprintf(install_desc),
		Long:      fmt.Sprintf(install_desc),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return chaincodeInstall(cmd, args, cf)
		},
	}

	return chaincodeInstallCmd
}

//install the depspec to "peer.address"
func install(chaincodeName string, chaincodeVersion string, cds *pb.ChaincodeDeploymentSpec, cf *ChaincodeCmdFactory) error {
	creator, err := cf.Signer.Serialize()
	if err != nil {
		return fmt.Errorf("Error serializing identity for %s: %s", cf.Signer.GetIdentifier(), err)
	}

	prop, _, err := utils.CreateInstallProposalFromCDS(cds, creator)
	if err != nil {
		return fmt.Errorf("Error creating proposal  %s: %s", chainFuncName, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = utils.GetSignedProposal(prop, cf.Signer)
	if err != nil {
		return fmt.Errorf("Error creating signed proposal  %s: %s", chainFuncName, err)
	}

	proposalResponse, err := cf.EndorserClient.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return fmt.Errorf("Error endorsing %s: %s", chainFuncName, err)
	}

	if proposalResponse != nil {
		logger.Debug("Installed remotely %v", proposalResponse)
	}

	return nil
}

// chaincodeInstall installs the chaincode. If remoteinstall, does it via a lccc call
func chaincodeInstall(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory) error {
	if chaincodePath == common.UndefinedParamValue || chaincodeVersion == common.UndefinedParamValue {
		return fmt.Errorf("Must supply value for %s path and version parameters.", chainFuncName)
	}

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(false)
		if err != nil {
			return err
		}
	}

	tmppkg, _ := ccprovider.GetChaincodePackage(chaincodeName, chaincodeVersion)
	if tmppkg != nil {
		return fmt.Errorf("chaincode %s:%s exists", chaincodeName, chaincodeVersion)
	}

	spec, err := getChaincodeSpecification(cmd)
	if err != nil {
		return err
	}

	cds, err := getChaincodeBytes(spec, true)
	if err != nil {
		return fmt.Errorf("Error getting chaincode code %s: %s", chainFuncName, err)
	}

	err = install(chaincodeName, chaincodeVersion, cds, cf)

	return err
}
