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

package chaincode

import (
	"fmt"

	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
)

// deployCmd returns the cobra command for Chaincode Deploy
func packageCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeDeployCmd = &cobra.Command{
		Use:       "package",
		Short:     fmt.Sprintf("Package the specified chaincode into a deployment spec."),
		Long:      fmt.Sprintf(`Package the specified chaincode into a deployment spec.`),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return chaincodePackage(cmd, args, cf)
		},
	}

	return chaincodeDeployCmd
}

// chaincodeDeploy deploys the chaincode. On success, the chaincode name
// (hash) is printed to STDOUT for use by subsequent chaincode-related CLI
// commands.
func chaincodePackage(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory) error {
	var err error
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
	logger.Debugf("Packaged chaincode into deployment spec of size <%d>, with args = %v", len(cdsBytes), args)
	fileToWrite := args[0]
	err = ioutil.WriteFile(fileToWrite, cdsBytes, 0700)
	if err != nil {
		logger.Errorf("Failed writing deployment spec to file [%s]: [%s]", fileToWrite, err)
		return err
	}

	return err
}
