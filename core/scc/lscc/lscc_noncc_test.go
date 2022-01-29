/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc_test

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/core/scc/lscc/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LSCC", func() {
	var (
		l                 *lscc.SCC
		fakeSupport       *mock.FileSystemSupport
		fakeCCPackage     *mock.CCPackage
		fakeSCCProvider   *mock.SystemChaincodeProvider
		fakeQueryExecutor *mock.QueryExecutor
		ccData            *ccprovider.ChaincodeData
		ccDataBytes       []byte
		err               error
	)

	BeforeEach(func() {
		fakeCCPackage = &mock.CCPackage{}
		fakeCCPackage.GetChaincodeDataReturns(&ccprovider.ChaincodeData{
			InstantiationPolicy: []byte("instantiation-policy"),
		})

		fakeSupport = &mock.FileSystemSupport{}
		fakeSupport.GetChaincodeFromLocalStorageReturns(fakeCCPackage, nil)

		fakeSCCProvider = &mock.SystemChaincodeProvider{}
		fakeQueryExecutor = &mock.QueryExecutor{}
		cryptoProvider, _ := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())

		l = &lscc.SCC{
			Support:     fakeSupport,
			SCCProvider: fakeSCCProvider,
			BCCSP:       cryptoProvider,
		}

		ccData = &ccprovider.ChaincodeData{
			Name:                "chaincode-data-name",
			Version:             "version",
			Escc:                "escc",
			Vscc:                "vscc",
			Policy:              []byte("policy"),
			Data:                []byte("data"),
			Id:                  []byte("id"),
			InstantiationPolicy: []byte("instantiation-policy"),
		}

		ccDataBytes, err = proto.Marshal(ccData)
		Expect(err).NotTo(HaveOccurred())

		fakeQueryExecutor = &mock.QueryExecutor{}
		fakeQueryExecutor.GetStateReturns(ccDataBytes, nil)
	})

	Describe("LegacySecurity", func() {
		var (
			fakeCCPackage  *mock.CCPackage
			legacySecurity *lscc.LegacySecurity
			packageCache   *lscc.PackageCache
		)

		BeforeEach(func() {
			fakeCCPackage = &mock.CCPackage{}
			fakeCCPackage.GetChaincodeDataReturns(&ccprovider.ChaincodeData{
				InstantiationPolicy: []byte("instantiation-policy"),
			})

			fakeSupport.GetChaincodeFromLocalStorageReturns(fakeCCPackage, nil)

			packageCache = &lscc.PackageCache{}

			legacySecurity = &lscc.LegacySecurity{
				Support:      fakeSupport,
				PackageCache: packageCache,
			}
		})

		It("returns nil if security checks are passed", func() {
			err := legacySecurity.SecurityCheckLegacyChaincode(ccData)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCCPackage.ValidateCCCallCount()).To(Equal(1))
			Expect(fakeCCPackage.ValidateCCArgsForCall(0)).To(Equal(ccData))
		})

		It("caches the result of the package validation", func() {
			err := legacySecurity.SecurityCheckLegacyChaincode(ccData)
			Expect(err).NotTo(HaveOccurred())

			Expect(packageCache.ValidatedPackages).To(HaveKey("chaincode-data-name:version"))
		})

		Context("when cc package validation fails", func() {
			BeforeEach(func() {
				fakeCCPackage.ValidateCCReturns(errors.New("fake-validation-error"))
			})

			It("returns an error", func() {
				err := legacySecurity.SecurityCheckLegacyChaincode(ccData)
				Expect(err).To(MatchError(lscc.InvalidCCOnFSError("fake-validation-error")))
			})

			It("does not cache the result of the package validation", func() {
				legacySecurity.SecurityCheckLegacyChaincode(ccData)

				Expect(packageCache.ValidatedPackages).NotTo(HaveKey("chaincode-data-name:version"))
			})
		})

		Context("when the instantiation policy doesn't match", func() {
			BeforeEach(func() {
				fakeCCPackage.GetChaincodeDataReturns(&ccprovider.ChaincodeData{
					InstantiationPolicy: []byte("bad-instantiation-policy"),
				})
			})

			It("returns an error", func() {
				err := legacySecurity.SecurityCheckLegacyChaincode(ccData)
				Expect(err).To(MatchError("Instantiation policy mismatch for cc chaincode-data-name:version"))
			})
		})
	})

	Describe("ChaincodeEndorsementInfo", func() {
		It("retrieves the chaincode data from the state", func() {
			chaincodeEndorsementInfo, err := l.ChaincodeEndorsementInfo("", "cc-name", fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			Expect(chaincodeEndorsementInfo).To(Equal(&lifecycle.ChaincodeEndorsementInfo{
				ChaincodeID:       "chaincode-data-name:version",
				Version:           "version",
				EndorsementPlugin: "escc",
			}))

			Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(1))
			namespace, key := fakeQueryExecutor.GetStateArgsForCall(0)
			Expect(namespace).To(Equal("lscc"))
			Expect(key).To(Equal("cc-name"))
		})

		Context("when the state getter fails", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, errors.New("fake-error"))
			})

			It("returns the wrapped error", func() {
				_, err := l.ChaincodeEndorsementInfo("", "cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("could not retrieve state for chaincode cc-name: fake-error"))
			})
		})

		Context("when the state getter does not find the key", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := l.ChaincodeEndorsementInfo("", "cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("chaincode cc-name not found"))
			})
		})

		Context("when the state getter returns invalid data", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns([]byte("garbage"), nil)
			})

			It("wraps and returns the error", func() {
				_, err := l.ChaincodeEndorsementInfo("", "cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError(MatchRegexp("chaincode cc-name has bad definition: proto:.*")))
			})
		})
	})

	Describe("ChaincodeDefinitionForValidation", func() {
		BeforeEach(func() {
		})

		It("retrieves the chaincode data from the state", func() {
			vscc, policy, unexpectedErr, validationErr := l.ValidationInfo("", "cc-name", fakeQueryExecutor)
			Expect(validationErr).NotTo(HaveOccurred())
			Expect(unexpectedErr).NotTo(HaveOccurred())
			Expect(vscc).To(Equal(ccData.Vscc))
			Expect(policy).To(Equal(ccData.Policy))

			Expect(fakeQueryExecutor.GetStateCallCount()).To(Equal(1))
			namespace, key := fakeQueryExecutor.GetStateArgsForCall(0)
			Expect(namespace).To(Equal("lscc"))
			Expect(key).To(Equal("cc-name"))
		})

		Context("when the state getter fails", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, errors.New("fake-error"))
			})

			It("returns the wrapped error", func() {
				_, _, unexpectedErr, validationErr := l.ValidationInfo("", "cc-name", fakeQueryExecutor)
				Expect(unexpectedErr).To(MatchError("could not retrieve state for chaincode cc-name: fake-error"))
				Expect(validationErr).NotTo(HaveOccurred())
			})
		})

		Context("when the state getter does not find the key", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				_, _, unexpectedErr, validationErr := l.ValidationInfo("", "cc-name", fakeQueryExecutor)
				Expect(unexpectedErr).NotTo(HaveOccurred())
				Expect(validationErr).To(MatchError("chaincode cc-name not found"))
			})
		})

		Context("when the state getter returns invalid data", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns([]byte("garbage"), nil)
			})

			It("wraps and returns the error", func() {
				_, _, unexpectedErr, validationErr := l.ValidationInfo("", "cc-name", fakeQueryExecutor)
				Expect(validationErr).NotTo(HaveOccurred())
				Expect(unexpectedErr).To(MatchError(MatchRegexp("chaincode cc-name has bad definition: proto:.*")))
			})
		})
	})
})
