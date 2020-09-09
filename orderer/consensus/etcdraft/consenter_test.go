/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	etcdraftproto "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	clustermocks "github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft/mocks"
	"github.com/hyperledger/fabric/orderer/consensus/inactive"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	certAsPEM []byte
)

//go:generate counterfeiter -o mocks/orderer_capabilities.go --fake-name OrdererCapabilities . ordererCapabilities

type ordererCapabilities interface {
	channelconfig.OrdererCapabilities
}

//go:generate counterfeiter -o mocks/orderer_config.go --fake-name OrdererConfig . ordererConfig

type ordererConfig interface {
	channelconfig.Orderer
}

var _ = Describe("Consenter", func() {
	var (
		chainManager       *mocks.ChainManager
		support            *consensusmocks.FakeConsenterSupport
		dataDir            string
		snapDir            string
		walDir             string
		genesisBlockApp    *common.Block
		serverCertificates [][]byte
	)

	BeforeEach(func() {
		ca, err := tlsgen.NewCA()
		Expect(err).NotTo(HaveOccurred())
		kp, err := ca.NewClientCertKeyPair()
		Expect(err).NotTo(HaveOccurred())
		if certAsPEM == nil {
			certAsPEM = kp.Cert
		}
		chainManager = &mocks.ChainManager{}
		support = &consensusmocks.FakeConsenterSupport{}
		dataDir, err = ioutil.TempDir("", "consenter-")
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
				Metadata: [][]byte{{}, protoutil.MarshalOrPanic(&common.Metadata{
					Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 0}),
				})},
			},
		}

		support.BlockReturns(lastBlock)

		serverCertificates = nil
		genesisBlockApp = nil
	})

	AfterEach(func() {
		os.RemoveAll(dataDir)
	})

	When("the consenter is extracting the channel", func() {
		It("extracts successfully from step requests", func() {
			consenter := newConsenter(chainManager)
			ch := consenter.TargetChannel(&orderer.ConsensusRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})
		It("extracts successfully from submit requests", func() {
			consenter := newConsenter(chainManager)
			ch := consenter.TargetChannel(&orderer.SubmitRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})
		It("returns an empty string for the rest of the messages", func() {
			consenter := newConsenter(chainManager)
			ch := consenter.TargetChannel(&common.Block{})
			Expect(ch).To(BeEmpty())
		})
	})

	When("the consenter is asked about join-block membership", func() {
		table.DescribeTable("identifies a bad block",
			func(block *common.Block, errExpected string) {
				consenter := newConsenter(chainManager)
				isMem, err := consenter.IsChannelMember(block)
				Expect(isMem).To(BeFalse())
				Expect(err).To(MatchError(errExpected))
			},
			table.Entry("nil block", nil, "nil block"),
			table.Entry("data is nil", &common.Block{}, "block data is nil"),
			table.Entry("data is empty", protoutil.NewBlock(10, []byte{1, 2, 3, 4}), "envelope index out of bounds"),
			table.Entry("bad data",
				func() *common.Block {
					block := protoutil.NewBlock(10, []byte{1, 2, 3, 4})
					block.Data.Data = [][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}}
					return block
				}(),
				"block data does not carry an envelope at index 0: error unmarshaling Envelope: proto: common.Envelope: illegal tag 0 (wire type 1)"),
		)

		BeforeEach(func() {
			tlsCA, _ := tlsgen.NewCA()
			confAppRaft := genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
			confAppRaft.Consortiums = nil
			confAppRaft.Consortium = ""
			serverCertificates = generateCertificates(confAppRaft, tlsCA, dataDir)
			bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
			Expect(err).NotTo(HaveOccurred())
			genesisBlockApp = bootstrapper.GenesisBlockForChannel("my-raft-channel")
			Expect(genesisBlockApp).NotTo(BeNil())
		})

		It("identifies a member block", func() {
			consenter := newConsenter(chainManager)
			for i := 0; i < len(serverCertificates); i++ {
				consenter.Cert = serverCertificates[i]
				isMem, err := consenter.IsChannelMember(genesisBlockApp)
				Expect(isMem).To(BeTrue())
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("identifies a non-member block", func() {
			consenter := newConsenter(chainManager)
			consenter.Cert = certAsPEM
			isMem, err := consenter.IsChannelMember(genesisBlockApp)
			Expect(isMem).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("the consenter is asked for a chain", func() {
		cryptoProvider, _ := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		chainInstance := &etcdraft.Chain{CryptoProvider: cryptoProvider}
		BeforeEach(func() {
			chainManager.On("GetConsensusChain", "mychannel").Return(chainInstance)
			chainManager.On("GetConsensusChain", "notmychannel").Return(nil)
			chainManager.On("GetConsensusChain", "notraftchain").Return(&inactive.Chain{Err: errors.New("not a raft chain")})
		})
		It("calls the chain manager and returns the reference when it is found", func() {
			consenter := newConsenter(chainManager)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("mychannel")
			Expect(chain).NotTo(BeNil())
			Expect(chain).To(BeIdenticalTo(chainInstance))
		})
		It("calls the chain manager and returns nil when it's not found", func() {
			consenter := newConsenter(chainManager)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("notmychannel")
			Expect(chain).To(BeNil())
		})
		It("calls the chain manager and returns nil when it's not a raft chain", func() {
			consenter := newConsenter(chainManager)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("notraftchain")
			Expect(chain).To(BeNil())
		})
	})

	It("successfully constructs a Chain", func() {
		certAsPEMWithLineFeed := certAsPEM
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
		metadata := protoutil.MarshalOrPanic(m)
		mockOrderer := &mocks.OrdererConfig{}
		mockOrderer.ConsensusMetadataReturns(metadata)
		mockOrderer.BatchSizeReturns(
			&orderer.BatchSize{
				PreferredMaxBytes: 2 * 1024 * 1024,
			},
		)
		support.SharedConfigReturns(mockOrderer)

		consenter := newConsenter(chainManager)
		consenter.EtcdRaftConfig.WALDir = walDir
		consenter.EtcdRaftConfig.SnapDir = snapDir
		// consenter.EtcdRaftConfig.EvictionSuspicion is missing
		var defaultSuspicionFallback bool
		var trackChainCallback bool
		consenter.Metrics = newFakeMetrics(newFakeMetricsFields())
		consenter.Logger = consenter.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if strings.Contains(entry.Message, "EvictionSuspicion not set, defaulting to 10m0s") {
				defaultSuspicionFallback = true
			}
			if strings.Contains(entry.Message, "With system channel: after eviction InactiveChainRegistry.TrackChain will be called") {
				trackChainCallback = true
			}
			return nil
		}))

		chain, err := consenter.HandleChain(support, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(chain).NotTo(BeNil())

		Expect(chain.Start).NotTo(Panic())
		Expect(defaultSuspicionFallback).To(BeTrue())
		Expect(trackChainCallback).To(BeTrue())
	})

	It("successfully constructs a Chain without a system channel", func() {
		// We append a line feed to our cert, just to ensure that we can still consume it and ignore.
		certAsPEMWithLineFeed := certAsPEM
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
		metadata := protoutil.MarshalOrPanic(m)
		mockOrderer := &mocks.OrdererConfig{}
		mockOrderer.ConsensusMetadataReturns(metadata)
		mockOrderer.BatchSizeReturns(
			&orderer.BatchSize{
				PreferredMaxBytes: 2 * 1024 * 1024,
			},
		)
		support.SharedConfigReturns(mockOrderer)

		consenter := newConsenter(chainManager)
		consenter.EtcdRaftConfig.WALDir = walDir
		consenter.EtcdRaftConfig.SnapDir = snapDir
		//without a system channel, the InactiveChainRegistry is nil
		consenter.InactiveChainRegistry = nil
		consenter.icr = nil

		// consenter.EtcdRaftConfig.EvictionSuspicion is missing
		var defaultSuspicionFallback bool
		var switchToFollowerCallback bool
		consenter.Metrics = newFakeMetrics(newFakeMetricsFields())
		consenter.Logger = consenter.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if strings.Contains(entry.Message, "EvictionSuspicion not set, defaulting to 10m0s") {
				defaultSuspicionFallback = true
			}
			if strings.Contains(entry.Message, "Without system channel: after eviction Registrar.SwitchToFollower will be called") {
				switchToFollowerCallback = true
			}
			return nil
		}))

		chain, err := consenter.HandleChain(support, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(chain).NotTo(BeNil())

		Expect(chain.Start).NotTo(Panic())
		Expect(defaultSuspicionFallback).To(BeTrue())
		Expect(switchToFollowerCallback).To(BeTrue())
		Expect(chain.Halt).NotTo(Panic())
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
		metadata := protoutil.MarshalOrPanic(m)
		support := &consensusmocks.FakeConsenterSupport{}
		mockOrderer := &mocks.OrdererConfig{}
		mockOrderer.ConsensusMetadataReturns(metadata)
		mockOrderer.BatchSizeReturns(
			&orderer.BatchSize{
				PreferredMaxBytes: 2 * 1024 * 1024,
			},
		)
		support.SharedConfigReturns(mockOrderer)
		support.ChannelIDReturns("foo")

		consenter := newConsenter(chainManager)

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
		metadata := protoutil.MarshalOrPanic(m)
		mockOrderer := &mocks.OrdererConfig{}
		mockOrderer.ConsensusMetadataReturns(metadata)
		mockOrderer.BatchSizeReturns(
			&orderer.BatchSize{
				PreferredMaxBytes: 2 * 1024 * 1024,
			},
		)
		support.SharedConfigReturns(mockOrderer)

		consenter := newConsenter(chainManager)

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
		metadata := protoutil.MarshalOrPanic(m)
		mockOrderer := &mocks.OrdererConfig{}
		mockOrderer.ConsensusMetadataReturns(metadata)
		mockOrderer.BatchSizeReturns(
			&orderer.BatchSize{
				PreferredMaxBytes: 2 * 1024 * 1024,
			},
		)
		mockOrderer.CapabilitiesReturns(&mocks.OrdererCapabilities{})
		support.SharedConfigReturns(mockOrderer)

		consenter := newConsenter(chainManager)

		chain, err := consenter.HandleChain(support, nil)
		Expect(chain).To(BeNil())
		Expect(err).To(MatchError("failed to parse TickInterval (500) to time duration"))
	})

	It("returns an error if no matching cert found", func() {
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
		metadata := protoutil.MarshalOrPanic(m)
		support := &consensusmocks.FakeConsenterSupport{}
		mockOrderer := &mocks.OrdererConfig{}
		mockOrderer.ConsensusMetadataReturns(metadata)
		mockOrderer.BatchSizeReturns(
			&orderer.BatchSize{
				PreferredMaxBytes: 2 * 1024 * 1024,
			},
		)
		support.SharedConfigReturns(mockOrderer)
		support.ChannelIDReturns("foo")

		consenter := newConsenter(chainManager)
		//without a system channel, the InactiveChainRegistry is nil
		consenter.InactiveChainRegistry = nil
		consenter.icr = nil

		chain, err := consenter.HandleChain(support, &common.Metadata{})
		Expect(chain).To((BeNil()))
		Expect(err).To(MatchError("without a system channel, a follower should have been created: not in the channel"))
	})
})

type consenter struct {
	*etcdraft.Consenter
	icr *mocks.InactiveChainRegistry
}

func newConsenter(chainManager *mocks.ChainManager) *consenter {
	communicator := &clustermocks.Communicator{}
	ca, err := tlsgen.NewCA()
	Expect(err).NotTo(HaveOccurred())
	communicator.On("Configure", mock.Anything, mock.Anything)
	icr := &mocks.InactiveChainRegistry{}
	icr.On("TrackChain", "foo", mock.Anything, mock.Anything)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	Expect(err).NotTo(HaveOccurred())

	c := &etcdraft.Consenter{
		ChainManager:          chainManager,
		InactiveChainRegistry: icr,
		Communication:         communicator,
		Cert:                  certAsPEM,
		Logger:                flogging.MustGetLogger("test"),
		Dispatcher: &etcdraft.Dispatcher{
			Logger:        flogging.MustGetLogger("test"),
			ChainSelector: &mocks.ReceiverGetter{},
		},
		Dialer: &cluster.PredicateDialer{
			Config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate: ca.CertBytes(),
				},
			},
		},
		BCCSP: cryptoProvider,
	}
	return &consenter{
		Consenter: c,
		icr:       icr,
	}
}

func generateCertificates(confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) [][]byte {
	certificats := [][]byte{}
	for i, c := range confAppRaft.Orderer.EtcdRaft.Consenters {
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		Expect(err).NotTo(HaveOccurred())
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = ioutil.WriteFile(srvP, srvC.Cert, 0644)
		Expect(err).NotTo(HaveOccurred())

		clnC, err := tlsCA.NewClientCertKeyPair()
		Expect(err).NotTo(HaveOccurred())
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = ioutil.WriteFile(clnP, clnC.Cert, 0644)
		Expect(err).NotTo(HaveOccurred())

		c.ServerTlsCert = []byte(srvP)
		c.ClientTlsCert = []byte(clnP)

		certificats = append(certificats, srvC.Cert)
	}
	return certificats
}
