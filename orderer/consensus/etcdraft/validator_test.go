/*
 Copyright IBM Corp All Rights Reserved.

 SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/proto"
	etcdraftproto "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	raftprotos "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	consensusmocks "github.com/hyperledger/fabric/orderer/consensus/mocks"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metadata Validation", func() {
	var (
		chain             *etcdraft.Chain
		tlsCA             tlsgen.CA
		channelID         string
		consenterMetadata *raftprotos.ConfigMetadata
		consenters        map[uint64]*raftprotos.Consenter
		support           *consensusmocks.FakeConsenterSupport
		dataDir           string
		err               error
		cryptoProvider    bccsp.BCCSP
		meta              *raftprotos.BlockMetadata
	)

	BeforeEach(func() {
		channelID = "test-channel"

		cryptoProvider, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		Expect(err).NotTo(HaveOccurred())

		dataDir, err = ioutil.TempDir("", "wal-")
		Expect(err).NotTo(HaveOccurred())

		tlsCA, err = tlsgen.NewCA()
		Expect(err).NotTo(HaveOccurred())

		support = &consensusmocks.FakeConsenterSupport{}
		support.ChannelIDReturns(channelID)
		consenterMetadata = createMetadata(3, tlsCA)
		mockOrdererConfig := mockOrdererWithTLSRootCert(time.Hour, marshalOrPanic(consenterMetadata), tlsCA)
		support.SharedConfigReturns(mockOrdererConfig)

		meta = &raftprotos.BlockMetadata{
			ConsenterIds:    make([]uint64, len(consenterMetadata.Consenters)),
			NextConsenterId: 1,
		}

		for i := range meta.ConsenterIds {
			meta.ConsenterIds[i] = meta.NextConsenterId
			meta.NextConsenterId++
		}

		consenters = map[uint64]*raftprotos.Consenter{}
		for i, c := range consenterMetadata.Consenters {
			consenters[meta.ConsenterIds[i]] = c
		}
	})

	JustBeforeEach(func() {
		c := newChain(10*time.Second, channelID, dataDir, 1, meta, consenters, cryptoProvider, support)
		c.init()
		chain = c.Chain
		chain.ActiveNodes.Store([]uint64{1, 2, 3})
	})

	When("determining parameter well-formedness", func() {
		It("succeeds when new consensus metadata is nil", func() {
			Expect(chain.ValidateConsensusMetadata(nil, nil, false)).To(Succeed())
		})

		It("fails when new consensus metadata is not nil while old consensus metadata is nil", func() {
			Expect(func() {
				chain.ValidateConsensusMetadata(nil, []byte("test"), false)
			}).To(Panic())
		})

		It("fails when old consensus metadata is not well-formed", func() {
			Expect(func() {
				chain.ValidateConsensusMetadata([]byte("test"), []byte("test"), false)
			}).To(Panic())
		})

		It("fails when new consensus metadata is not well-formed", func() {
			oldBytes, _ := proto.Marshal(&etcdraftproto.ConfigMetadata{})
			Expect(chain.ValidateConsensusMetadata(oldBytes, []byte("test"), false)).NotTo(Succeed())
		})
	})

	Context("valid old consensus metadata", func() {
		var (
			oldBytes    []byte
			oldMetadata *etcdraftproto.ConfigMetadata
			newMetadata *etcdraftproto.ConfigMetadata
			newChannel  bool
		)

		BeforeEach(func() {
			oldMetadata = &etcdraftproto.ConfigMetadata{
				Options: &etcdraftproto.Options{
					TickInterval:         "500ms",
					ElectionTick:         10,
					HeartbeatTick:        1,
					MaxInflightBlocks:    5,
					SnapshotIntervalSize: 20 * 1024 * 1024, // 20 MB
				},
				Consenters: []*etcdraftproto.Consenter{
					{
						Host:          "host1",
						Port:          10001,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
					{
						Host:          "host2",
						Port:          10002,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
					{
						Host:          "host3",
						Port:          10003,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
				},
			}
			newMetadata = oldMetadata
			oldBytes, _ = proto.Marshal(oldMetadata)
			newChannel = false
		})

		It("fails when new consensus metadata has invalid options", func() {
			// NOTE: we are not checking all failures here since tests for CheckConfigMetadata does that
			newMetadata.Options.TickInterval = ""
			newBytes, _ := proto.Marshal(newMetadata)
			Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).NotTo(Succeed())
		})

		Context("new channel creation", func() {
			BeforeEach(func() {
				newChannel = true
			})

			It("fails when the new consenters are an empty set", func() {
				newMetadata.Consenters = []*etcdraftproto.Consenter{}
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).NotTo(Succeed())
			})

			It("succeeds when the new consenters are the same as the existing consenters", func() {
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
			})

			It("succeeds when the new consenters are a subset of the existing consenters", func() {
				newMetadata.Consenters = newMetadata.Consenters[:2]
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
			})

			It("fails when the new consenters are not a subset of the existing consenters", func() {
				newMetadata.Consenters[2].ClientTlsCert = clientTLSCert(tlsCA)
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).NotTo(Succeed())
			})
		})

		Context("config update on a channel", func() {
			BeforeEach(func() {
				newChannel = false
				chain.ActiveNodes.Store([]uint64{1, 2, 3})
			})

			It("fails when the new consenters are an empty set", func() {
				// NOTE: This also takes care of the case when we remove node from a singleton consenter set
				newMetadata.Consenters = []*etcdraftproto.Consenter{}
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).NotTo(Succeed())
			})

			It("succeeds when the new consenters are the same as the existing consenters", func() {
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
			})

			It("succeeds on addition of a single consenter", func() {
				newMetadata.Consenters = append(newMetadata.Consenters, &etcdraftproto.Consenter{
					Host:          "host4",
					Port:          10004,
					ClientTlsCert: clientTLSCert(tlsCA),
					ServerTlsCert: serverTLSCert(tlsCA),
				})
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
			})

			It("fails on addition of more than one consenter", func() {
				newMetadata.Consenters = append(newMetadata.Consenters,
					&etcdraftproto.Consenter{
						Host:          "host4",
						Port:          10004,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
					&etcdraftproto.Consenter{
						Host:          "host5",
						Port:          10005,
						ClientTlsCert: clientTLSCert(tlsCA),
						ServerTlsCert: serverTLSCert(tlsCA),
					},
				)
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).NotTo(Succeed())
			})

			It("succeeds on removal of a single consenter", func() {
				newMetadata.Consenters = newMetadata.Consenters[:2]
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
			})

			It("fails on removal of more than one consenter", func() {
				newMetadata.Consenters = newMetadata.Consenters[:1]
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).NotTo(Succeed())
			})

			It("succeeds on rotating certs in case of both addition and removal of a node each to reuse the raft NodeId", func() {
				newMetadata.Consenters = append(newMetadata.Consenters[:2], &etcdraftproto.Consenter{
					Host:          "host4",
					Port:          10004,
					ClientTlsCert: clientTLSCert(tlsCA),
					ServerTlsCert: serverTLSCert(tlsCA),
				})
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
			})

			It("succeeds on removal of inactive node in 2/3 cluster", func() {
				chain.ActiveNodes.Store([]uint64{1, 2})
				newMetadata.Consenters = newMetadata.Consenters[:2]
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
			})

			It("fails on removal of active node in 2/3 cluster", func() {
				chain.ActiveNodes.Store([]uint64{1, 2})
				newMetadata.Consenters = newMetadata.Consenters[1:]
				newBytes, _ := proto.Marshal(newMetadata)
				Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(
					MatchError("2 out of 3 nodes are alive, configuration will result in quorum loss"))
			})

			When("node id starts from 2", func() {
				// this test case is mainly to assure that validator does NOT assume node ID to
				// always start from 1. Consider following case:
				// - we have [2, 3, 4] in consenter set
				// - 4 is inactive and subject to remove, which should NOT result in quorum loss
				// - if validator assumes node to start from 1, it would incorrectly conclude that
				//   node 3 is to be removed, therefore rejecting such request
				// - instead, validator should recognize that 4 is the one to be removed, which is
				//   not harmful to network and happily allows it

				BeforeEach(func() {
					meta = &raftprotos.BlockMetadata{
						ConsenterIds:    make([]uint64, len(consenterMetadata.Consenters)),
						NextConsenterId: 2, // id starts from 2
					}

					for i := range meta.ConsenterIds {
						meta.ConsenterIds[i] = meta.NextConsenterId
						meta.NextConsenterId++
					}

					consenters = map[uint64]*raftprotos.Consenter{}
					for i, c := range consenterMetadata.Consenters {
						consenters[meta.ConsenterIds[i]] = c
					}
				})

				It("succeeds on removal of inactive node in 2/3 cluster", func() {
					chain.ActiveNodes.Store([]uint64{2, 3}) // 4 is inactive
					newMetadata.Consenters = newMetadata.Consenters[:2]
					newBytes, _ := proto.Marshal(newMetadata)
					Expect(chain.ValidateConsensusMetadata(oldBytes, newBytes, newChannel)).To(Succeed())
				})
			})
		})
	})
})
