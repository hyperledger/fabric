/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lscc_test

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/core/scc/lscc/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LSCC", func() {
	var (
		l                 *lscc.LifeCycleSysCC
		fakeSupport       *mock.FileSystemSupport
		fakeSCCProvider   *mock.SystemChaincodeProvider
		fakeQueryExecutor *mock.QueryExecutor
		ccData            *ccprovider.ChaincodeData
		ccDataBytes       []byte
		err               error
	)

	BeforeEach(func() {
		fakeSupport = &mock.FileSystemSupport{}
		fakeSCCProvider = &mock.SystemChaincodeProvider{}
		fakeQueryExecutor = &mock.QueryExecutor{}

		l = &lscc.LifeCycleSysCC{
			Support:     fakeSupport,
			SCCProvider: fakeSCCProvider,
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
		)

		BeforeEach(func() {
			fakeCCPackage = &mock.CCPackage{}
			fakeCCPackage.GetChaincodeDataReturns(&ccprovider.ChaincodeData{
				InstantiationPolicy: []byte("instantiation-policy"),
			})

			fakeSupport.GetChaincodeFromLocalStorageReturns(fakeCCPackage, nil)

			legacySecurity = &lscc.LegacySecurity{
				ChaincodeData: ccData,
				Support:       fakeSupport,
			}
		})

		It("returns nil if security checks are passed", func() {
			err := legacySecurity.SecurityCheckLegacyChaincode()
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCCPackage.ValidateCCCallCount()).To(Equal(1))
			Expect(fakeCCPackage.ValidateCCArgsForCall(0)).To(Equal(ccData))
		})

		Context("when cc package validation fails", func() {
			BeforeEach(func() {
				fakeCCPackage.ValidateCCReturns(errors.New("fake-validation-error"))
			})

			It("returns an error", func() {
				err := legacySecurity.SecurityCheckLegacyChaincode()
				Expect(err).To(MatchError(lscc.InvalidCCOnFSError("fake-validation-error")))
			})
		})

		Context("when the instantiation policy doesn't match", func() {
			BeforeEach(func() {
				fakeCCPackage.GetChaincodeDataReturns(&ccprovider.ChaincodeData{
					InstantiationPolicy: []byte("bad-instantiation-policy"),
				})
			})

			It("returns an error", func() {
				err := legacySecurity.SecurityCheckLegacyChaincode()
				Expect(err).To(MatchError("Instantiation policy mismatch for cc chaincode-data-name:version"))
			})
		})
	})

	Describe("ChaincodeDefinition", func() {
		It("retrieves the chaincode data from the state", func() {
			chaincodeDefinition, err := l.ChaincodeDefinition("", "cc-name", fakeQueryExecutor)
			Expect(err).NotTo(HaveOccurred())
			returnedChaincodeData, ok := chaincodeDefinition.(*lscc.LegacySecurity)
			Expect(ok).To(BeTrue())
			Expect(returnedChaincodeData.ChaincodeData).To(Equal(ccData))
			Expect(returnedChaincodeData.Support).To(Equal(fakeSupport))

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
				_, err := l.ChaincodeDefinition("", "cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("could not retrieve state for chaincode cc-name: fake-error"))
			})
		})

		Context("when the state getter does not find the key", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns(nil, nil)
			})

			It("returns an error", func() {
				_, err := l.ChaincodeDefinition("", "cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("chaincode cc-name not found"))
			})
		})

		Context("when the state getter returns invalid data", func() {
			BeforeEach(func() {
				fakeQueryExecutor.GetStateReturns([]byte("garbage"), nil)
			})

			It("wraps and returns the error", func() {
				_, err := l.ChaincodeDefinition("", "cc-name", fakeQueryExecutor)
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
