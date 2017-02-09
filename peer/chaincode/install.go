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
	"io/ioutil"
	"os"

	"github.com/hyperledger/fabric/peer/common"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var chaincodeInstallCmd *cobra.Command

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

func createCCInstallPath(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	chaincodePath := path + "/chaincodes"
	if s, err := os.Stat(chaincodePath); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(chaincodePath, 0755); err != nil {
				return "", err
			}
			return chaincodePath, nil
		}
		return "", err
	} else if !s.IsDir() {
		return "", fmt.Errorf("chaincode path exists but not a dir: %s", chaincodePath)
	}

	return chaincodePath, nil
}

func packageCC(chaincodeBin []byte) ([]byte, error) {
	//TODO create proper, secured package, for now return chaincode binary asis
	return chaincodeBin, nil
}

func installCC(path string, bin []byte) error {
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("chaincode %s exists", path)
	}

	if err := ioutil.WriteFile(path, bin, 0644); err != nil {
		logger.Errorf("Failed writing deployment spec to file [%s]: [%s]", path, err)
		return err
	}

	return nil
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

	var ccpath string
	var err error

	//create the chaincodes dir if necessary
	if ccpath, err = createCCInstallPath(peerPath); err != nil {
		return err
	}

	//check if chaincode already exists
	fileToWrite := ccpath + "/" + chaincodeName + "." + chaincodeVersion
	if _, err := os.Stat(fileToWrite); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("chaincode %s exists", fileToWrite)
	}

	spec, err := getChaincodeSpecification(cmd)
	if err != nil {
		return err
	}

	cds, err := getChaincodeBytes(spec)
	if err != nil {
		return fmt.Errorf("Error getting chaincode code %s: %s", chainFuncName, err)
	}

	cdsBytes, err := proto.Marshal(cds)
	if err != nil {
		return fmt.Errorf("Error marshalling chaincode deployment spec : %s", err)
	}

	//TODO - packageCC is just a stub. It needs to be filled out with other items
	//such as serialized policy and has to be signed.
	pkgBytes, err := packageCC(cdsBytes)
	if err != nil {
		logger.Errorf("Failed creating package [%s]", err)
		return err
	}

	err = installCC(fileToWrite, pkgBytes)
	if err != nil {
		logger.Errorf("Failed writing deployment spec to file [%s]: [%s]", fileToWrite, err)
		return err
	}

	logger.Debugf("Installed chaincode (%s,%s) of size <%d>", chaincodeName, chaincodeVersion, len(cdsBytes))
	return err
}
