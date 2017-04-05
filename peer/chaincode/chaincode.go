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

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/op/go-logging"
	"github.com/spf13/cobra"
)

const (
	chainFuncName = "chaincode"
)

var logger = logging.MustGetLogger("chaincodeCmd")

func AddFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&chaincodeLang, "lang", "l", "golang",
		fmt.Sprintf("Language the %s is written in", chainFuncName))
	flags.StringVarP(&chaincodeCtorJSON, "ctor", "c", "{}",
		fmt.Sprintf("Constructor message for the %s in JSON format", chainFuncName))
	flags.StringVarP(&chaincodePath, "path", "p", common.UndefinedParamValue,
		fmt.Sprintf("Path to %s", chainFuncName))
	flags.StringVarP(&chaincodeName, "name", "n", common.UndefinedParamValue,
		fmt.Sprint("Name of the chaincode"))
	flags.StringVarP(&chaincodeVersion, "version", "v", common.UndefinedParamValue,
		fmt.Sprint("Version of the chaincode specified in install/instantiate/upgrade commands"))
	flags.StringVarP(&chaincodeUsr, "username", "u", common.UndefinedParamValue,
		fmt.Sprint("Username for chaincode operations when security is enabled"))
	flags.StringVarP(&customIDGenAlg, "tid", "t", common.UndefinedParamValue,
		fmt.Sprint("Name of a custom ID generation algorithm (hashing and decoding) e.g. sha256base64"))
	flags.StringVarP(&chainID, "chainID", "C", util.GetTestChainID(),
		fmt.Sprint("The chain on which this command should be executed"))
	flags.StringVarP(&policy, "policy", "P", common.UndefinedParamValue,
		fmt.Sprint("The endorsement policy associated to this chaincode"))
	flags.StringVarP(&escc, "escc", "E", common.UndefinedParamValue,
		fmt.Sprint("The name of the endorsement system chaincode to be used for this chaincode"))
	flags.StringVarP(&vscc, "vscc", "V", common.UndefinedParamValue,
		fmt.Sprint("The name of the verification system chaincode to be used for this chaincode"))
	flags.StringVarP(&orderingEndpoint, "orderer", "o", "", "Ordering service endpoint")
	flags.BoolVarP(&tls, "tls", "", false, "Use TLS when communicating with the orderer endpoint")
	flags.StringVarP(&caFile, "cafile", "", "", "Path to file containing PEM-encoded trusted certificate(s) for the ordering endpoint")
}

// Cmd returns the cobra command for Chaincode
func Cmd(cf *ChaincodeCmdFactory) *cobra.Command {
	AddFlags(chaincodeCmd)

	chaincodeCmd.AddCommand(instantiateCmd(cf))
	chaincodeCmd.AddCommand(invokeCmd(cf))
	chaincodeCmd.AddCommand(queryCmd(cf))
	chaincodeCmd.AddCommand(upgradeCmd(cf))
	chaincodeCmd.AddCommand(packageCmd(cf))
	chaincodeCmd.AddCommand(installCmd(cf))

	return chaincodeCmd
}

// Chaincode-related variables.
var (
	chaincodeLang     string
	chaincodeCtorJSON string
	chaincodePath     string
	chaincodeName     string
	chaincodeUsr      string
	chaincodeQueryRaw bool
	chaincodeQueryHex bool
	customIDGenAlg    string
	chainID           string
	chaincodeVersion  string
	policy            string
	escc              string
	vscc              string
	policyMarhsalled  []byte
	orderingEndpoint  string
	tls               bool
	caFile            string
)

var chaincodeCmd = &cobra.Command{
	Use:   chainFuncName,
	Short: fmt.Sprintf("%s specific commands.", chainFuncName),
	Long:  fmt.Sprintf("%s specific commands.", chainFuncName),
}
