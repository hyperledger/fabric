/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/internal/peer/packaging"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	chainFuncName = "chaincode"
	chainCmdDes   = "Operate a chaincode: install|instantiate|invoke|package|query|signpackage|upgrade|list."
)

var logger = flogging.MustGetLogger("chaincodeCmd")

// XXX This is a terrible singleton hack, however
// it simply making a latent dependency explicit.
// It should be removed along with the other package
// scoped variables
var platformRegistry = packaging.NewRegistry(packaging.SupportedPlatforms...)

func addFlags(cmd *cobra.Command) {
	common.AddOrdererFlags(cmd)
	flags := cmd.PersistentFlags()
	flags.StringVarP(&transient, "transient", "", "", "Transient map of arguments in JSON encoding")
}

// Cmd returns the cobra command for Chaincode
func Cmd(cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) *cobra.Command {
	addFlags(chaincodeCmd)

	chaincodeCmd.AddCommand(installCmd(cf, nil, cryptoProvider))
	chaincodeCmd.AddCommand(instantiateCmd(cf, cryptoProvider))
	chaincodeCmd.AddCommand(invokeCmd(cf, cryptoProvider))
	chaincodeCmd.AddCommand(packageCmd(cf, nil, nil, cryptoProvider))
	chaincodeCmd.AddCommand(queryCmd(cf, cryptoProvider))
	chaincodeCmd.AddCommand(signpackageCmd(cf, cryptoProvider))
	chaincodeCmd.AddCommand(upgradeCmd(cf, cryptoProvider))
	chaincodeCmd.AddCommand(listCmd(cf, cryptoProvider))

	return chaincodeCmd
}

// Chaincode-related variables.
var (
	chaincodeLang         string
	chaincodeCtorJSON     string
	chaincodePath         string
	chaincodeName         string
	chaincodeUsr          string // Not used
	chaincodeQueryRaw     bool
	chaincodeQueryHex     bool
	channelID             string
	chaincodeVersion      string
	policy                string
	escc                  string
	vscc                  string
	policyMarshalled      []byte
	transient             string
	isInit                bool
	collectionsConfigFile string
	collectionConfigBytes []byte
	peerAddresses         []string
	tlsRootCertFiles      []string
	connectionProfile     string
	waitForEvent          bool
	waitForEventTimeout   time.Duration
)

var chaincodeCmd = &cobra.Command{
	Use:   chainFuncName,
	Short: fmt.Sprint(chainCmdDes),
	Long:  fmt.Sprint(chainCmdDes),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		common.InitCmd(cmd, args)
		common.SetOrdererEnv(cmd, args)
	},
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
		"Name of the chaincode")
	flags.StringVarP(&chaincodeVersion, "version", "v", common.UndefinedParamValue,
		"Version of the chaincode specified in install/instantiate/upgrade commands")
	flags.StringVarP(&chaincodeUsr, "username", "u", common.UndefinedParamValue,
		"Username for chaincode operations when security is enabled")
	flags.StringVarP(&channelID, "channelID", "C", "",
		"The channel on which this command should be executed")
	flags.StringVarP(&policy, "policy", "P", common.UndefinedParamValue,
		"The endorsement policy associated to this chaincode")
	flags.StringVarP(&escc, "escc", "E", common.UndefinedParamValue,
		"The name of the endorsement system chaincode to be used for this chaincode")
	flags.StringVarP(&vscc, "vscc", "V", common.UndefinedParamValue,
		"The name of the verification system chaincode to be used for this chaincode")
	flags.BoolVarP(&isInit, "isInit", "I", false,
		"Is this invocation for init (useful for supporting legacy chaincodes in the new lifecycle)")
	flags.BoolVarP(&getInstalledChaincodes, "installed", "", false,
		"Get the installed chaincodes on a peer")
	flags.BoolVarP(&getInstantiatedChaincodes, "instantiated", "", false,
		"Get the instantiated chaincodes on a channel")
	flags.StringVar(&collectionsConfigFile, "collections-config", common.UndefinedParamValue,
		"The fully qualified path to the collection JSON file including the file name")
	flags.StringArrayVarP(&peerAddresses, "peerAddresses", "", []string{common.UndefinedParamValue},
		"The addresses of the peers to connect to")
	flags.StringArrayVarP(&tlsRootCertFiles, "tlsRootCertFiles", "", []string{common.UndefinedParamValue},
		"If TLS is enabled, the paths to the TLS root cert files of the peers to connect to. The order and number of certs specified should match the --peerAddresses flag")
	flags.StringVarP(&connectionProfile, "connectionProfile", "", common.UndefinedParamValue,
		"Connection profile that provides the necessary connection information for the network. Note: currently only supported for providing peer connection information")
	flags.BoolVar(&waitForEvent, "waitForEvent", false,
		"Whether to wait for the event from each peer's deliver filtered service signifying that the 'invoke' transaction has been committed successfully")
	flags.DurationVar(&waitForEventTimeout, "waitForEventTimeout", 30*time.Second,
		"Time to wait for the event from each peer's deliver filtered service signifying that the 'invoke' transaction has been committed successfully")
	flags.BoolVarP(&createSignedCCDepSpec, "cc-package", "s", false,
		"create CC deployment spec for owner endorsements instead of raw CC deployment spec")
	flags.BoolVarP(&signCCDepSpec, "sign", "S", false,
		"if creating CC deployment spec package for owner endorsements, also sign it with local MSP")
	flags.StringVarP(&instantiationPolicy, "instantiate-policy", "i", "",
		"instantiation policy for the chaincode")
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
