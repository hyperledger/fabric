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
	"io/ioutil"
	"strconv"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/cobra"
)

func fetchCmd(cf *ChannelCmdFactory) *cobra.Command {
	fetchCmd := &cobra.Command{
		Use:   "fetch <newest|oldest|config|(number)> [outputfile]",
		Short: "Fetch a block",
		Long:  "Fetch a specified block, writing it to a file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fetch(cmd, args, cf)
		},
	}
	flagList := []string{
		"channelID",
	}
	attachFlags(fetchCmd, flagList)

	return fetchCmd
}

func fetch(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserNotRequired, OrdererRequired)
		if err != nil {
			return err
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("fetch target required, oldest, newest, config, or a number")
	}

	if len(args) > 2 {
		return fmt.Errorf("trailing args detected")
	}

	var block *cb.Block

	switch args[0] {
	case "oldest":
		block, err = cf.DeliverClient.getOldestBlock()
	case "newest":
		block, err = cf.DeliverClient.getNewestBlock()
	case "config":
		iBlock, err := cf.DeliverClient.getNewestBlock()
		if err != nil {
			return err
		}
		lc, err := utils.GetLastConfigIndexFromBlock(iBlock)
		if err != nil {
			return err
		}
		block, err = cf.DeliverClient.getSpecifiedBlock(lc)
	default:
		num, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("fetch target illegal: %s", args[0])
		}
		block, err = cf.DeliverClient.getSpecifiedBlock(uint64(num))
	}

	if err != nil {
		return err
	}

	b, err := proto.Marshal(block)
	if err != nil {
		return err
	}

	var file string
	if len(args) == 1 {
		file = chainID + "_" + args[0] + ".block"
	} else {
		file = args[1]
	}

	if err = ioutil.WriteFile(file, b, 0644); err != nil {
		return err
	}

	return nil
}
