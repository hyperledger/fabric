/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"fmt"
	"runtime"

	"github.com/hyperledger/fabric/common/metadata"
	"github.com/spf13/cobra"
)

// Program name
const ProgramName = "peer"

// Cmd returns the Cobra Command for Version
func Cmd() *cobra.Command {
	return cobraCommand
}

var cobraCommand = &cobra.Command{
	Use:   "version",
	Short: "Print fabric peer version.",
	Long:  `Print current version of the fabric peer server.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(GetInfo())
	},
}

// GetInfo returns version information for the peer
func GetInfo() string {
	if metadata.Version == "" {
		metadata.Version = "development build"
	}

	ccinfo := fmt.Sprintf(" Base Image Version: %s\n"+
		"  Base Docker Namespace: %s\n"+
		"  Base Docker Label: %s\n"+
		"  Docker Namespace: %s\n",
		metadata.BaseVersion, metadata.BaseDockerNamespace,
		metadata.BaseDockerLabel, metadata.DockerNamespace)

	return fmt.Sprintf("%s:\n Version: %s\n Go version: %s\n OS/Arch: %s\n"+
		" Chaincode:\n %s\n",
		ProgramName, metadata.Version, runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH), ccinfo)
}
