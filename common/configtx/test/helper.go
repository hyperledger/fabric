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

package test

import (
	"io/ioutil"
	"os"

	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/genesis"
	peersharedconfig "github.com/hyperledger/fabric/peer/sharedconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/peer"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("common/configtx/test")

const (
	// AcceptAllPolicyKey is the key of the AcceptAllPolicy.
	AcceptAllPolicyKey = "AcceptAllPolicy"
)

const (
	OrdererTemplateName = "orderer.template"
	MSPTemplateName     = "msp.template"
)

var ordererTemplate configtx.Template
var mspTemplate configtx.Template

var genesisFactory genesis.Factory

func readTemplate(name string) configtx.Template {
	gopath := os.Getenv("GOPATH")
	data, err := ioutil.ReadFile(gopath + "/src/github.com/hyperledger/fabric/common/configtx/test/" + name)
	if err != nil {
		peerConfig := os.Getenv("PEER_CFG_PATH")
		data, err = ioutil.ReadFile(peerConfig + "/common/configtx/test/" + name)
		if err != nil {
			panic(err)
		}
	}

	templateProto := &cb.ConfigurationTemplate{}
	err = proto.Unmarshal(data, templateProto)
	if err != nil {
		panic(err)
	}

	return configtx.NewSimpleTemplate(templateProto.Items...)
}

func init() {
	ordererTemplate = readTemplate(OrdererTemplateName)
	mspTemplate = readTemplate(MSPTemplateName)
	anchorPeers := []*peer.AnchorPeer{{Host: "fakehost", Port: 2000, Cert: []byte{}}}
	gossTemplate := configtx.NewSimpleTemplate(peersharedconfig.TemplateAnchorPeers(anchorPeers))
	genesisFactory = genesis.NewFactoryImpl(configtx.NewCompositeTemplate(mspTemplate, ordererTemplate, gossTemplate))
}

// WriteTemplate takes an output file and set of config items and writes them to that file as a marshaled ConfigurationTemplate
func WriteTemplate(outputFile string, items ...*cb.ConfigurationItem) {
	logger.Debugf("Encoding configuration template")
	outputData := utils.MarshalOrPanic(&cb.ConfigurationTemplate{
		Items: items,
	})

	logger.Debugf("Writing configuration to disk")
	ioutil.WriteFile(outputFile, outputData, 0644)
}

func MakeGenesisBlock(chainID string) (*cb.Block, error) {
	return genesisFactory.Block(chainID)
}

// GetOrderererTemplate returns the test orderer template
func GetOrdererTemplate() configtx.Template {
	return ordererTemplate
}

// GetMSPerTemplate returns the test MSP template
func GetMSPTemplate() configtx.Template {
	return mspTemplate
}
