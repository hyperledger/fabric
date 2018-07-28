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

package node

import (
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/spf13/cobra"
)

const (
	nodeFuncName = "node"
	nodeCmdDes   = "Operate a peer node: start|status."
)

var logger = flogging.MustGetLogger("nodeCmd")

// Cmd returns the cobra command for Node
func Cmd() *cobra.Command {
	nodeCmd.AddCommand(startCmd())
	nodeCmd.AddCommand(statusCmd())

	return nodeCmd
}

var nodeCmd = &cobra.Command{
	Use:              nodeFuncName,
	Short:            fmt.Sprint(nodeCmdDes),
	Long:             fmt.Sprint(nodeCmdDes),
	PersistentPreRun: common.InitCmd,
}
