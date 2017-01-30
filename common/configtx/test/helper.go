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
)

const (
	// AcceptAllPolicyKey is the key of the AcceptAllPolicy.
	AcceptAllPolicyKey = "AcceptAllPolicy"
)

var ordererTemplate configtx.Template

var genesisFactory genesis.Factory

// XXX This is a hacky singleton, which should go away, but is an artifact of using the existing utils implementation
type MSPTemplate struct{}

func (msp MSPTemplate) Items(chainID string) ([]*cb.SignedConfigurationItem, error) {
	return []*cb.SignedConfigurationItem{utils.EncodeMSP(chainID)}, nil
}

func init() {

	gopath := os.Getenv("GOPATH")
	data, err := ioutil.ReadFile(gopath + "/src/github.com/hyperledger/fabric/common/configtx/test/orderer.template")
	if err != nil {
		peerConfig := os.Getenv("PEER_CFG_PATH")
		data, err = ioutil.ReadFile(peerConfig + "/common/configtx/test/orderer.template")
		if err != nil {
			panic(err)
		}
	}
	templateProto := &cb.ConfigurationTemplate{}
	err = proto.Unmarshal(data, templateProto)
	if err != nil {
		panic(err)
	}

	ordererTemplate = configtx.NewSimpleTemplate(templateProto.Items...)
	anchorPeers := []*peer.AnchorPeer{{Host: "fakehost", Port: 2000, Cert: []byte{}}}
	gossTemplate := configtx.NewSimpleTemplate(peersharedconfig.TemplateAnchorPeers(anchorPeers))
	genesisFactory = genesis.NewFactoryImpl(configtx.NewCompositeTemplate(MSPTemplate{}, ordererTemplate, gossTemplate))
}

func MakeGenesisBlock(chainID string) (*cb.Block, error) {
	return genesisFactory.Block(chainID)
}

// GetOrderererTemplate returns the test orderer template
func GetOrdererTemplate() configtx.Template {
	return ordererTemplate
}
