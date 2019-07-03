/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"encoding/pem"
	"io/ioutil"
	"os"
	"path"
	"strings"

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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("Consenter", func() {
	var (
		certAsPEM   []byte
		chainGetter *mocks.ChainGetter
		support     *consensusmocks.FakeConsenterSupport
		dataDir     string
		snapDir     string
		walDir      string
		err         error
	)

	BeforeEach(func() {
		certAsPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("cert bytes")})
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
			ch := consenter.TargetChannel(&orderer.ConsensusRequest{Channel: "mychannel"})
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
		// We append a line feed to our cert, just to ensure that we can still consume it and ignore.
		certAsPEMWithLineFeed := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("cert bytes")})
		certAsPEMWithLineFeed = append(certAsPEMWithLineFeed, []byte("\n")...)
		m := &etcdraftproto.ConfigMetadata{
			Consenters: []*etcdraftproto.Consenter{
				{ServerTlsCert: certAsPEMWithLineFeed},
			},
			Options: &etcdraftproto.Options{
				TickInterval:      "500ms",
				ElectionTick:      10,
				HeartbeatTick:     1,
				MaxInflightBlocks: 5,
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{
			ConsensusMetadataVal: metadata,
			BatchSizeVal:         &orderer.BatchSize{PreferredMaxBytes: 2 * 1024 * 1024},
		})

		consenter := newConsenter(chainGetter)
		consenter.EtcdRaftConfig.WALDir = walDir
		consenter.EtcdRaftConfig.SnapDir = snapDir
		// consenter.EtcdRaftConfig.EvictionSuspicion is missing
		var defaultSuspicionFallback bool
		consenter.Metrics = newFakeMetrics(newFakeMetricsFields())
		consenter.Logger = consenter.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if strings.Contains(entry.Message, "EvictionSuspicion not set, defaulting to 10m0s") {
				defaultSuspicionFallback = true
			}
			return nil
		}))

		chain, err := consenter.HandleChain(support, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(chain).NotTo(BeNil())

		Expect(chain.Start).NotTo(Panic())
		Expect(defaultSuspicionFallback).To(BeTrue())
	})

	It("fails to handle chain if no matching cert found", func() {
		m := &etcdraftproto.ConfigMetadata{
			Consenters: []*etcdraftproto.Consenter{
				{ServerTlsCert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("foo")})},
			},
			Options: &etcdraftproto.Options{
				TickInterval:      "500ms",
				ElectionTick:      10,
				HeartbeatTick:     1,
				MaxInflightBlocks: 5,
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support := &consensusmocks.FakeConsenterSupport{}
		support.SharedConfigReturns(&mockconfig.Orderer{
			ConsensusMetadataVal: metadata,
			BatchSizeVal:         &orderer.BatchSize{PreferredMaxBytes: 2 * 1024 * 1024},
		})
		support.ChainIDReturns("foo")

		consenter := newConsenter(chainGetter)

		chain, err := consenter.HandleChain(support, &common.Metadata{})
		Expect(chain).To(Not(BeNil()))
		Expect(err).To(Not(HaveOccurred()))
		Expect(chain.Order(nil, 0).Error()).To(Equal("channel foo is not serviced by me"))
		consenter.icr.AssertNumberOfCalls(testingInstance, "TrackChain", 1)
	})

	It("fails to handle chain if etcdraft options have not been provided", func() {
		m := &etcdraftproto.ConfigMetadata{
			Consenters: []*etcdraftproto.Consenter{
				{ServerTlsCert: []byte("cert.orderer1.org1")},
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{
			ConsensusMetadataVal: metadata,
			BatchSizeVal:         &orderer.BatchSize{PreferredMaxBytes: 2 * 1024 * 1024},
		})

		consenter := newConsenter(chainGetter)

		chain, err := consenter.HandleChain(support, nil)
		Expect(chain).To(BeNil())
		Expect(err).To(MatchError("etcdraft options have not been provided"))
	})

	It("fails to handle chain if tick interval is invalid", func() {
		m := &etcdraftproto.ConfigMetadata{
			Consenters: []*etcdraftproto.Consenter{
				{ServerTlsCert: certAsPEM},
			},
			Options: &etcdraftproto.Options{
				TickInterval:      "500",
				ElectionTick:      10,
				HeartbeatTick:     1,
				MaxInflightBlocks: 5,
			},
		}
		metadata := utils.MarshalOrPanic(m)
		support.SharedConfigReturns(&mockconfig.Orderer{
			ConsensusMetadataVal: metadata,
			CapabilitiesVal:      &mockconfig.OrdererCapabilities{},
			BatchSizeVal:         &orderer.BatchSize{PreferredMaxBytes: 2 * 1024 * 1024},
		})

		consenter := newConsenter(chainGetter)

		chain, err := consenter.HandleChain(support, nil)
		Expect(chain).To(BeNil())
		Expect(err).To(MatchError("failed to parse TickInterval (500) to time duration"))
	})
})

type consenter struct {
	*etcdraft.Consenter
	icr *mocks.InactiveChainRegistry
}

func newConsenter(chainGetter *mocks.ChainGetter) *consenter {
	communicator := &clustermocks.Communicator{}
	ca, err := tlsgen.NewCA()
	Expect(err).NotTo(HaveOccurred())
	communicator.On("Configure", mock.Anything, mock.Anything)
	icr := &mocks.InactiveChainRegistry{}
	icr.On("TrackChain", "foo", mock.Anything, mock.Anything)
	certAsPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("cert bytes")})
	c := &etcdraft.Consenter{
		InactiveChainRegistry: icr,
		Communication:         communicator,
		Cert:                  certAsPEM,
		Logger:                flogging.MustGetLogger("test"),
		Chains:                chainGetter,
		Dispatcher: &etcdraft.Dispatcher{
			Logger:        flogging.MustGetLogger("test"),
			ChainSelector: &mocks.ReceiverGetter{},
		},
		Dialer: &cluster.PredicateDialer{
			ClientConfig: comm.ClientConfig{
				SecOpts: &comm.SecureOptions{
					Certificate: ca.CertBytes(),
				},
			},
		},
	}
	return &consenter{
		Consenter: c,
		icr:       icr,
	}
}
