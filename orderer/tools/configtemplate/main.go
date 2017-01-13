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

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/tools/baseconfig")

const defaultOutputFile = "orderer.template"

func main() {
	var outputFile string
	flag.StringVar(&outputFile, "outputFile", defaultOutputFile, "The file to write the configuration templatee to")
	flag.Parse()

	conf := config.Load()
	flogging.InitFromSpec(conf.General.LogLevel)

	logger.Debugf("Initializing generator")
	generator := provisional.New(conf)

	logger.Debugf("Producing template items")
	templateItems := generator.TemplateItems()

	logger.Debugf("Encoding configuration template")
	outputData := utils.MarshalOrPanic(&cb.ConfigurationTemplate{
		Items: templateItems,
	})

	logger.Debugf("Writing configuration to disk")
	ioutil.WriteFile(outputFile, outputData, 0644)

}
