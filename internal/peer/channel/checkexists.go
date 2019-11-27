/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package channel

import (
	"fmt"
	"strings"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func checkexistsCmd(cf *ChannelCmdFactory) *cobra.Command {
	// Set the flags on the channel start command.
	checkCmd := &cobra.Command{
		Use:   "checkexists",
		Short: "Check if a certain channel exists.",
		Long:  "Check if a certain channel exists.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected: %s", args)
			}
			return checkexists(cmd, args, cf)
		},
	}
	flagList := []string{
		"channelID",
	}
	attachFlags(checkCmd, flagList)

	return checkCmd

}

func checkexists(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	// the global chainID filled by the "-c" command
	if channelID == common.UndefinedParamValue {
		return errors.New("must supply channel ID")
	}

	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	// default to fetching from orderer
	ordererRequired := OrdererRequired
	peerDeliverRequired := PeerDeliverNotRequired
	if len(strings.Split(common.OrderingEndpoint, ":")) != 2 {
		// if no orderer endpoint supplied, connect to peer's deliver service
		ordererRequired = OrdererNotRequired
		peerDeliverRequired = PeerDeliverRequired
	}
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserNotRequired, peerDeliverRequired, ordererRequired)
		if err != nil {
			return err
		}
	}

	_, err = cf.DeliverClient.GetOldestBlock()
	if err != nil {
		if strings.Contains(err.Error(), "&{NOT_FOUND}") {
			fmt.Printf("Channel %s doesn't exists\n", channelID)
			osExit(99)
			return nil
		}
		return err
	}

	fmt.Printf("Channel %s exists\n", channelID)
	osExit(0)
	return nil
}
