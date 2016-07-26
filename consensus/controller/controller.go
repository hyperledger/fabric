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

package controller

import (
	"strings"

	"github.com/op/go-logging"
	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/consensus"
	"github.com/hyperledger/fabric/consensus/noops"
	"github.com/hyperledger/fabric/consensus/pbft"
)

var logger *logging.Logger // package-level logger
var consenter consensus.Consenter

func init() {
	logger = logging.MustGetLogger("consensus/controller")
}

// NewConsenter constructs a Consenter object if not already present
func NewConsenter(stack consensus.Stack) consensus.Consenter {

	plugin := strings.ToLower(viper.GetString("peer.validator.consensus.plugin"))
	if plugin == "pbft" {
		logger.Infof("Creating consensus plugin %s", plugin)
		return pbft.GetPlugin(stack)
	}
	logger.Info("Creating default consensus plugin (noops)")
	return noops.GetNoops(stack)

}
