/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MetadataManager", func() {
	var (
		metadataManager            *lifecycle.MetadataManager
		fakeMetadataUpdateListener *mock.MetadataUpdateListener
	)

	BeforeEach(func() {
		metadataManager = lifecycle.NewMetadataManager()
		fakeMetadataUpdateListener = &mock.MetadataUpdateListener{}
		metadataManager.AddListener(lifecycle.HandleMetadataUpdateFunc(fakeMetadataUpdateListener.HandleMetadataUpdate))
	})

	It("initializes metadata", func() {
		chaincodes := chaincode.MetadataSet{
			{
				Name:    "cc-name",
				Version: "cc-version",
				Policy:  []byte("cc-policy"),
			},
		}
		metadataManager.InitializeMetadata("testchannel", chaincodes)
		Expect(metadataManager.MetadataSet["testchannel"]).To(HaveLen(1))
		Expect(metadataManager.MetadataSet["testchannel"]).To(ConsistOf(
			chaincode.Metadata{
				Name:    "cc-name",
				Version: "cc-version",
				Policy:  []byte("cc-policy"),
			},
		))
	})

	It("updates metadata and notifies registered listeners", func() {
		chaincodes := chaincode.MetadataSet{
			{
				Name:    "cc-name",
				Version: "cc-version",
				Policy:  []byte("cc-policy"),
			},
		}

		metadataManager.UpdateMetadata("testchannel", chaincodes)
		Expect(metadataManager.MetadataSet["testchannel"]).To(HaveLen(1))
		Expect(metadataManager.MetadataSet["testchannel"]).To(ConsistOf(
			chaincode.Metadata{
				Name:    "cc-name",
				Version: "cc-version",
				Policy:  []byte("cc-policy"),
			},
		))
		Expect(fakeMetadataUpdateListener.HandleMetadataUpdateCallCount()).To(Equal(1))
	})

	It("updates legacy metadata and notifies registered listeners", func() {
		chaincodes := chaincode.MetadataSet{
			{
				Name:    "legacy-cc-name",
				Version: "legacy-cc-version",
				Policy:  []byte("legacy-cc-policy"),
			},
		}

		metadataManager.HandleMetadataUpdate("testchannel", chaincodes)
		Expect(metadataManager.LegacyMetadataSet["testchannel"]).To(HaveLen(1))
		Expect(metadataManager.LegacyMetadataSet["testchannel"]).To(ConsistOf(
			chaincode.Metadata{
				Name:    "legacy-cc-name",
				Version: "legacy-cc-version",
				Policy:  []byte("legacy-cc-policy"),
			},
		))
		Expect(fakeMetadataUpdateListener.HandleMetadataUpdateCallCount()).To(Equal(1))
	})

	Context("when _lifecycle and lscc chaincodes are both defined and an lscc chaincode is updated", func() {
		var legacyChaincodes chaincode.MetadataSet

		BeforeEach(func() {
			chaincodes := chaincode.MetadataSet{
				{
					Name:      "cc-name",
					Version:   "cc-version",
					Policy:    []byte("cc-policy"),
					Approved:  true,
					Installed: true,
				},
			}
			legacyChaincodes = chaincode.MetadataSet{
				{
					Name:    "legacy-cc-name",
					Version: "legacy-cc-version",
					Policy:  []byte("legacy-cc-policy"),
				},
			}
			metadataManager.InitializeMetadata("testchannel", chaincodes)
			Expect(metadataManager.MetadataSet["testchannel"]).To(HaveLen(1))
			Expect(metadataManager.MetadataSet["testchannel"]).To(ConsistOf(
				chaincode.Metadata{
					Name:      "cc-name",
					Version:   "cc-version",
					Policy:    []byte("cc-policy"),
					Approved:  true,
					Installed: true,
				},
			))
		})

		It("aggregates the chaincode metadata", func() {
			metadataManager.HandleMetadataUpdate("testchannel", legacyChaincodes)
			Expect(fakeMetadataUpdateListener.HandleMetadataUpdateCallCount()).To(Equal(1))
			channelArg, metadataSetArg := fakeMetadataUpdateListener.HandleMetadataUpdateArgsForCall(0)
			Expect(channelArg).To(Equal("testchannel"))
			Expect(metadataSetArg).To(HaveLen(2))
			Expect(metadataSetArg).To(ConsistOf(
				chaincode.Metadata{
					Name:      "cc-name",
					Version:   "cc-version",
					Policy:    []byte("cc-policy"),
					Approved:  true,
					Installed: true,
				},
				chaincode.Metadata{
					Name:    "legacy-cc-name",
					Version: "legacy-cc-version",
					Policy:  []byte("legacy-cc-policy"),
				},
			))
		})
	})

	Context("when _lifecycle and lscc chaincodes are both for the same chaincode", func() {
		var legacyChaincodes chaincode.MetadataSet

		BeforeEach(func() {
			chaincodes := chaincode.MetadataSet{
				{
					Name:      "cc-name",
					Version:   "cc-version",
					Policy:    []byte("cc-policy"),
					Approved:  true,
					Installed: true,
				},
			}
			legacyChaincodes = chaincode.MetadataSet{
				{
					Name:    "cc-name",
					Version: "legacy-cc-version",
					Policy:  []byte("legacy-cc-policy"),
				},
			}
			metadataManager.InitializeMetadata("testchannel", chaincodes)
			Expect(metadataManager.MetadataSet["testchannel"]).To(HaveLen(1))
			Expect(metadataManager.MetadataSet["testchannel"]).To(ConsistOf(
				chaincode.Metadata{
					Name:      "cc-name",
					Version:   "cc-version",
					Policy:    []byte("cc-policy"),
					Approved:  true,
					Installed: true,
				},
			))
		})

		It("returns the version from _lifecycle", func() {
			metadataManager.HandleMetadataUpdate("testchannel", legacyChaincodes)
			Expect(fakeMetadataUpdateListener.HandleMetadataUpdateCallCount()).To(Equal(1))
			channelArg, metadataSetArg := fakeMetadataUpdateListener.HandleMetadataUpdateArgsForCall(0)
			Expect(channelArg).To(Equal("testchannel"))
			Expect(metadataSetArg).To(HaveLen(1))
			Expect(metadataSetArg).To(ConsistOf(
				chaincode.Metadata{
					Name:      "cc-name",
					Version:   "cc-version",
					Policy:    []byte("cc-policy"),
					Approved:  true,
					Installed: true,
				},
			))
		})
	})

	Context("when _lifecycle and lscc chaincodes are both for the same chaincode but has not been approved/installed in _lifecycle", func() {
		var legacyChaincodes chaincode.MetadataSet

		BeforeEach(func() {
			chaincodes := chaincode.MetadataSet{
				{
					Name:      "notapproved",
					Version:   "na-version",
					Policy:    []byte("na-policy"),
					Approved:  false,
					Installed: true,
				},
				{
					Name:      "notinstalled",
					Version:   "ni-version",
					Policy:    []byte("ni-policy"),
					Approved:  true,
					Installed: false,
				},
			}
			legacyChaincodes = chaincode.MetadataSet{
				{
					Name:    "notapproved",
					Version: "na-version",
					Policy:  []byte("legacy-na-policy"),
				},
				{
					Name:    "notinstalled",
					Version: "ni-version",
					Policy:  []byte("legacy-ni-policy"),
				},
			}
			metadataManager.InitializeMetadata("testchannel", chaincodes)
			Expect(metadataManager.MetadataSet["testchannel"]).To(HaveLen(2))
			Expect(metadataManager.MetadataSet["testchannel"]).To(ConsistOf(
				chaincode.Metadata{
					Name:      "notapproved",
					Version:   "na-version",
					Policy:    []byte("na-policy"),
					Approved:  false,
					Installed: true,
				},
				chaincode.Metadata{
					Name:      "notinstalled",
					Version:   "ni-version",
					Policy:    []byte("ni-policy"),
					Approved:  true,
					Installed: false,
				},
			))
		})

		It("does not include stale definitions from lscc", func() {
			metadataManager.HandleMetadataUpdate("testchannel", legacyChaincodes)
			Expect(fakeMetadataUpdateListener.HandleMetadataUpdateCallCount()).To(Equal(1))
			channelArg, metadataSetArg := fakeMetadataUpdateListener.HandleMetadataUpdateArgsForCall(0)
			Expect(channelArg).To(Equal("testchannel"))
			Expect(metadataSetArg).To(BeEmpty())
		})
	})
})
