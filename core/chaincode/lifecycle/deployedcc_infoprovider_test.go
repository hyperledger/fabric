/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lifecycle", func() {
	Describe("DeployedCCInfoProvider", func() {
		var (
			l                  *lifecycle.Lifecycle
			fakeLegacyProvider *mock.LegacyDeployedCCInfoProvider
			fakeQueryExecutor  *mock.SimpleQueryExecutor
		)

		BeforeEach(func() {
			fakeLegacyProvider = &mock.LegacyDeployedCCInfoProvider{}

			l = &lifecycle.Lifecycle{
				LegacyDeployedCCInfoProvider: fakeLegacyProvider,
			}

			fakeQueryExecutor = &mock.SimpleQueryExecutor{}
		})

		Describe("Namespaces", func() {
			BeforeEach(func() {
				fakeLegacyProvider.NamespacesReturns([]string{"a", "b", "c"})
			})

			It("appends its own namespaces the legacy impl", func() {
				res := l.Namespaces()
				Expect(res).To(Equal([]string{"+lifecycle", "a", "b", "c"}))
				Expect(fakeLegacyProvider.NamespacesCallCount()).To(Equal(1))
			})
		})

		Describe("UpdatedChaincodes", func() {
			var (
				updates map[string][]*kvrwset.KVWrite
			)

			BeforeEach(func() {
				updates = map[string][]*kvrwset.KVWrite{
					"+lifecycle": {
						{Key: "some/random/value"},
						{Key: "namespaces/fields/cc-name/Sequence"},
						{Key: "prefix/namespaces/fields/cc-name/Sequence"},
						{Key: "namespaces/fields/Sequence/infix"},
						{Key: "namespaces/fields/cc-name/Sequence/Postfix"},
					},
					"other-namespace": nil,
				}
				fakeLegacyProvider.UpdatedChaincodesReturns([]*ledger.ChaincodeLifecycleInfo{
					{Name: "foo"},
					{Name: "bar"},
				}, nil)
			})

			It("checks its own namespace, then passes through to the legacy impl", func() {
				res, err := l.UpdatedChaincodes(updates)
				Expect(res).To(Equal([]*ledger.ChaincodeLifecycleInfo{
					{Name: "cc-name"},
					{Name: "foo"},
					{Name: "bar"},
				}))
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeLegacyProvider.UpdatedChaincodesCallCount()).To(Equal(1))
				Expect(fakeLegacyProvider.UpdatedChaincodesArgsForCall(0)).To(Equal(updates))
			})

			Context("when the legacy provider returns an error", func() {
				BeforeEach(func() {
					fakeLegacyProvider.UpdatedChaincodesReturns(nil, fmt.Errorf("legacy-error"))
				})

				It("wraps and returns the error", func() {
					_, err := l.UpdatedChaincodes(updates)
					Expect(err).To(MatchError("error invoking legacy deployed cc info provider: legacy-error"))
				})
			})
		})

		Describe("ChaincodeInfo", func() {
			BeforeEach(func() {
				fakeLegacyProvider.ChaincodeInfoReturns(&ledger.DeployedChaincodeInfo{
					Name:    "cc-name",
					Hash:    []byte("hash"),
					Version: "cc-version",
				}, fmt.Errorf("chaincode-info-error"))
			})

			It("passes through to the legacy impl", func() {
				res, err := l.ChaincodeInfo("channel-name", "cc-name", fakeQueryExecutor)
				Expect(res).To(Equal(&ledger.DeployedChaincodeInfo{
					Name:    "cc-name",
					Hash:    []byte("hash"),
					Version: "cc-version",
				}))
				Expect(err).To(MatchError("chaincode-info-error"))
				Expect(fakeLegacyProvider.ChaincodeInfoCallCount()).To(Equal(1))
				channelID, ccName, qe := fakeLegacyProvider.ChaincodeInfoArgsForCall(0)
				Expect(channelID).To(Equal("channel-name"))
				Expect(ccName).To(Equal("cc-name"))
				Expect(qe).To(Equal(fakeQueryExecutor))
			})
		})

		Describe("CollectionInfo", func() {
			var (
				collInfo *cb.StaticCollectionConfig
			)

			BeforeEach(func() {
				collInfo = &cb.StaticCollectionConfig{}
				fakeLegacyProvider.CollectionInfoReturns(collInfo, fmt.Errorf("collection-info-error"))
			})

			It("passes through to the legacy impl", func() {
				res, err := l.CollectionInfo("channel-name", "cc-name", "collection-name", fakeQueryExecutor)
				Expect(res).To(Equal(collInfo))
				Expect(err).To(MatchError("collection-info-error"))
				Expect(fakeLegacyProvider.CollectionInfoCallCount()).To(Equal(1))
				channelID, ccName, collName, qe := fakeLegacyProvider.CollectionInfoArgsForCall(0)
				Expect(channelID).To(Equal("channel-name"))
				Expect(ccName).To(Equal("cc-name"))
				Expect(collName).To(Equal("collection-name"))
				Expect(qe).To(Equal(fakeQueryExecutor))
			})
		})

		Describe("ImplicitCollections", func() {
			It("always returns nil", func() {
				res, err := l.ImplicitCollections("channel-id", "chaincode-id", fakeQueryExecutor)
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(BeNil())
			})
		})
	})
})
