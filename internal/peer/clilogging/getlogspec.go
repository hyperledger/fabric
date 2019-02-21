/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package clilogging

import (
	"context"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
)

func getLogSpecCmd(cf *LoggingCmdFactory) *cobra.Command {
	var loggingGetLogSpecCmd = &cobra.Command{
		Use:   "getlogspec",
		Short: "Returns the active log spec.",
		Long:  `Returns the active logging specification of the peer.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getLogSpec(cf, cmd, args)
		},
	}

	return loggingGetLogSpecCmd
}

func getLogSpec(cf *LoggingCmdFactory, cmd *cobra.Command, args []string) (err error) {
	err = checkLoggingCmdParams(cmd, args)
	if err == nil {
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true

		if cf == nil {
			cf, err = InitCmdFactory()
			if err != nil {
				return err
			}
		}
		env := cf.wrapWithEnvelope(&pb.AdminOperation{})
		logResponse, err := cf.AdminClient.GetLogSpec(context.Background(), env)
		if err != nil {
			return err
		}
		logger.Infof("Current logging spec: %s", logResponse.LogSpec)
	}
	return err
}
