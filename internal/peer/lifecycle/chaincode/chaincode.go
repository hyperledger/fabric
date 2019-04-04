/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	lifecycleName   = "_lifecycle"
	approveFuncName = "ApproveChaincodeDefinitionForMyOrg"
	commitFuncName  = "CommitChaincodeDefinition"
)

var logger = flogging.MustGetLogger("cli.lifecycle.chaincode")

// XXX This is a terrible singleton hack, however
// it simply making a latent dependency explicit.
// It should be removed along with the other package
// scoped variables
var platformRegistry = platforms.NewRegistry(platforms.SupportedPlatforms...)

func addFlags(cmd *cobra.Command) {
	common.AddOrdererFlags(cmd)
}

// Cmd returns the cobra command for Chaincode
func Cmd() *cobra.Command {
	addFlags(chaincodeCmd)

	chaincodeCmd.AddCommand(PackageCmd(nil))
	chaincodeCmd.AddCommand(InstallCmd(nil))
	chaincodeCmd.AddCommand(QueryInstalledCmd(nil))
	chaincodeCmd.AddCommand(ApproveForMyOrgCmd(nil))
	chaincodeCmd.AddCommand(queryApprovalStatusCmd(nil))
	chaincodeCmd.AddCommand(CommitCmd(nil))
	chaincodeCmd.AddCommand(QueryCommittedCmd(nil))

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
	waitForEvent          bool
	waitForEventTimeout   time.Duration
	packageID             string
	sequence              int
	initRequired          bool
)

var chaincodeCmd = &cobra.Command{
	Use:   "chaincode",
	Short: "Perform chaincode operations: package|install|queryinstalled|approveformyorg|queryapprovalstatus|commit|querycommitted",
	Long:  "Perform _lifecycle operations: package|install|queryinstalled|approveformyorg|queryapprovalstatus|commit|querycommitted",
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
	flags.StringVarP(&endorsementPlugin, "endorsement-plugin", "E", "",
		fmt.Sprint("The name of the endorsement plugin to be used for this chaincode"))
	flags.StringVarP(&validationPlugin, "validation-plugin", "V", "",
		fmt.Sprint("The name of the validation plugin to be used for this chaincode"))
	flags.StringVar(&collectionsConfigFile, "collections-config", "",
		fmt.Sprint("The fully qualified path to the collection JSON file including the file name"))
	flags.StringArrayVarP(&peerAddresses, "peerAddresses", "", []string{""},
		fmt.Sprint("The addresses of the peers to connect to"))
	flags.StringArrayVarP(&tlsRootCertFiles, "tlsRootCertFiles", "", []string{""},
		fmt.Sprint("If TLS is enabled, the paths to the TLS root cert files of the peers to connect to. The order and number of certs specified should match the --peerAddresses flag"))
	flags.StringVarP(&connectionProfilePath, "connectionProfile", "", "",
		fmt.Sprint("The fully qualified path to the connection profile that provides the necessary connection information for the network. Note: currently only supported for providing peer connection information"))
	flags.BoolVar(&waitForEvent, "waitForEvent", true,
		fmt.Sprint("Whether to wait for the event from each peer's deliver filtered service signifying that the transaction has been committed successfully"))
	flags.DurationVar(&waitForEventTimeout, "waitForEventTimeout", 30*time.Second,
		fmt.Sprint("Time to wait for the event from each peer's deliver filtered service signifying that the 'invoke' transaction has been committed successfully"))
	flags.StringVarP(&packageID, "package-id", "", "", "The identifier of the chaincode install package")
	flags.IntVarP(&sequence, "sequence", "", 1, "The sequence number of the chaincode definition for the channel")
	flags.BoolVarP(&initRequired, "init-required", "", false, "Whether the chaincode requires invoking 'init'")
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
