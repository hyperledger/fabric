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
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	clustermocks "github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft/mocks"
	"github.com/hyperledger/fabric/orderer/consensus/follower"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//These fixtures contain certificates for testing consenters.
//In both folders certificates generated using tlsgen pkg, each certificate is singed by ca.pem inside corresponding folder.
//Each cert has 10years expiration time (tlsgenCa.NewServerCertKeyPair("localhost")).

//NOTE ONLY FOR GO 1.15+: prior to go1.15 tlsgen produced CA root cert without SubjectKeyId, which is not allowed by MSP validator.
//In this test left tags @ONLY-GO1.15+ in places where fixtures can be replaced with tlsgen runtime generated certs once fabric moved to 1.15

const (
	consentersTestDataDir = "testdata/consenters_certs/"
	ca1Dir                = consentersTestDataDir + "ca1"
	ca2Dir                = consentersTestDataDir + "ca2"
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
		chainGetter *mocks.ChainGetter
		support     *consensusmocks.FakeConsenterSupport
		dataDir     string
		snapDir     string
		walDir      string
		mspDir      string
		tlsCA       tlsgen.CA
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
				Metadata: [][]byte{{}, protoutil.MarshalOrPanic(&common.Metadata{
					Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 0}),
				})},
			},
		}

		support.BlockReturns(lastBlock)
	})

	AfterEach(func() {
		os.RemoveAll(dataDir)
		os.RemoveAll(mspDir)
	})

	When("the consenter is extracting the channel", func() {
		It("extracts successfully from step requests", func() {
			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			ch := consenter.TargetChannel(&orderer.ConsensusRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})
		It("extracts successfully from submit requests", func() {
			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			ch := consenter.TargetChannel(&orderer.SubmitRequest{Channel: "mychannel"})
			Expect(ch).To(BeIdenticalTo("mychannel"))
		})
		It("returns an empty string for the rest of the messages", func() {
			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			ch := consenter.TargetChannel(&common.Block{})
			Expect(ch).To(BeEmpty())
		})
	})

	When("the consenter is asked for a chain", func() {
		cryptoProvider, _ := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		chainInstance := &etcdraft.Chain{CryptoProvider: cryptoProvider}
		cs := &multichannel.ChainSupport{
			Chain: chainInstance,
			BCCSP: cryptoProvider,
		}
		BeforeEach(func() {
			chainGetter.On("GetChain", "mychannel").Return(cs)
			chainGetter.On("GetChain", "badChainObject").Return(&multichannel.ChainSupport{})
			chainGetter.On("GetChain", "notmychannel").Return(nil)
			chainGetter.On("GetChain", "notraftchain").Return(&multichannel.ChainSupport{
				Chain: &multichannel.ChainSupport{},
			})
		})
		It("calls the chain manager and returns the reference when it is found", func() {
			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("mychannel")
			Expect(chain).NotTo(BeNil())
			Expect(chain).To(BeIdenticalTo(chainInstance))
		})
		It("calls the chain manager and returns nil when it's not found", func() {
			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("notmychannel")
			Expect(chain).To(BeNil())
		})
		It("calls the chain manager and returns nil when it's not a raft chain", func() {
			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			Expect(consenter).NotTo(BeNil())

			chain := consenter.ReceiverByChain("notraftchain")
			Expect(chain).To(BeNil())
		})
		It("calls the chain getter and panics when the chain has a bad internal state", func() {
			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			Expect(consenter).NotTo(BeNil())

			Expect(func() {
				consenter.ReceiverByChain("badChainObject")
			}).To(Panic())
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

		consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
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

		consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
		consenter.EtcdRaftConfig.WALDir = walDir
		consenter.EtcdRaftConfig.SnapDir = snapDir
		//without a system channel, the InactiveChainRegistry is nil
		consenter.InactiveChainRegistry = nil
		consenter.icr = nil

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

		consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)

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

		consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)

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

		consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)

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
					TickInterval:      "500s",
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

			consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
			consenter.EtcdRaftConfig.TickIntervalOverride = "seven"

			_, err := consenter.HandleChain(support, nil)
			Expect(err).To(MatchError(HavePrefix("failed parsing Consensus.TickIntervalOverride:")))
			Expect(err).To(MatchError(ContainSubstring("seven")))
		})
	})

	It("constructs a follower chain if no matching cert found", func() {
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

		consenter := newConsenter(chainGetter, tlsCA.CertBytes(), certAsPEM)
		//without a system channel, the InactiveChainRegistry is nil
		consenter.InactiveChainRegistry = nil
		consenter.icr = nil

		chain, err := consenter.HandleChain(support, &common.Metadata{})
		Expect(chain).To(Not(BeNil()))
		Expect(err).To(Not(HaveOccurred()))
		Expect(chain.Order(nil, 0).Error()).To(Equal("orderer is a follower of channel foo"))
		_, ok := chain.(*follower.Chain)
		Expect(ok).To(BeTrue())
	})
})

type consenter struct {
	*etcdraft.Consenter
	icr *mocks.InactiveChainRegistry
}

func newConsenter(chainGetter *mocks.ChainGetter, caCert, cert []byte) *consenter {
	communicator := &clustermocks.Communicator{}
	communicator.On("Configure", mock.Anything, mock.Anything)
	icr := &mocks.InactiveChainRegistry{}
	icr.On("TrackChain", "foo", mock.Anything, mock.Anything)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	Expect(err).NotTo(HaveOccurred())

	c := &etcdraft.Consenter{
		InactiveChainRegistry: icr,
		Communication:         communicator,
		Cert:                  cert,
		Logger:                flogging.MustGetLogger("test"),
		Chains:                chainGetter,
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
		icr:       icr,
	}
}

func generateCertificates(confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) [][]byte {
	certificates := [][]byte{}
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

		certificates = append(certificates, srvC.Cert)
	}

	return certificates
}
