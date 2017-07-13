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

package clilogging

import (
	"github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/spf13/cobra"
)

// LoggingCmdFactory holds the clients used by LoggingCmd
type LoggingCmdFactory struct {
	AdminClient pb.AdminClient
}

// InitCmdFactory init the LoggingCmdFactory with default admin client
func InitCmdFactory() (*LoggingCmdFactory, error) {
	var err error
	var adminClient pb.AdminClient

	adminClient, err = common.GetAdminClient()
	if err != nil {
		return nil, err
	}

	return &LoggingCmdFactory{
		AdminClient: adminClient,
	}, nil
}

func checkLoggingCmdParams(cmd *cobra.Command, args []string) error {
	var err error
	if cmd.Name() == "revertlevels" {
		if len(args) > 0 {
			err = errors.ErrorWithCallstack("LOG", "400", "More parameters than necessary were provided. Expected 0, received %d.", len(args))
			return err
		}
	} else {
		// check that at least one parameter is passed in
		if len(args) == 0 {
			err = errors.ErrorWithCallstack("LOG", "400", "No parameters provided.")
			return err
		}
	}

	if cmd.Name() == "setlevel" {
		// check that log level parameter is provided
		if len(args) == 1 {
			err = errors.ErrorWithCallstack("LOG", "400", "No log level provided.")
		} else {
			// check that log level is valid. if not, err is set
			err = common.CheckLogLevel(args[1])
		}
	}

	return err
}
