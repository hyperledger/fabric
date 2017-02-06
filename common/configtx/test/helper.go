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
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
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
	PeerTemplateName    = "peer.template"
)

var ordererTemplate configtx.Template
var mspTemplate configtx.Template
var peerTemplate configtx.Template

var compositeTemplate configtx.Template

var genesisFactory genesis.Factory

func init() {
	ordererTemplate = readTemplate(OrdererTemplateName)
	mspTemplate = readTemplate(MSPTemplateName)
	peerTemplate = readTemplate(PeerTemplateName)

	compositeTemplate = configtx.NewCompositeTemplate(mspTemplate, ordererTemplate, peerTemplate)
	genesisFactory = genesis.NewFactoryImpl(compositeTemplate)
}

func resolveName(name string) (string, []byte) {
	path := os.Getenv("GOPATH") + "/src/github.com/hyperledger/fabric/common/configtx/test/" + name
	data, err := ioutil.ReadFile(path)
	if err == nil {
		return path, data
	}

	path = os.Getenv("PEER_CFG_PATH") + "/common/configtx/test/" + name
	data, err = ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	return path, data
}

func readTemplate(name string) configtx.Template {
	_, data := resolveName(name)

	templateProto := &cb.ConfigTemplate{}
	err := proto.Unmarshal(data, templateProto)
	if err != nil {
		panic(err)
	}

	return configtx.NewSimpleTemplate(templateProto.Items...)
}

// WriteTemplate takes an output file and set of config items and writes them to that file as a marshaled ConfigTemplate
func WriteTemplate(name string, items ...*cb.ConfigItem) {
	path, _ := resolveName(name)

	logger.Debugf("Encoding config template")
	outputData := utils.MarshalOrPanic(&cb.ConfigTemplate{
		Items: items,
	})

	logger.Debugf("Writing config to %s", path)
	ioutil.WriteFile(path, outputData, 0644)
}

// MakeGenesisBlock creates a genesis block using the test templates for the given chainID
func MakeGenesisBlock(chainID string) (*cb.Block, error) {
	return genesisFactory.Block(chainID)
}

// OrderererTemplate returns the test orderer template
func OrdererTemplate() configtx.Template {
	return ordererTemplate
}

// MSPerTemplate returns the test MSP template
func MSPTemplate() configtx.Template {
	return mspTemplate
}

// MSPerTemplate returns the test peer template
func PeerTemplate() configtx.Template {
	return peerTemplate
}

// CompositeTemplate returns the composite template of peer, orderer, and MSP
func CompositeTemplate() configtx.Template {
	return compositeTemplate
}
