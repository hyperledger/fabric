/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	lifecycleName                = "_lifecycle"
	approveFuncName              = "ApproveChaincodeDefinitionForMyOrg"
	commitFuncName               = "CommitChaincodeDefinition"
	checkCommitReadinessFuncName = "CheckCommitReadiness"
)

var logger = flogging.MustGetLogger("cli.lifecycle.chaincode")

func addFlags(cmd *cobra.Command) {
	common.AddOrdererFlags(cmd)
}

// Cmd returns the cobra command for Chaincode
func Cmd(cryptoProvider bccsp.BCCSP) *cobra.Command {
	addFlags(chaincodeCmd)

	chaincodeCmd.AddCommand(PackageCmd(nil))
	chaincodeCmd.AddCommand(CalculatePackageIDCmd(nil))
	chaincodeCmd.AddCommand(InstallCmd(nil, cryptoProvider))
	chaincodeCmd.AddCommand(QueryInstalledCmd(nil, cryptoProvider))
	chaincodeCmd.AddCommand(GetInstalledPackageCmd(nil, cryptoProvider))
	chaincodeCmd.AddCommand(ApproveForMyOrgCmd(nil, cryptoProvider))
	chaincodeCmd.AddCommand(QueryApprovedCmd(nil, cryptoProvider))
	chaincodeCmd.AddCommand(CheckCommitReadinessCmd(nil, cryptoProvider))
	chaincodeCmd.AddCommand(CommitCmd(nil, cryptoProvider))
	chaincodeCmd.AddCommand(QueryCommittedCmd(nil, cryptoProvider))

	return chaincodeCmd
}

// Chaincode-related variables.
var (
	chaincodeLang         string
	chaincodePath         string
	chaincodeName         string
	channelID             string
	chaincodeVersion      string
	packageLabel          string
	signaturePolicy       string
	channelConfigPolicy   string
	endorsementPlugin     string
	validationPlugin      string
	collectionsConfigFile string
	peerAddresses         []string
	tlsRootCertFiles      []string
	connectionProfilePath string
	targetPeer            string
	waitForEvent          bool
	waitForEventTimeout   time.Duration
	packageID             string
	sequence              int
	initRequired          bool
	output                string
	outputDirectory       string
)

var chaincodeCmd = &cobra.Command{
	Use:   "chaincode",
	Short: "Perform chaincode operations: package|install|queryinstalled|getinstalledpackage|calculatepackageid|approveformyorg|queryapproved|checkcommitreadiness|commit|querycommitted",
	Long:  "Perform chaincode operations: package|install|queryinstalled|getinstalledpackage|calculatepackageid|approveformyorg|queryapproved|checkcommitreadiness|commit|querycommitted",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		common.InitCmd(cmd, args)
		common.SetOrdererEnv(cmd, args)
	},
}

var flags *pflag.FlagSet

func init() {
	ResetFlags()
}

// ResetFlags resets the values of these flags to facilitate tests
func ResetFlags() {
	flags = &pflag.FlagSet{}

	flags.StringVarP(&chaincodeLang, "lang", "l", "golang", "Language the chaincode is written in")
	flags.StringVarP(&chaincodePath, "path", "p", "", "Path to the chaincode")
	flags.StringVarP(&chaincodeName, "name", "n", "", "Name of the chaincode")
	flags.StringVarP(&chaincodeVersion, "version", "v", "", "Version of the chaincode")
	flags.StringVarP(&packageLabel, "label", "", "", "The package label contains a human-readable description of the package")
	flags.StringVarP(&channelID, "channelID", "C", "", "The channel on which this command should be executed")
	flags.StringVarP(&signaturePolicy, "signature-policy", "", "", "The endorsement policy associated to this chaincode specified as a signature policy")
	flags.StringVarP(&channelConfigPolicy, "channel-config-policy", "", "", "The endorsement policy associated to this chaincode specified as a channel config policy reference")
	flags.StringVarP(&endorsementPlugin, "endorsement-plugin", "E", "", "The name of the endorsement plugin to be used for this chaincode")
	flags.StringVarP(&validationPlugin, "validation-plugin", "V", "", "The name of the validation plugin to be used for this chaincode")
	flags.StringVar(&collectionsConfigFile, "collections-config", "", "The fully qualified path to the collection JSON file including the file name")
	flags.StringArrayVarP(&peerAddresses, "peerAddresses", "", []string{""}, "The addresses of the peers to connect to")
	flags.StringArrayVarP(&tlsRootCertFiles, "tlsRootCertFiles", "", []string{""},
		"If TLS is enabled, the paths to the TLS root cert files of the peers to connect to. The order and number of certs specified should match the --peerAddresses flag")
	flags.StringVarP(&connectionProfilePath, "connectionProfile", "", "",
		"The fully qualified path to the connection profile that provides the necessary connection information for the network. Note: currently only supported for providing peer connection information")
	flags.StringVarP(&targetPeer, "targetPeer", "", "",
		"When using a connection profile, the name of the peer to target for this action")
	flags.BoolVar(&waitForEvent, "waitForEvent", true,
		"Whether to wait for the event from each peer's deliver filtered service signifying that the transaction has been committed successfully")
	flags.DurationVar(&waitForEventTimeout, "waitForEventTimeout", 30*time.Second,
		"Time to wait for the event from each peer's deliver filtered service signifying that the 'invoke' transaction has been committed successfully")
	flags.StringVarP(&packageID, "package-id", "", "", "The identifier of the chaincode install package")
	flags.IntVarP(&sequence, "sequence", "", 0, "The sequence number of the chaincode definition for the channel")
	flags.BoolVarP(&initRequired, "init-required", "", false, "Whether the chaincode requires invoking 'init'")
	flags.StringVarP(&output, "output", "O", "", "The output format for query results. Default is human-readable plain-text. json is currently the only supported format.")
	flags.StringVarP(&outputDirectory, "output-directory", "", "", "The output directory to use when writing a chaincode install package to disk. Default is the current working directory.")
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
