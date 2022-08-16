/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/integration/nwo"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigTx ConsenterMapping", func() {
	It("generates genesis block and checks it contains the ConsenterMapping", func() {
		var err error
		testDir, err := ioutil.TempDir("", "configtx")
		Expect(err).NotTo(HaveOccurred())

		network := nwo.New(nwo.MultiNodeBFT(), testDir, nil, StartPort(), components)

		channelName := network.Channels[0].Name

		// Generate config
		network.GenerateConfigTree()

		// bootstrap the network, which generates the genesis block
		network.Bootstrap()

		// check the config transaction in the genesis block contains the ConsenterMapping
		sysProfile := genesisconfig.Load("SampleDevModeBFT", network.RootDir)
		Expect(sysProfile.Orderer).NotTo(BeNil())
		pgen := encoder.New(sysProfile)
		genesisBlock := pgen.GenesisBlockForChannel(channelName)
		Expect(genesisBlock.GetData().GetData()).NotTo(BeNil())
		env, err := protoutil.UnmarshalEnvelope(genesisBlock.Data.Data[0])
		Expect(err).NotTo(HaveOccurred())
		payload, err := protoutil.UnmarshalPayload(env.Payload)
		Expect(err).NotTo(HaveOccurred())
		configEnv, err := protoutil.UnmarshalConfigEnvelope(payload.GetData())
		Expect(err).NotTo(HaveOccurred())
		group := configEnv.GetConfig().GetChannelGroup().GetGroups()["Orderer"]
		o := &common.Orderers{}
		err = proto.Unmarshal(group.GetValues()["Orderers"].GetValue(), o)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(o.GetConsenterMapping())).To(Equal(3))
	})
})
