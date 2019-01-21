/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache", func() {
	var (
		c            *lifecycle.Cache
		l            *lifecycle.Lifecycle
		chaincodeMap map[string]*lifecycle.CachedChaincodeDefinition
	)

	BeforeEach(func() {
		l = &lifecycle.Lifecycle{
			Serializer: &lifecycle.Serializer{},
		}
		c = lifecycle.NewCache(l)

		chaincodeMap = map[string]*lifecycle.CachedChaincodeDefinition{
			"chaincode-name": {
				Definition: &lifecycle.ChaincodeDefinition{
					Sequence: 3,
				},
			},
		}
		lifecycle.SetChaincodeMap(c, "channel-id", chaincodeMap)
	})

	Describe("ChaincodeDefinition", func() {
		It("returns the cached chaincode definition", func() {
			cachedDefinition, err := c.ChaincodeDefinition("channel-id", "chaincode-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(cachedDefinition).To(Equal(&lifecycle.ChaincodeDefinition{
				Sequence: 3,
			}))
		})

		Context("when the chaincode has no cache", func() {
			It("returns an error", func() {
				_, err := c.ChaincodeDefinition("channel-id", "missing-name")
				Expect(err).To(MatchError("unknown chaincode 'missing-name' for channel 'channel-id'"))
			})
		})

		Context("when the channel does not exist", func() {
			It("returns an error", func() {
				_, err := c.ChaincodeDefinition("missing-channel-id", "missing-name")
				Expect(err).To(MatchError("unknown channel 'missing-channel-id'"))
			})
		})
	})

	Describe("Update", func() {
		var (
			fakePublicState   MapLedgerShim
			fakeQueryExecutor *mock.SimpleQueryExecutor
			dirtyChaincodes   map[string]struct{}
		)

		BeforeEach(func() {
			fakePublicState = MapLedgerShim(map[string][]byte{})
			fakeQueryExecutor = &mock.SimpleQueryExecutor{}
			fakeQueryExecutor.GetStateStub = func(namespace, key string) ([]byte, error) {
				return fakePublicState.GetState(key)
			}

			err := l.Serializer.Serialize(lifecycle.NamespacesName, "chaincode-name", &lifecycle.ChaincodeDefinition{
				Sequence: 7,
			}, fakePublicState)
			Expect(err).NotTo(HaveOccurred())

			dirtyChaincodes = map[string]struct{}{"chaincode-name": {}}
		})

		It("updates the dirty definition from the state", func() {
			err := c.Update("channel-id", dirtyChaincodes, fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			Expect(chaincodeMap["chaincode-name"].Definition.Sequence).To(Equal(int64(7)))
		})

		Context("when the definition is not in the new state", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, nil)
			})

			It("deletes the cached definition", func() {
				err := c.Update("channel-id", dirtyChaincodes, fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(chaincodeMap["chaincode-name"]).To(BeNil())
			})
		})

		Context("when the update is for an unknown channel", func() {
			It("creates the underlying map", func() {
				Expect(lifecycle.GetChaincodeMap(c, "new-channel")).To(BeNil())
				err := c.Update("new-channel", dirtyChaincodes, fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(lifecycle.GetChaincodeMap(c, "new-channel")).NotTo(BeNil())
			})
		})

		Context("when there are no dirty chaincodes", func() {
			BeforeEach(func() {
				dirtyChaincodes = map[string]struct{}{}
			})

			It("does nothing", func() {
				err := c.Update("channel-id", dirtyChaincodes, fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the state returns an error", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				err := c.Update("channel-id", dirtyChaincodes, fakeQueryExecutor)
				Expect(err).To(MatchError("could not get chaincode definition for 'chaincode-name' on channel 'channel-id': could not deserialize metadata for chaincode chaincode-name: could not query metadata for namespace namespaces/chaincode-name: get-state-error"))
			})
		})
	})

})
