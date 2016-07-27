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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/peer/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

// chaincodeDeploy deploys the chaincode. On success, the chaincode name
// (hash) is printed to STDOUT for use by subsequent chaincode-related CLI
// commands.
func chaincodeDeploy(cmd *cobra.Command, args []string) error {
	if err := checkChaincodeCmdParams(cmd); err != nil {
		return err
	}
	devopsClient, err := common.GetDevopsClient(cmd)
	if err != nil {
		return fmt.Errorf("Error building %s: %s", chainFuncName, err)
	}
	// Build the spec
	input := &pb.ChaincodeInput{}
	if err := json.Unmarshal([]byte(chaincodeCtorJSON), &input); err != nil {
		return fmt.Errorf("Chaincode argument error: %s", err)
	}

	var attributes []string
	if err := json.Unmarshal([]byte(chaincodeAttributesJSON), &attributes); err != nil {
		return fmt.Errorf("Chaincode argument error: %s", err)
	}

	chaincodeLang = strings.ToUpper(chaincodeLang)
	spec := &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[chaincodeLang]),
		ChaincodeID: &pb.ChaincodeID{Path: chaincodePath, Name: chaincodeName},
		CtorMsg:     input,
		Attributes:  attributes,
	}

	// If security is enabled, add client login token
	if core.SecurityEnabled() {
		logger.Debug("Security is enabled. Include security context in deploy spec")
		if chaincodeUsr == common.UndefinedParamValue {
			return errors.New("Must supply username for chaincode when security is enabled")
		}

		// Retrieve the CLI data storage path
		// Returns /var/openchain/production/client/
		localStore := util.GetCliFilePath()

		// Check if the user is logged in before sending transaction
		if _, err = os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			logger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				panic(fmt.Errorf("Fatal error when reading client login token: %s\n", err))
			}

			// Add the login token to the chaincodeSpec
			spec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				logger.Info("Set confidentiality level to CONFIDENTIAL.\n")
				spec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				return fmt.Errorf("User '%s' not logged in. Use the 'login' command to obtain a security token.", chaincodeUsr)
			}
			// Unexpected error
			panic(fmt.Errorf("Fatal error when checking for client login token: %s\n", err))
		}
	} else {
		if chaincodeUsr != common.UndefinedParamValue {
			logger.Warning("Username supplied but security is disabled.")
		}
		if viper.GetBool("security.privacy") {
			panic(errors.New("Privacy cannot be enabled as requested because security is disabled"))
		}
	}

	chaincodeDeploymentSpec, err := devopsClient.Deploy(context.Background(), spec)
	if err != nil {
		return fmt.Errorf("Error building %s: %s\n", chainFuncName, err)
	}
	logger.Infof("Deploy result: %s", chaincodeDeploymentSpec.ChaincodeSpec)

	return nil
}
