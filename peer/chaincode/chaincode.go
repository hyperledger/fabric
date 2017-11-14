/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	chainFuncName = "chaincode"
	shortDes      = "Operate a chaincode: install|instantiate|invoke|package|query|signpackage|upgrade|list."
	longDes       = "Operate a chaincode: install|instantiate|invoke|package|query|signpackage|upgrade|list."
)

var logger = flogging.MustGetLogger("chaincodeCmd")

func addFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()

	flags.StringVarP(&orderingEndpoint, "orderer", "o", "", "Ordering service endpoint")
	flags.BoolVarP(&tls, "tls", "", false, "Use TLS when communicating with the orderer endpoint")
	flags.StringVarP(&caFile, "cafile", "", "", "Path to file containing PEM-encoded trusted certificate(s) for the ordering endpoint")
	flags.StringVarP(&transient, "transient", "", "", "Transient map of arguments in JSON encoding")
}

// Cmd returns the cobra command for Chaincode
func Cmd(cf *ChaincodeCmdFactory) *cobra.Command {
	addFlags(chaincodeCmd)

	chaincodeCmd.AddCommand(installCmd(cf))
	chaincodeCmd.AddCommand(instantiateCmd(cf))
	chaincodeCmd.AddCommand(invokeCmd(cf))
	chaincodeCmd.AddCommand(packageCmd(cf, nil))
	chaincodeCmd.AddCommand(queryCmd(cf))
	chaincodeCmd.AddCommand(signpackageCmd(cf))
	chaincodeCmd.AddCommand(upgradeCmd(cf))
	chaincodeCmd.AddCommand(listCmd(cf))

	return chaincodeCmd
}

// Chaincode-related variables.
var (
	chaincodeLang            string
	chaincodeCtorJSON        string
	chaincodePath            string
	chaincodeName            string
	chaincodeUsr             string // Not used
	chaincodeQueryRaw        bool
	chaincodeQueryHex        bool
	customIDGenAlg           string
	channelID                string
	chaincodeVersion         string
	policy                   string
	escc                     string
	vscc                     string
	policyMarshalled         []byte
	orderingEndpoint         string
	tls                      bool
	caFile                   string
	transient                string
	resourceEnvelopeSavePath string
	resourceEnvelopeLoadPath string
)

var chaincodeCmd = &cobra.Command{
	Use:   chainFuncName,
	Short: fmt.Sprint(shortDes),
	Long:  fmt.Sprint(longDes),
}

var flags *pflag.FlagSet

func init() {
	resetFlags()
}

// Explicitly define a method to facilitate tests
func resetFlags() {
	flags = &pflag.FlagSet{}

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
	flags.StringVarP(&channelID, "channelID", "C", "",
		fmt.Sprint("The channel on which this command should be executed"))
	flags.StringVarP(&policy, "policy", "P", common.UndefinedParamValue,
		fmt.Sprint("The endorsement policy associated to this chaincode"))
	flags.StringVarP(&escc, "escc", "E", common.UndefinedParamValue,
		fmt.Sprint("The name of the endorsement system chaincode to be used for this chaincode"))
	flags.StringVarP(&vscc, "vscc", "V", common.UndefinedParamValue,
		fmt.Sprint("The name of the verification system chaincode to be used for this chaincode"))
	flags.StringVarP(&resourceEnvelopeSavePath, "resourceEnvelopeSavePath", "S", common.UndefinedParamValue,
		fmt.Sprint("Specifies the file to save the resource config update to. If not specified, sends config update"))
	flags.StringVarP(&resourceEnvelopeLoadPath, "resourceEnvelopeLoadPath", "L", common.UndefinedParamValue,
		fmt.Sprint("Specifies the file to load the resource config update from. If not specified, creates a new config update"))
	flags.BoolVarP(&getInstalledChaincodes, "installed", "", false,
		"Get the installed chaincodes on a peer")
	flags.BoolVarP(&getInstantiatedChaincodes, "instantiated", "", false,
		"Get the instantiated chaincodes on a channel")
}

func attachFlags(cmd *cobra.Command, names []string) {
	cmdFlags := cmd.Flags()
	for _, name := range names {
		if flag := flags.Lookup(name); flag != nil {
			cmdFlags.AddFlag(flag)
		} else {
			logger.Fatalf("Could not find flag '%s' to attach to command '%s'", name, cmd.Name())
		}
	}
}
