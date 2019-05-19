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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MetadataProvider", func() {
	var (
		fakeChaincodeInfoProvider  *mock.ChaincodeInfoProvider
		fakeLegacyMetadataProvider *mock.LegacyMetadataProvider
		metadataProvider           *lifecycle.MetadataProvider
	)

	BeforeEach(func() {
		fakeChaincodeInfoProvider = &mock.ChaincodeInfoProvider{}
		ccInfo := &lifecycle.LocalChaincodeInfo{
			Definition: &lifecycle.ChaincodeDefinition{
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version: "cc-version",
				},
				ValidationInfo: &lb.ChaincodeValidationInfo{
					ValidationParameter: []byte("validation-parameter"),
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
		metadataProvider = &lifecycle.MetadataProvider{
			ChaincodeInfoProvider:  fakeChaincodeInfoProvider,
			LegacyMetadataProvider: fakeLegacyMetadataProvider,
		}
	})

	It("returns metadata using the ChaincodeInfoProvider", func() {
		metadata := metadataProvider.Metadata("testchannel", "cc-name", true)
		Expect(metadata).To(Equal(
			&chaincode.Metadata{
				Name:              "cc-name",
				Version:           "cc-version",
				Policy:            []byte("validation-parameter"),
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
})
