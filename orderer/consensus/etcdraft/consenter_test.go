/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"github.com/hyperledger/fabric/common/flogging"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	clustermocks "github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft/mocks"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	etcdraftproto "github.com/hyperledger/fabric/protos/orderer/etcdraft"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Consenter", func() {
	var (
		chainGetter *mocks.ChainGetter
		support     *consensusmocks.FakeConsenterSupport
	)

	BeforeEach(func() {
		chainGetter = &mocks.ChainGetter{}
		support = &consensusmocks.FakeConsenterSupport{}
	})

	When("the consenter is extracting the channel", func() {
		It("extracts successfully from step requests", func() {
			consenter := newConsenter(chainGetter)
			ch := consenter.TargetChannel(&orderer.StepRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})
		It("extracts successfully from submit requests", func() {
			consenter := newConsenter(chainGetter)
			ch := consenter.TargetChannel(&orderer.SubmitRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})
		It("returns an empty string for the rest of the messages", func() {
			consenter := newConsenter(chainGetter)
			ch := consenter.TargetChannel(&common.Block{})
			Expect(ch).To(BeEmpty())
		})
	})

	When("the consenter is asked for a chain", func() {
		chainInstance := &etcdraft.Chain{}
		cs := &multichannel.ChainSupport{
			Chain: chainInstance,
		}
		BeforeEach(func() {
			chainGetter.On("GetChain", "mychannel").Return(cs, true)
			chainGetter.On("GetChain", "badChainObject").Return(&multichannel.ChainSupport{}, true)
			chainGetter.On("GetChain", "notmychannel").Return(nil, false)
			chainGetter.On("GetChain", "notraftchain").Return(&multichannel.ChainSupport{
				Chain: &multichannel.ChainSupport{},
			}, true)
		})
		It("calls the chain getter and returns the reference when it is found", func() {
			consenter := newConsenter(chainGetter)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("mychannel")
			Expect(chain).NotTo(BeNil())
			Expect(chain).To(BeIdenticalTo(chainInstance))
		})
		It("calls the chain getter and returns nil when it's not found", func() {
			consenter := newConsenter(chainGetter)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("notmychannel")
			Expect(chain).To(BeNil())
		})
		It("calls the chain getter and returns nil when it's not a raft chain", func() {
			consenter := newConsenter(chainGetter)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("notraftchain")
			Expect(chain).To(BeNil())
		})
		It("calls the chain getter and panics when the chain has a bad internal state", func() {
			consenter := newConsenter(chainGetter)
			Expect(consenter).NotTo(BeNil())

			Expect(func() {
				consenter.ReceiverByChain("badChainObject")
			}).To(Panic())
		})
	})

	It("successfully constructs a Chain", func() {
		certBytes := []byte("cert.orderer0.org0")
		m := &etcdraftproto.Metadata{
			Consenters: []*etcdraftproto.Consenter{
				{ServerTlsCert: certBytes},
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{ConsensusMetadataVal: metadata})

		consenter := newConsenter(chainGetter)

		chain, err := consenter.HandleChain(support, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(chain).NotTo(BeNil())

		Expect(chain.Start).NotTo(Panic())
	})

	It("fails to handle chain if no matching cert found", func() {
		m := &etcdraftproto.Metadata{
			Consenters: []*etcdraftproto.Consenter{
				{ServerTlsCert: []byte("cert.orderer1.org1")},
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{ConsensusMetadataVal: metadata})

		consenter := newConsenter(chainGetter)

		chain, err := consenter.HandleChain(support, nil)
		Expect(chain).To(BeNil())
		Expect(err).To(MatchError("failed to detect own Raft ID because no matching certificate found"))
	})
})

func newConsenter(chainGetter *mocks.ChainGetter) *etcdraft.Consenter {
	comm := &clustermocks.Communicator{}
	comm.On("Configure", mock.Anything, mock.Anything)
	consenter := &etcdraft.Consenter{
		Communication: comm,
		Cert:          []byte("cert.orderer0.org0"),
		Logger:        flogging.MustGetLogger("test"),
		Chains:        chainGetter,
		Dispatcher: &etcdraft.Dispatcher{
			Logger:        flogging.MustGetLogger("test"),
			ChainSelector: &mocks.ReceiverGetter{},
		},
	}
	return consenter
}
