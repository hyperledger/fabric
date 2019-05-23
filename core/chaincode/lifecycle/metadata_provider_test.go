/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	cb "github.com/hyperledger/fabric/protos/common"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	"github.com/pkg/errors"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MetadataProvider", func() {
	var (
		fakeChaincodeInfoProvider          *mock.ChaincodeInfoProvider
		fakeLegacyMetadataProvider         *mock.LegacyMetadataProvider
		fakeChannelPolicyReferenceProvider *mock.ChannelPolicyReferenceProvider
		fakeConvertedPolicy                *mock.ConvertiblePolicy
		metadataProvider                   *lifecycle.MetadataProvider
		ccInfo                             *lifecycle.LocalChaincodeInfo
	)

	BeforeEach(func() {
		fakeChaincodeInfoProvider = &mock.ChaincodeInfoProvider{}
		ccInfo = &lifecycle.LocalChaincodeInfo{
			Definition: &lifecycle.ChaincodeDefinition{
				Sequence: 1,
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version: "cc-version",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationParameter: protoutil.MarshalOrPanic(&peer.ApplicationPolicy{
						Type: &peer.ApplicationPolicy_SignaturePolicy{
							SignaturePolicy: cauthdsl.AcceptAllPolicy,
						},
					}),
				},
				Collections: &cb.CollectionConfigPackage{},
			},
		}
		fakeChaincodeInfoProvider.ChaincodeInfoReturns(ccInfo, nil)

		legacyCCMetadata := &chaincode.Metadata{
			Name:              "legacy-cc",
			Version:           "legacy-version",
			Policy:            []byte("legacy-policy"),
			CollectionsConfig: &cb.CollectionConfigPackage{},
		}
		fakeLegacyMetadataProvider = &mock.LegacyMetadataProvider{}
		fakeLegacyMetadataProvider.MetadataReturns(legacyCCMetadata)

		metadataProvider = lifecycle.NewMetadataProvider(fakeChaincodeInfoProvider, fakeLegacyMetadataProvider, nil)
	})

	It("returns metadata using the ChaincodeInfoProvider (SignaturePolicyEnvelope case)", func() {
		metadata := metadataProvider.Metadata("testchannel", "cc-name", true)
		Expect(metadata).To(Equal(
			&chaincode.Metadata{
				Name:              "cc-name",
				Version:           "1",
				Policy:            cauthdsl.MarshaledAcceptAllPolicy,
				CollectionsConfig: &cb.CollectionConfigPackage{},
			},
		))
	})

	Context("when the chaincode is not found by the ChaincodeInfoProvider", func() {
		BeforeEach(func() {
			fakeChaincodeInfoProvider.ChaincodeInfoReturns(nil, errors.New("scrumtrulescent"))
		})

		It("returns metadata using the LegacyMetadataProvider", func() {
			metadata := metadataProvider.Metadata("testchannel", "legacy-cc", true)
			Expect(metadata).To(Equal(
				&chaincode.Metadata{
					Name:              "legacy-cc",
					Version:           "legacy-version",
					Policy:            []byte("legacy-policy"),
					CollectionsConfig: &cb.CollectionConfigPackage{},
				},
			))
		})
	})

	Context("when the policy is bad", func() {
		BeforeEach(func() {
			ccInfo.Definition.ValidationInfo.ValidationParameter = []byte{0, 1, 2}
			fakeChaincodeInfoProvider.ChaincodeInfoReturns(ccInfo, nil)
		})

		It("returns metadata after providing a reject-all policy", func() {
			metadata := metadataProvider.Metadata("testchannel", "cc-name", true)
			Expect(metadata).To(Equal(
				&chaincode.Metadata{
					Name:              "cc-name",
					Version:           "1",
					Policy:            cauthdsl.MarshaledRejectAllPolicy,
					CollectionsConfig: &cb.CollectionConfigPackage{},
				},
			))
		})
	})

	Context("when the policy is of the channel reference type", func() {
		BeforeEach(func() {
			ccInfo.Definition.ValidationInfo.ValidationParameter = protoutil.MarshalOrPanic(
				&peer.ApplicationPolicy{
					Type: &peer.ApplicationPolicy_ChannelConfigPolicyReference{
						ChannelConfigPolicyReference: "barf",
					},
				})
			fakeChaincodeInfoProvider.ChaincodeInfoReturns(ccInfo, nil)

			fakeConvertedPolicy = &mock.ConvertiblePolicy{}
			fakeConvertedPolicy.ConvertReturns(cauthdsl.AcceptAllPolicy, nil)

			fakeChannelPolicyReferenceProvider = &mock.ChannelPolicyReferenceProvider{}
			fakeChannelPolicyReferenceProvider.NewPolicyReturns(fakeConvertedPolicy, nil)
			metadataProvider.ChannelPolicyReferenceProvider = fakeChannelPolicyReferenceProvider
		})

		It("returns metadata after translating the policy", func() {
			metadata := metadataProvider.Metadata("testchannel", "cc-name", true)
			Expect(metadata).To(Equal(
				&chaincode.Metadata{
					Name:              "cc-name",
					Version:           "1",
					Policy:            cauthdsl.MarshaledAcceptAllPolicy,
					CollectionsConfig: &cb.CollectionConfigPackage{},
				},
			))
		})

		Context("when NewPolicy returns an error", func() {
			BeforeEach(func() {
				fakeChannelPolicyReferenceProvider = &mock.ChannelPolicyReferenceProvider{}
				fakeChannelPolicyReferenceProvider.NewPolicyReturns(nil, errors.New("go away"))
				metadataProvider.ChannelPolicyReferenceProvider = fakeChannelPolicyReferenceProvider
			})

			It("returns metadata after providing a reject-all policy", func() {
				metadata := metadataProvider.Metadata("testchannel", "cc-name", true)
				Expect(metadata).To(Equal(
					&chaincode.Metadata{
						Name:              "cc-name",
						Version:           "1",
						Policy:            cauthdsl.MarshaledRejectAllPolicy,
						CollectionsConfig: &cb.CollectionConfigPackage{},
					},
				))
			})
		})

		Context("when the policy is not convertible", func() {
			BeforeEach(func() {
				fakeChannelPolicyReferenceProvider = &mock.ChannelPolicyReferenceProvider{}
				fakeChannelPolicyReferenceProvider.NewPolicyReturns(&mock.InconvertiblePolicy{}, nil)
				metadataProvider.ChannelPolicyReferenceProvider = fakeChannelPolicyReferenceProvider
			})

			It("returns metadata after providing a reject-all policy", func() {
				metadata := metadataProvider.Metadata("testchannel", "cc-name", true)
				Expect(metadata).To(Equal(
					&chaincode.Metadata{
						Name:              "cc-name",
						Version:           "1",
						Policy:            cauthdsl.MarshaledRejectAllPolicy,
						CollectionsConfig: &cb.CollectionConfigPackage{},
					},
				))
			})
		})

		Context("when conversion fails", func() {
			BeforeEach(func() {
				fakeConvertedPolicy = &mock.ConvertiblePolicy{}
				fakeConvertedPolicy.ConvertReturns(nil, errors.New("go away"))

				fakeChannelPolicyReferenceProvider = &mock.ChannelPolicyReferenceProvider{}
				fakeChannelPolicyReferenceProvider.NewPolicyReturns(fakeConvertedPolicy, nil)
				metadataProvider.ChannelPolicyReferenceProvider = fakeChannelPolicyReferenceProvider
			})

			It("returns metadata after providing a reject-all policy", func() {
				metadata := metadataProvider.Metadata("testchannel", "cc-name", true)
				Expect(metadata).To(Equal(
					&chaincode.Metadata{
						Name:              "cc-name",
						Version:           "1",
						Policy:            cauthdsl.MarshaledRejectAllPolicy,
						CollectionsConfig: &cb.CollectionConfigPackage{},
					},
				))
			})
		})
	})
})
