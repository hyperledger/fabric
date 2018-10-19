/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
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
		dataDir     string
		snapDir     string
		walDir      string
		err         error
	)

	BeforeEach(func() {
		chainGetter = &mocks.ChainGetter{}
		support = &consensusmocks.FakeConsenterSupport{}
		dataDir, err = ioutil.TempDir("", "snap-")
		Expect(err).NotTo(HaveOccurred())
		walDir = path.Join(dataDir, "wal-")
		snapDir = path.Join(dataDir, "snap-")

		blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
		Expect(err).NotTo(HaveOccurred())

		goodConfigBlock := &common.Block{}
		proto.Unmarshal(blockBytes, goodConfigBlock)

		lastBlock := &common.Block{
			Header: &common.BlockHeader{
				Number: 1,
			},
			Data: goodConfigBlock.Data,
			Metadata: &common.BlockMetadata{
				Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
					Value: utils.MarshalOrPanic(&common.LastConfig{Index: 0}),
				})},
			},
		}

		support.BlockReturns(lastBlock)
	})

	AfterEach(func() {
		os.RemoveAll(dataDir)
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
			chainGetter.On("GetChain", "mychannel").Return(cs)
			chainGetter.On("GetChain", "badChainObject").Return(&multichannel.ChainSupport{})
			chainGetter.On("GetChain", "notmychannel").Return(nil)
			chainGetter.On("GetChain", "notraftchain").Return(&multichannel.ChainSupport{
				Chain: &multichannel.ChainSupport{},
			})
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
			Options: &etcdraftproto.Options{
				TickInterval:    100,
				ElectionTick:    10,
				HeartbeatTick:   1,
				MaxInflightMsgs: 256,
				MaxSizePerMsg:   1048576,
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{ConsensusMetadataVal: metadata})

		consenter := newConsenter(chainGetter)
		consenter.EtcdRaftConfig.WALDir = walDir
		consenter.EtcdRaftConfig.SnapDir = snapDir

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
			Options: &etcdraftproto.Options{
				TickInterval:    100,
				ElectionTick:    10,
				HeartbeatTick:   1,
				MaxInflightMsgs: 256,
				MaxSizePerMsg:   1048576,
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{ConsensusMetadataVal: metadata})

		consenter := newConsenter(chainGetter)

		chain, err := consenter.HandleChain(support, &common.Metadata{})
		Expect(chain).To(BeNil())
		Expect(err).To(MatchError("failed to detect own Raft ID because no matching certificate found"))
	})

	It("fails to handle chain if WAL is expected but no data found", func() {
		c := &etcdraftproto.Consenter{ServerTlsCert: []byte("cert.orderer0.org0")}
		m := &etcdraftproto.Metadata{
			Consenters: []*etcdraftproto.Consenter{c},
			Options: &etcdraftproto.Options{
				TickInterval:    100,
				ElectionTick:    10,
				HeartbeatTick:   1,
				MaxInflightMsgs: 256,
				MaxSizePerMsg:   1048576,
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{ConsensusMetadataVal: metadata})

		dir, err := ioutil.TempDir("", "wal-")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(dir)

		consenter := newConsenter(chainGetter)
		consenter.EtcdRaftConfig.WALDir = walDir
		consenter.EtcdRaftConfig.SnapDir = snapDir

		d := &etcdraftproto.RaftMetadata{
			Consenters: map[uint64]*etcdraftproto.Consenter{1: c},
			RaftIndex:  uint64(2),
		}
		chain, err := consenter.HandleChain(support, &common.Metadata{Value: utils.MarshalOrPanic(d)})

		Expect(chain).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("no WAL data found")))
	})

	It("fails to handle chain if etcdraft options have not been provided", func() {
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
		Expect(err).To(MatchError("etcdraft options have not been provided"))
	})

})

func newConsenter(chainGetter *mocks.ChainGetter) *etcdraft.Consenter {
	communicator := &clustermocks.Communicator{}
	ca, err := tlsgen.NewCA()
	Expect(err).NotTo(HaveOccurred())
	communicator.On("Configure", mock.Anything, mock.Anything)
	consenter := &etcdraft.Consenter{
		Communication: communicator,
		Cert:          []byte("cert.orderer0.org0"),
		Logger:        flogging.MustGetLogger("test"),
		Chains:        chainGetter,
		Dispatcher: &etcdraft.Dispatcher{
			Logger:        flogging.MustGetLogger("test"),
			ChainSelector: &mocks.ReceiverGetter{},
		},
		Dialer: cluster.NewTLSPinningDialer(comm.ClientConfig{
			SecOpts: &comm.SecureOptions{
				Certificate: ca.CertBytes(),
			},
		}),
	}
	return consenter
}
