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
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	etcdraftproto "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	clustermocks "github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft/mocks"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gtypes "github.com/onsi/gomega/types"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var certAsPEM []byte

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
		chainManager *mocks.ChainManager
		support      *consensusmocks.FakeConsenterSupport
		dataDir      string
		snapDir      string
		walDir       string
		tlsCA        tlsgen.CA
	)

	BeforeEach(func() {
		var err error
		tlsCA, err = tlsgen.NewCA()
		Expect(err).NotTo(HaveOccurred())
		kp, err := tlsCA.NewClientCertKeyPair()
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
	})

	AfterEach(func() {
		if dataDir != "" {
			os.RemoveAll(dataDir)
		}
	})

	When("the consenter is extracting the channel", func() {
		It("extracts successfully from step requests", func() {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
			ch := consenter.TargetChannel(&orderer.ConsensusRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})

		It("extracts successfully from submit requests", func() {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
			ch := consenter.TargetChannel(&orderer.SubmitRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})

		It("returns an empty string for the rest of the messages", func() {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
			ch := consenter.TargetChannel(&common.Block{})
			Expect(ch).To(BeEmpty())
		})
	})

	DescribeTable("identifies a bad block",
		func(block *common.Block, errMatcher gtypes.GomegaMatcher) {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
			isMem, err := consenter.IsChannelMember(block)
			Expect(isMem).To(BeFalse())
			Expect(err).To(errMatcher)
		},
		Entry("nil block", nil, MatchError("nil block")),
		Entry("data is nil", &common.Block{}, MatchError("block data is nil")),
		Entry("data is empty", protoutil.NewBlock(10, []byte{1, 2, 3, 4}), MatchError("envelope index out of bounds")),
		Entry("bad data",
			func() *common.Block {
				block := protoutil.NewBlock(10, []byte{1, 2, 3, 4})
				block.Data.Data = [][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}}
				return block
			}(),
			MatchError(HavePrefix("block data does not carry an envelope at index 0: error unmarshalling Envelope: proto:"))),
	)

	When("the consenter is asked about join-block membership", func() {
		var (
			mspDir          string
			memberKeyPair   *tlsgen.CertKeyPair
			genesisBlockApp *common.Block
			confAppRaft     *genesisconfig.Profile
		)

		BeforeEach(func() {
			var err error
			mspDir, err = ioutil.TempDir(dataDir, "msp")
			Expect(err).NotTo(HaveOccurred())

			confAppRaft = genesisconfig.Load(genesisconfig.SampleDevModeEtcdRaftProfile, configtest.GetDevConfigDir())
			confAppRaft.Consortiums = nil
			confAppRaft.Consortium = ""

			// IsChannelMember verifies config meta along with it's tls certs
			// ofconsenters. So when we add new conseter with tls certs, they must be
			// signed by any msp from orderer config. Consenters in this test will
			// have certificates from fixtures generated by tlsgen pkg. To pass
			// validation, root ca cert should be part of a MSP in orderer config.
			// Adding tls ca root cert to an existing ordering org's MSP definition.
			Expect(confAppRaft.Orderer).NotTo(BeNil())
			Expect(confAppRaft.Orderer.Organizations).ToNot(HaveLen(0))
			Expect(confAppRaft.Orderer.EtcdRaft.Consenters).ToNot(HaveLen(0))

			// one consenter is enough for testing
			confAppRaft.Orderer.EtcdRaft.Consenters = confAppRaft.Orderer.EtcdRaft.Consenters[:1]

			// Generate client pair using tlsCA and set it to the consenter
			memberKeyPair, err = tlsCA.NewServerCertKeyPair("127.0.0.1", "::1", "localhost")
			Expect(err).NotTo(HaveOccurred())
			consenterDir, err := ioutil.TempDir(dataDir, "consenter")
			Expect(err).NotTo(HaveOccurred())
			consenterCertPath := filepath.Join(consenterDir, "client.pem")
			err = ioutil.WriteFile(consenterCertPath, memberKeyPair.Cert, 0o644)
			Expect(err).NotTo(HaveOccurred())

			confAppRaft.Orderer.EtcdRaft.Consenters[0].ClientTlsCert = []byte(consenterCertPath)
			confAppRaft.Orderer.EtcdRaft.Consenters[0].ServerTlsCert = []byte(consenterCertPath)

			// Don't want to spoil sampleconfig, copying it to some tmp dir.
			err = testutil.CopyDir(confAppRaft.Orderer.Organizations[0].MSPDir, mspDir, true)
			Expect(err).NotTo(HaveOccurred())
			confAppRaft.Orderer.Organizations[0].MSPDir = mspDir
			confAppRaft.Orderer.Organizations[0].ID = fmt.Sprintf("SampleMSP-%d", time.Now().UnixNano())

			// Write the TLS root cert to the msp folder
			err = ioutil.WriteFile(filepath.Join(mspDir, "tlscacerts", "cert.pem"), tlsCA.CertBytes(), 0o644)
			Expect(err).NotTo(HaveOccurred())

			bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
			Expect(err).NotTo(HaveOccurred())
			genesisBlockApp = bootstrapper.GenesisBlockForChannel("my-raft-channel")
			Expect(genesisBlockApp).NotTo(BeNil())
		})

		It("identifies a member block", func() {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), memberKeyPair.Cert)
			isMem, err := consenter.IsChannelMember(genesisBlockApp)
			Expect(isMem).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())
		})

		It("identifies a non-member block", func() {
			foreignCA, err := tlsgen.NewCA()
			Expect(err).NotTo(HaveOccurred())
			nonMemberKeyPair, err := foreignCA.NewServerCertKeyPair("127.0.0.1", "::1", "localhost")
			Expect(err).NotTo(HaveOccurred())

			consenter := newConsenter(chainManager, tlsCA.CertBytes(), nonMemberKeyPair.Cert)
			isMem, err := consenter.IsChannelMember(genesisBlockApp)
			Expect(isMem).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
		})

		It("raft config has consenter with certificate that is not signed by any msp", func() {
			// This TLS root cert will not be part of the MSP.
			foreignCA, err := tlsgen.NewCA()
			Expect(err).NotTo(HaveOccurred())
			foreignKeyPair, err := foreignCA.NewServerCertKeyPair("127.0.0.1", "::1", "localhost")
			Expect(err).NotTo(HaveOccurred())

			consenterDir, err := ioutil.TempDir(dataDir, "foreign-consenter")
			Expect(err).NotTo(HaveOccurred())
			foreignConsenterCertPath := filepath.Join(consenterDir, "client.pem")
			err = ioutil.WriteFile(foreignConsenterCertPath, foreignKeyPair.Cert, 0o644)
			Expect(err).NotTo(HaveOccurred())

			confAppRaft.Orderer.EtcdRaft.Consenters[0].ClientTlsCert = []byte(foreignConsenterCertPath)
			confAppRaft.Orderer.EtcdRaft.Consenters[0].ServerTlsCert = []byte(foreignConsenterCertPath)

			consenter := newConsenter(chainManager, foreignCA.CertBytes(), foreignKeyPair.Cert)

			bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
			Expect(err).NotTo(HaveOccurred())
			genesisBlockApp = bootstrapper.GenesisBlockForChannel("my-raft-channel")
			Expect(genesisBlockApp).NotTo(BeNil())

			isMem, err := consenter.IsChannelMember(genesisBlockApp)
			Expect(isMem).To(BeFalse())
			Expect(err).To(HaveOccurred())
		})
	})

	When("the consenter is asked for a chain", func() {
		cryptoProvider, _ := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		chainInstance := &etcdraft.Chain{CryptoProvider: cryptoProvider}
		BeforeEach(func() {
			chainManager.GetConsensusChainStub = func(channel string) consensus.Chain {
				switch channel {
				case "mychannel":
					return chainInstance
				default:
					return nil
				}
			}
		})

		It("calls the chain manager and returns the reference when it is found", func() {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("mychannel")
			Expect(chain).NotTo(BeNil())
			Expect(chain).To(BeIdenticalTo(chainInstance))
		})

		It("calls the chain manager and returns nil when it's not found", func() {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("notmychannel")
			Expect(chain).To(BeNil())
		})

		It("calls the chain manager and returns nil when it's not a raft chain", func() {
			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
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

		consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
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
			if strings.Contains(entry.Message, "After eviction from the cluster Registrar.SwitchToFollower will be called, and the orderer will become a follower of the channel.") {
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

		consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
		consenter.EtcdRaftConfig.WALDir = walDir
		consenter.EtcdRaftConfig.SnapDir = snapDir

		// consenter.EtcdRaftConfig.EvictionSuspicion is missing
		var defaultSuspicionFallback bool
		var switchToFollowerCallback bool
		consenter.Metrics = newFakeMetrics(newFakeMetricsFields())
		consenter.Logger = consenter.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if strings.Contains(entry.Message, "EvictionSuspicion not set, defaulting to 10m0s") {
				defaultSuspicionFallback = true
			}
			if strings.Contains(entry.Message, "After eviction from the cluster Registrar.SwitchToFollower will be called, and the orderer will become a follower of the channel.") {
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

		consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)

		chain, err := consenter.HandleChain(support, &common.Metadata{})
		Expect(chain).To((BeNil()))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("without a system channel, a follower should have been created"))
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

		consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)

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

		consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)

		chain, err := consenter.HandleChain(support, nil)
		Expect(chain).To(BeNil())
		Expect(err).To(MatchError("failed to parse TickInterval (500) to time duration"))
	})

	When("the TickIntervalOverride is invalid", func() {
		It("returns an error", func() {
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

			consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)
			consenter.EtcdRaftConfig.TickIntervalOverride = "seven"

			_, err := consenter.HandleChain(support, nil)
			Expect(err).To(MatchError(HavePrefix("failed parsing Consensus.TickIntervalOverride:")))
			Expect(err).To(MatchError(ContainSubstring("seven")))
		})
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

		consenter := newConsenter(chainManager, tlsCA.CertBytes(), certAsPEM)

		chain, err := consenter.HandleChain(support, &common.Metadata{})
		Expect(chain).To((BeNil()))
		Expect(err).To(MatchError("without a system channel, a follower should have been created: not in the channel"))
	})
})

type consenter struct {
	*etcdraft.Consenter
}

func newConsenter(chainManager *mocks.ChainManager, caCert, cert []byte) *consenter {
	communicator := &clustermocks.Communicator{}
	communicator.On("Configure", mock.Anything, mock.Anything)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	Expect(err).NotTo(HaveOccurred())

	c := &etcdraft.Consenter{
		ChainManager:  chainManager,
		Communication: communicator,
		Cert:          cert,
		Logger:        flogging.MustGetLogger("test"),
		Dispatcher: &etcdraft.Dispatcher{
			Logger:        flogging.MustGetLogger("test"),
			ChainSelector: &mocks.ReceiverGetter{},
		},
		Dialer: &cluster.PredicateDialer{
			Config: comm.ClientConfig{
				SecOpts: comm.SecureOptions{
					Certificate: caCert,
				},
			},
		},
		BCCSP: cryptoProvider,
	}
	return &consenter{
		Consenter: c,
	}
}
