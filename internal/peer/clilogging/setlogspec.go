/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package clilogging

import (
	"context"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func setLogSpecCmd(cf *LoggingCmdFactory) *cobra.Command {
	var loggingSetLogSpecCmd = &cobra.Command{
		Use:   "setlogspec",
		Short: "Sets the logging spec.",
		Long:  `Sets the active logging specification of the peer.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return setLogSpec(cf, cmd, args)
		},
	}

	return loggingSetLogSpecCmd
}

func setLogSpec(cf *LoggingCmdFactory, cmd *cobra.Command, args []string) (err error) {
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
		op := &pb.AdminOperation{
			Content: &pb.AdminOperation_LogSpecReq{
				LogSpecReq: &pb.LogSpecRequest{
					LogSpec: args[0],
				},
			},
		}
		env := cf.wrapWithEnvelope(op)
		logResponse, err := cf.AdminClient.SetLogSpec(context.Background(), env)
		if err != nil {
			return err
		}
		if logResponse.Error != "" {
			return errors.New(logResponse.Error)
		}
		logger.Infof("Current logging spec set to: %s", logResponse.LogSpec)
	}
	return err
}
