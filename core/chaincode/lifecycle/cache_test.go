/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache", func() {
	var (
		c            *lifecycle.Cache
		l            *lifecycle.Lifecycle
		channelCache *lifecycle.ChannelCache
	)

	BeforeEach(func() {
		l = &lifecycle.Lifecycle{
			Serializer: &lifecycle.Serializer{},
		}
		c = lifecycle.NewCache(l, "my-mspid")

		channelCache = &lifecycle.ChannelCache{
			Chaincodes: map[string]*lifecycle.CachedChaincodeDefinition{
				"chaincode-name": {
					Definition: &lifecycle.ChaincodeDefinition{
						Sequence: 3,
					},
					Approved: true,
					Hashes: []string{
						string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#3"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Sequence"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/ValidationInfo"))),
						string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Collections"))),
					},
				},
			},
			InterestingHashes: map[string]string{
				string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#3"))):               "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Sequence"))):        "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/EndorsementInfo"))): "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/ValidationInfo"))):  "chaincode-name",
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#3/Collections"))):     "chaincode-name",
			},
		}

		lifecycle.SetChaincodeMap(c, "channel-id", channelCache)
	})

	Describe("ChaincodeDefinition", func() {
		It("returns the cached chaincode definition", func() {
			cachedDefinition, approved, err := c.ChaincodeDefinition("channel-id", "chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(cachedDefinition).To(Equal(&lifecycle.ChaincodeDefinition{
				Sequence: 3,
			}))
			Expect(approved).To(BeTrue())
		})

		Context("when the chaincode has no cache", func() {
			It("returns an error", func() {
				_, _, err := c.ChaincodeDefinition("channel-id", "missing-name")
				Expect(err).To(MatchError("unknown chaincode 'missing-name' for channel 'channel-id'"))
			})
		})

		Context("when the channel does not exist", func() {
			It("returns an error", func() {
				_, _, err := c.ChaincodeDefinition("missing-channel-id", "missing-name")
				Expect(err).To(MatchError("unknown channel 'missing-channel-id'"))
			})
		})
	})

	Describe("Initialize", func() {
		var (
			fakePublicState   MapLedgerShim
			fakePrivateState  MapLedgerShim
			fakeQueryExecutor *mock.SimpleQueryExecutor
		)

		BeforeEach(func() {
			fakePublicState = MapLedgerShim(map[string][]byte{})
			fakePrivateState = MapLedgerShim(map[string][]byte{})
			fakeQueryExecutor = &mock.SimpleQueryExecutor{}
			fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
				return fakePublicState.GetState(key)
			}

			fakeQueryExecutor.GetStateRangeScanIteratorStub = func(namespace, begin, end string) (commonledger.ResultsIterator, error) {
				fakeResultsIterator := &mock.ResultsIterator{}
				i := 0
				for key, value := range fakePublicState {
					if key >= begin && key < end {
						fakeResultsIterator.NextReturnsOnCall(i, &queryresult.KV{
							Key:   key,
							Value: value,
						}, nil)
						i++
					}
				}
				return fakeResultsIterator, nil
			}

			fakeQueryExecutor.GetPrivateDataHashStub = func(namespace, collection, key string) ([]byte, error) {
				return fakePrivateState.GetStateHash(key)
			}

			err := l.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeDefinition{
				Sequence: 7,
			}, fakePublicState)
			Expect(err).NotTo(HaveOccurred())

			err = l.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name#7", &lifecycle.ChaincodeParameters{}, fakePrivateState)
			Expect(err).NotTo(HaveOccurred())
		})

		It("sets the definitions from the state", func() {
			err := c.Initialize("channel-id", fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			Expect(channelCache.Chaincodes["chaincode-name"].Definition.Sequence).To(Equal(int64(7)))
			Expect(channelCache.Chaincodes["chaincode-name"].Approved).To(BeTrue())
			Expect(channelCache.Chaincodes["chaincode-name"].Hashes).To(Equal([]string{
				string(util.ComputeSHA256([]byte("namespaces/metadata/chaincode-name#7"))),
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#7/EndorsementInfo"))),
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#7/ValidationInfo"))),
				string(util.ComputeSHA256([]byte("namespaces/fields/chaincode-name#7/Collections"))),
			}))
			for _, hash := range channelCache.Chaincodes["chaincode-name"].Hashes {
				Expect(channelCache.InterestingHashes[hash]).To(Equal("chaincode-name"))
			}
		})

		Context("when the namespaces query fails", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateRangeScanIteratorReturns(nil, fmt.Errorf("range-error"))
			})

			It("wraps and returns the error", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).To(MatchError("could not query namespace definitions: could not query namespace metadata: could not get state range for namespace namespaces: could not get state iterator: range-error"))
			})
		})

		Context("when the namespace is not of type chaincode", func() {
			BeforeEach(func() {
				err := l.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeParameters{}, fakePublicState)
				Expect(err).NotTo(HaveOccurred())
			})

			It("ignores the definition", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the definition is not in the new state", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, nil)
			})

			It("deletes the cached definition", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"]).To(BeNil())
			})
		})

		Context("when the org's definition differs", func() {
			BeforeEach(func() {
				err := l.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name#7", &lifecycle.ChaincodeParameters{
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						EndorsementPlugin: "different",
					},
				}, fakePrivateState)
				Expect(err).NotTo(HaveOccurred())
			})

			It("does not mark the definition approved", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(channelCache.Chaincodes["chaincode-name"].Approved).To(BeFalse())
			})

			Context("when the org has no definition", func() {
				BeforeEach(func() {
					fakeQueryExecutor.GetPrivateDataHashReturns(nil, nil)
				})

				It("does not mark the definition approved", func() {
					err := c.Initialize("channel-id", fakeQueryExecutor)
					Expect(err).NotTo(HaveOccurred())
					Expect(channelCache.Chaincodes["chaincode-name"].Approved).To(BeFalse())
				})
			})
		})

		Context("when the update is for an unknown channel", func() {
			It("creates the underlying map", func() {
				Expect(lifecycle.GetChaincodeMap(c, "new-channel")).To(BeNil())
				err := c.Initialize("new-channel", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(lifecycle.GetChaincodeMap(c, "new-channel")).NotTo(BeNil())
			})
		})

		Context("when the state returns an error", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).To(MatchError("could not get chaincode definition for 'chaincode-name' on channel 'channel-id': could not deserialize metadata for chaincode chaincode-name: could not query metadata for namespace namespaces/chaincode-name: get-state-error"))
			})
		})

		Context("when the private state returns an error", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetPrivateDataHashReturns(nil, fmt.Errorf("private-data-error"))
			})

			It("wraps and returns the error", func() {
				err := c.Initialize("channel-id", fakeQueryExecutor)
				Expect(err).To(MatchError("could not check opaque org state for 'chaincode-name' on channel 'channel-id': could not get value for key namespaces/metadata/chaincode-name#7: private-data-error"))
			})
		})
	})
})
