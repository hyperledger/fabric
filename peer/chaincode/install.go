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

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/peer/common"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var chaincodeInstallCmd *cobra.Command

const install_cmdname = "install"

// installCmd returns the cobra command for Chaincode Deploy
func installCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeInstallCmd = &cobra.Command{
		Use:       "install",
		Short:     fmt.Sprintf("Package the specified chaincode into a deployment spec and save it on the peer's path."),
		Long:      fmt.Sprintf(`Package the specified chaincode into a deployment spec and save it on the peer's path.`),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return chaincodeInstall(cmd, args, cf)
		},
	}

	return chaincodeInstallCmd
}

// chaincodeInstall deploys the chaincode. On success, the chaincode name
// (hash) is printed to STDOUT for use by subsequent chaincode-related CLI
// commands.
func chaincodeInstall(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory) error {
	if chaincodePath == common.UndefinedParamValue || chaincodeVersion == common.UndefinedParamValue {
		return fmt.Errorf("Must supply value for %s path and version parameters.\n", chainFuncName)
	}

	peerPath := viper.GetString("peer.fileSystemPath")
	if peerPath == "" {
		return fmt.Errorf("Peer's environment \"peer.fileSystemPath\" is not set")
	}

	ccprovider.SetChaincodesPath(peerPath + "/chaincodes")

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

	if err = ccprovider.PutChaincodeIntoFS(cds); err != nil {
		return fmt.Errorf("Error installing chaincode code %s:%s(%s)", chaincodeName, chaincodeVersion, err)
	}

	logger.Debugf("Installed chaincode %s:%s", chaincodeName, chaincodeVersion)

	return err
}
