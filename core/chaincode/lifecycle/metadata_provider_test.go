/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/policydsl"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/pkg/errors"

	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
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
					ValidationParameter: protoutil.MarshalOrPanic(&pb.ApplicationPolicy{
						Type: &pb.ApplicationPolicy_SignaturePolicy{
							SignaturePolicy: policydsl.AcceptAllPolicy,
						},
					}),
				},
				Collections: &pb.CollectionConfigPackage{},
			},
		}
		fakeChaincodeInfoProvider.ChaincodeInfoReturns(ccInfo, nil)

		legacyCCMetadata := &chaincode.Metadata{
			Name:               "legacy-cc",
			Version:            "legacy-version",
			Policy:             []byte("legacy-policy"),
			CollectionPolicies: nil,
			CollectionsConfig:  &pb.CollectionConfigPackage{},
		}
		fakeLegacyMetadataProvider = &mock.LegacyMetadataProvider{}
		fakeLegacyMetadataProvider.MetadataReturns(legacyCCMetadata)

		metadataProvider = lifecycle.NewMetadataProvider(fakeChaincodeInfoProvider, fakeLegacyMetadataProvider, nil)
	})

	It("returns metadata using the ChaincodeInfoProvider (SignaturePolicyEnvelope case)", func() {
		metadata := metadataProvider.Metadata("testchannel", "cc-name", "col1")
		Expect(metadata).To(Equal(
			&chaincode.Metadata{
				Name:               "cc-name",
				Version:            "1",
				Policy:             policydsl.MarshaledAcceptAllPolicy,
				CollectionPolicies: map[string][]byte{},
				CollectionsConfig:  &pb.CollectionConfigPackage{},
			},
		))
	})

	It("returns metadata for implicit collections", func() {
		metadata := metadataProvider.Metadata("testchannel", "cc-name", "_implicit_org_msp1")
		Expect(metadata.CollectionPolicies).To(HaveKey("_implicit_org_msp1"))
	})

	Context("when the chaincode is not found by the ChaincodeInfoProvider", func() {
		BeforeEach(func() {
			fakeChaincodeInfoProvider.ChaincodeInfoReturns(nil, errors.New("scrumtrulescent"))
		})

		It("returns metadata using the LegacyMetadataProvider", func() {
			metadata := metadataProvider.Metadata("testchannel", "legacy-cc", "col1")
			Expect(metadata).To(Equal(
				&chaincode.Metadata{
					Name:               "legacy-cc",
					Version:            "legacy-version",
					Policy:             []byte("legacy-policy"),
					CollectionPolicies: nil,
					CollectionsConfig:  &pb.CollectionConfigPackage{},
				},
			))
		})
	})

	Context("when the policy is bad", func() {
		BeforeEach(func() {
			ccInfo.Definition.ValidationInfo.ValidationParameter = []byte{0, 1, 2}
			fakeChaincodeInfoProvider.ChaincodeInfoReturns(ccInfo, nil)
		})

		It("returns nil", func() {
			metadata := metadataProvider.Metadata("testchannel", "cc-name", "col1")
			Expect(metadata).To(BeNil())
		})
	})

	Context("when the policy is of the channel reference type", func() {
		BeforeEach(func() {
			ccInfo.Definition.ValidationInfo.ValidationParameter = protoutil.MarshalOrPanic(
				&pb.ApplicationPolicy{
					Type: &pb.ApplicationPolicy_ChannelConfigPolicyReference{
						ChannelConfigPolicyReference: "barf",
					},
				})
			fakeChaincodeInfoProvider.ChaincodeInfoReturns(ccInfo, nil)

			fakeConvertedPolicy = &mock.ConvertiblePolicy{}
			fakeConvertedPolicy.ConvertReturns(policydsl.AcceptAllPolicy, nil)

			fakeChannelPolicyReferenceProvider = &mock.ChannelPolicyReferenceProvider{}
			fakeChannelPolicyReferenceProvider.NewPolicyReturns(fakeConvertedPolicy, nil)
			metadataProvider.ChannelPolicyReferenceProvider = fakeChannelPolicyReferenceProvider
		})

		It("returns metadata after translating the policy", func() {
			metadata := metadataProvider.Metadata("testchannel", "cc-name", "col1")
			Expect(metadata).To(Equal(
				&chaincode.Metadata{
					Name:               "cc-name",
					Version:            "1",
					Policy:             policydsl.MarshaledAcceptAllPolicy,
					CollectionPolicies: map[string][]byte{},
					CollectionsConfig:  &pb.CollectionConfigPackage{},
				},
			))
		})

		Context("when NewPolicy returns an error", func() {
			BeforeEach(func() {
				fakeChannelPolicyReferenceProvider = &mock.ChannelPolicyReferenceProvider{}
				fakeChannelPolicyReferenceProvider.NewPolicyReturns(nil, errors.New("go away"))
				metadataProvider.ChannelPolicyReferenceProvider = fakeChannelPolicyReferenceProvider
			})

			It("returns nil", func() {
				metadata := metadataProvider.Metadata("testchannel", "cc-name", "col1")
				Expect(metadata).To(BeNil())
			})
		})

		Context("when the policy is not convertible", func() {
			BeforeEach(func() {
				fakeChannelPolicyReferenceProvider = &mock.ChannelPolicyReferenceProvider{}
				fakeChannelPolicyReferenceProvider.NewPolicyReturns(&mock.InconvertiblePolicy{}, nil)
				metadataProvider.ChannelPolicyReferenceProvider = fakeChannelPolicyReferenceProvider
			})

			It("returns nil", func() {
				metadata := metadataProvider.Metadata("testchannel", "cc-name", "col1")
				Expect(metadata).To(BeNil())
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

			It("returns nil", func() {
				metadata := metadataProvider.Metadata("testchannel", "cc-name", "col1")
				Expect(metadata).To(BeNil())
			})
		})
	})

	Context("when collection endorsement policies exist", func() {
		var (
			expectedSignaturePolicy         *common.SignaturePolicyEnvelope
			expectedCollectionConfigPackage *pb.CollectionConfigPackage
		)

		BeforeEach(func() {
			expectedSignaturePolicy = &common.SignaturePolicyEnvelope{
				Identities: []*msp.MSPPrincipal{
					{
						Principal: []byte("test"),
					},
				},
			}

			expectedCollectionConfigPackage = &pb.CollectionConfigPackage{
				Config: []*pb.CollectionConfig{
					{
						Payload: &pb.CollectionConfig_StaticCollectionConfig{
							StaticCollectionConfig: &pb.StaticCollectionConfig{
								Name: "col1",
								EndorsementPolicy: &pb.ApplicationPolicy{
									Type: &pb.ApplicationPolicy_SignaturePolicy{
										SignaturePolicy: expectedSignaturePolicy,
									},
								},
							},
						},
					},
					{
						Payload: &pb.CollectionConfig_StaticCollectionConfig{
							StaticCollectionConfig: &pb.StaticCollectionConfig{
								Name: "col2",
								EndorsementPolicy: &pb.ApplicationPolicy{
									Type: &pb.ApplicationPolicy_SignaturePolicy{
										SignaturePolicy: expectedSignaturePolicy,
									},
								},
							},
						},
					},
					{
						Payload: &pb.CollectionConfig_StaticCollectionConfig{
							StaticCollectionConfig: &pb.StaticCollectionConfig{
								Name: "col3",
								// col3 has no endorsement policy
								EndorsementPolicy: nil,
							},
						},
					},
				},
			}
			ccInfo = &lifecycle.LocalChaincodeInfo{
				Definition: &lifecycle.ChaincodeDefinition{
					Sequence: 1,
					EndorsementInfo: &lb.ChaincodeEndorsementInfo{
						Version: "cc-version",
					},
					ValidationInfo: &lb.ChaincodeValidationInfo{
						ValidationParameter: protoutil.MarshalOrPanic(&pb.ApplicationPolicy{
							Type: &pb.ApplicationPolicy_SignaturePolicy{
								SignaturePolicy: policydsl.AcceptAllPolicy,
							},
						}),
					},
					Collections: expectedCollectionConfigPackage,
				},
			}
			fakeChaincodeInfoProvider.ChaincodeInfoReturns(ccInfo, nil)
		})

		It("returns metadata that includes the collection policies", func() {
			metadata := metadataProvider.Metadata("testchannel", "cc-name", "col1", "col2", "col3")
			Expect(metadata).To(Equal(
				&chaincode.Metadata{
					Name:    "cc-name",
					Version: "1",
					Policy:  policydsl.MarshaledAcceptAllPolicy,
					CollectionPolicies: map[string][]byte{
						"col1": protoutil.MarshalOrPanic(expectedSignaturePolicy),
						"col2": protoutil.MarshalOrPanic(expectedSignaturePolicy),
						// col3 has no endorsement policy
					},
					CollectionsConfig: expectedCollectionConfigPackage,
				},
			))
		})
	})
})
