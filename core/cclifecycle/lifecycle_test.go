/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cc_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/cclifecycle"
	"github.com/hyperledger/fabric/core/cclifecycle/mocks"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestHandleMetadataUpdate(t *testing.T) {
	f := func(channel string, chaincodes chaincode.MetadataSet) {
		assert.Len(t, chaincodes, 2)
		assert.Equal(t, "mychannel", channel)
	}
	cc.HandleMetadataUpdate(f).LifeCycleChangeListener("mychannel", chaincode.MetadataSet{{}, {}})
}

func TestEnumerate(t *testing.T) {
	f := func() ([]chaincode.InstalledChaincode, error) {
		return []chaincode.InstalledChaincode{{}, {}}, nil
	}
	ccs, err := cc.Enumerate(f).Enumerate()
	assert.NoError(t, err)
	assert.Len(t, ccs, 2)
}

func TestLifecycleInitFailure(t *testing.T) {
	listCCs := &mocks.Enumerator{}
	listCCs.EnumerateReturns(nil, errors.New("failed accessing DB"))
	lc, err := cc.NewLifeCycle(listCCs)
	assert.Nil(t, lc)
	assert.Contains(t, err.Error(), "failed accessing DB")
}

func TestLifeCycleSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LifeCycle Suite")
}

var _ = Describe("LifeCycle", func() {
	var (
		nilSlice chaincode.MetadataSet
		err      error
		cc1Bytes = utils.MarshalOrPanic(&ccprovider.ChaincodeData{
			Name:    "cc1",
			Version: "1.0",
			Id:      []byte{42},
			Policy:  []byte{1, 2, 3, 4, 5},
		})
		cc1BytesAfterUpgrade = utils.MarshalOrPanic(&ccprovider.ChaincodeData{
			Name:    "cc1",
			Version: "1.1",
			Id:      []byte{42},
			Policy:  []byte{5, 4, 3, 2, 1},
		})
		cc2Bytes = utils.MarshalOrPanic(&ccprovider.ChaincodeData{
			Name:    "cc2",
			Version: "1.0",
		})
		metadataAtStartup = chaincode.Metadata{
			Name:    "cc1",
			Version: "1.0",
			Id:      []byte{42},
			Policy:  []byte{1, 2, 3, 4, 5},
		}
		metadataAfterUpgrade = chaincode.Metadata{
			Name:    "cc1",
			Version: "1.1",
			Id:      []byte{42},
			Policy:  []byte{5, 4, 3, 2, 1},
		}
		metadataOfSecondChaincode = chaincode.Metadata{
			Name:    "cc2",
			Version: "1.0",
		}
		listCCs  = &mocks.Enumerator{}
		lsnr     = &mocks.Listener{}
		query    = &mocks.Query{}
		newQuery = func() (cc.Query, error) {
			return query, nil
		}
		lc *cc.Lifecycle
	)

	BeforeEach(func() {
		listCCs.EnumerateReturns([]chaincode.InstalledChaincode{
			{
				Name:    "cc1",
				Version: "1.0",
				Id:      []byte{42},
			},
			{
				// This chaincode isn't instantiated on the channel, but is installed
				Name:    "cc3",
				Version: "1.0",
				Id:      []byte{50},
			},
		}, nil)
		lc, err = cc.NewLifeCycle(listCCs)
		Expect(err).NotTo(HaveOccurred())
		lc.AddListener(lsnr)
	})
	Context("When a subscription is made successfully", func() {
		BeforeEach(func() {
			query.GetStateReturnsOnCall(0, cc1Bytes, nil)
			query.GetStateReturnsOnCall(1, nil, nil)
		})
		var sub *cc.Subscription
		It("is given the initial lifecycle data via invocation of LifeCycleChangeListener", func() {
			// Before we subscribe to any channel, the listener should not be called
			Expect(lsnr.LifeCycleChangeListenerCallCount()).To(Equal(0))
			sub, err = lc.NewChannelSubscription("mychannel", newQuery)
			Expect(err).NotTo(HaveOccurred())
			// Ensure that the listener was called for the channel we subscribed for
			Expect(lsnr.LifeCycleChangeListenerCallCount()).To(Equal(1))
			Expect(lsnr.Invocations()["LifeCycleChangeListener"][0]).To(Equal([]interface{}{
				"mychannel",
				chaincode.MetadataSet{metadataAtStartup},
			}))
		})

		Context("when a chaincode upgrade occurs", func() {
			BeforeEach(func() {
				query.GetStateReturnsOnCall(2, cc1BytesAfterUpgrade, nil)
			})
			It("causes the listeners to be invoked with the updated metadata", func() {
				sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{
					Name:    "cc1",
					Version: "1.1", // 1.0 --> 1.1
					Hash:    []byte{42},
				}, nil)
				Expect(lsnr.LifeCycleChangeListenerCallCount()).To(Equal(2))
				Expect(lsnr.Invocations()["LifeCycleChangeListener"][1]).To(Equal([]interface{}{
					"mychannel",
					chaincode.MetadataSet{metadataAfterUpgrade},
				}))
			})
		})

		Context("when a chaincode install and deploy occurs", func() {
			BeforeEach(func() {
				query.GetStateReturnsOnCall(3, cc2Bytes, nil)
			})
			It("causes the listeners to be invoked with the updated metadata of the new and previous chaincodes", func() {
				sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{
					Name:    "cc2",
					Version: "1.0",
				}, nil)
				Expect(lsnr.LifeCycleChangeListenerCallCount()).To(Equal(3))
				// Argument 0 is "mychannel" and 1 is the metadata passed
				metadataPassed := lsnr.Invocations()["LifeCycleChangeListener"][2][1].(chaincode.MetadataSet)
				Expect(len(metadataPassed)).To(Equal(2))
				Expect(metadataPassed).To(ContainElement(metadataAfterUpgrade))
				Expect(metadataPassed).To(ContainElement(metadataOfSecondChaincode))
			})
		})
	})
	Context("When a subscription is not made successfully", func() {
		query := &mocks.Query{}
		BeforeEach(func() {
			query.GetStateReturns(nil, errors.New("failed querying lscc"))
		})
		It("might fail because problem of instantiating a new query", func() {
			_, err = lc.NewChannelSubscription("mychannel", func() (cc.Query, error) {
				return nil, errors.New("DB not available")
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DB not available"))
		})
		It("might fail when it tries to query the state DB", func() {
			_, err = lc.NewChannelSubscription("mychannel", func() (cc.Query, error) {
				return query, nil
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed querying lscc"))
		})
	})

	Context("When a subscription fails later on", func() {
		query := &mocks.Query{}
		BeforeEach(func() {
			query.GetStateReturnsOnCall(0, cc1Bytes, nil)
			query.GetStateReturnsOnCall(1, nil, nil)
			query.GetStateReturnsOnCall(2, nil, errors.New("failed querying lscc"))
		})
		var sub *cc.Subscription
		It("might fail the deployment notification when it queries lscc", func() {
			sub, err = lc.NewChannelSubscription("mychannel", func() (cc.Query, error) {
				return query, nil
			})
			Expect(err).To(Not(HaveOccurred()))
			err = sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{
				Name:    "cc1",
				Version: "1.0",
				Hash:    []byte{42},
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed querying lscc"))
		})

		It("might fail the deployment notification when it creates a new query", func() {
			var secondQuery bool
			sub, err = lc.NewChannelSubscription("mychannel", func() (cc.Query, error) {
				if !secondQuery {
					secondQuery = true
					return query, nil
				}
				return nil, errors.New("failed accessing DB")
			})
			Expect(err).To(Not(HaveOccurred()))
			err = sub.HandleChaincodeDeploy(&cceventmgmt.ChaincodeDefinition{
				Name:    "cc1",
				Version: "1.0",
				Hash:    []byte{42},
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed accessing DB"))

			// We also can't query the DB via Metadata()
			md := lc.Metadata("mychannel", "cc5")
			var nilMetadata *chaincode.Metadata
			Expect(md).To(Equal(nilMetadata))
		})
	})

	Describe("Channel metadata retrieval", func() {
		var query *mocks.Query
		var nilMetadata *chaincode.Metadata

		BeforeEach(func() {
			query = &mocks.Query{}
			query.GetStateReturnsOnCall(0, cc1Bytes, nil)
			query.GetStateReturnsOnCall(1, nil, nil)
			_, err = lc.NewChannelSubscription("mychannel", func() (cc.Query, error) {
				return query, nil
			})
		})

		Context("When the chaincode is installed and deployed", func() {
			It("does not attempt to query LSCC for chaincodes that are installed, instead - it fetches from memory", func() {
				md := lc.Metadata("mychannel", "cc1")
				Expect(*md).To(Equal(metadataAtStartup))
			})
		})

		Context("When the query fails", func() {
			JustBeforeEach(func() {
				query.GetStateReturnsOnCall(2, nil, errors.New("failed querying lscc"))
			})
			It("does not return the metadata if the query fails", func() {
				md := lc.Metadata("mychannel", "cc5")
				Expect(md).To(Equal(nilMetadata))
			})
		})

		Context("When a non existent channel is queried for", func() {
			It("does not return the metadata", func() {
				md := lc.Metadata("mychannel5", "cc1")
				Expect(md).To(Equal(nilMetadata))
			})
		})

		Context("When the chaincode isn't deployed or installed", func() {
			JustBeforeEach(func() {
				query.GetStateReturnsOnCall(2, nil, nil)
			})
			It("returns empty metadata for chaincodes that are not installed and not deployed", func() {
				md := lc.Metadata("mychannel", "cc5")
				Expect(md).To(Equal(nilMetadata))
			})
		})

		Context("When the chaincode is deployed but not installed", func() {
			JustBeforeEach(func() {
				query.GetStateReturnsOnCall(2, cc2Bytes, nil)
			})
			It("returns empty metadata for chaincodes that are not installed and not deployed", func() {
				md := lc.Metadata("mychannel", "cc2")
				Expect(*md).To(Equal(chaincode.Metadata{
					Name:    "cc2",
					Version: "1.0",
				}))
			})
		})
	})

	Context("When the chaincode installed is of a different version", func() {
		query := &mocks.Query{}
		BeforeEach(func() {
			query.GetStateReturnsOnCall(0, cc1BytesAfterUpgrade, nil)
			query.GetStateReturnsOnCall(1, nil, nil)
			lc, err = cc.NewLifeCycle(listCCs)
			Expect(err).NotTo(HaveOccurred())
			lsnr = &mocks.Listener{}
			lc.AddListener(lsnr)
			query.GetStateReturns(cc1BytesAfterUpgrade, nil)
		})
		It("doesn't end up in the results", func() {
			_, err = lc.NewChannelSubscription("mychannel", func() (cc.Query, error) {
				return query, nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(lsnr.LifeCycleChangeListenerCallCount()).To(Equal(1))
			Expect(lsnr.Invocations()["LifeCycleChangeListener"][0]).To(Equal([]interface{}{
				"mychannel",
				nilSlice,
			}))
		})
	})

	Context("When a chaincode is deployed but read from lscc returns wrong ID compared to what is in the filesystem", func() {
		query := &mocks.Query{}
		BeforeEach(func() {
			lsnr = &mocks.Listener{}
			lc.AddListener(lsnr)
			query.GetStateReturnsOnCall(0, utils.MarshalOrPanic(&ccprovider.ChaincodeData{
				Name:    "cc1",
				Version: "1.0",
				Id:      []byte{100},
			}), nil)
			query.GetStateReturnsOnCall(1, nil, nil)
		})
		It("doesn't end up in the results", func() {
			_, err = lc.NewChannelSubscription("mychannel2", func() (cc.Query, error) {
				return query, nil
			})
			Expect(lsnr.LifeCycleChangeListenerCallCount()).To(Equal(1))
			Expect(lsnr.Invocations()["LifeCycleChangeListener"][0]).To(Equal([]interface{}{
				"mychannel2",
				nilSlice,
			}))
		})
	})

	Context("when a subscription to the same channel is made once again", func() {
		It("doesn't load data from the state DB", func() {
			_, err = lc.NewChannelSubscription("mychannel", newQuery)
			Expect(err).NotTo(HaveOccurred())
			currentInvocationCount := query.GetStateCallCount()
			_, err = lc.NewChannelSubscription("mychannel", newQuery)
			Expect(err).NotTo(HaveOccurred())
			Expect(query.GetStateCallCount()).To(Equal(currentInvocationCount))
		})
	})
})
