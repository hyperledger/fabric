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

package main

import (
	"flag"
	"io/ioutil"

	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/protos/utils"

	logging "github.com/op/go-logging"
)

const (
	DefaultGenesisFileLocation = "genesis.block"
)

var logger = logging.MustGetLogger("common/configtx/tool")

func main() {
	var writePath, profile string

	dryRun := flag.Bool("dryRun", false, "Whether to only generate but not write the genesis block file.")
	flag.StringVar(&writePath, "path", DefaultGenesisFileLocation, "The path to write the genesis block to.")
	flag.StringVar(&profile, "profile", genesisconfig.SampleInsecureProfile, "The profile from configtx.yaml to use for generation.")
	flag.Parse()

	logging.SetLevel(logging.INFO, "")

	logger.Info("Loading configuration")
	config := genesisconfig.Load(profile)

	logger.Info("Generating genesis block")
	genesisBlock := provisional.New(config).GenesisBlock()

	if !*dryRun {
		logger.Info("Writing genesis block")
		ioutil.WriteFile(writePath, utils.MarshalOrPanic(genesisBlock), 0644)
	}
}
