/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"fmt"
	"unicode/utf8"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/fake"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChaincodeSupport", func() {
	var (
		chaincodeSupport *chaincode.ChaincodeSupport

		fakeApplicationConfigRetriever *fake.ApplicationConfigRetriever
		fakeApplicationConfig          *mock.ApplicationConfig
		fakeApplicationCapabilities    *mock.ApplicationCapabilities
	)

	BeforeEach(func() {
		fakeApplicationCapabilities = &mock.ApplicationCapabilities{}
		fakeApplicationCapabilities.LifecycleV20Returns(true)

		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeApplicationConfig.CapabilitiesReturns(fakeApplicationCapabilities)

		fakeApplicationConfigRetriever = &fake.ApplicationConfigRetriever{}
		fakeApplicationConfigRetriever.GetApplicationConfigReturns(fakeApplicationConfig, true)

		chaincodeSupport = &chaincode.ChaincodeSupport{
			AppConfig: fakeApplicationConfigRetriever,
		}
	})

	Describe("CheckInit", func() {
		var (
			txParams *ccprovider.TransactionParams
			cccid    *ccprovider.CCContext
			input    *pb.ChaincodeInput

			fakeSimulator *mock.TxSimulator
		)

		BeforeEach(func() {
			fakeSimulator = &mock.TxSimulator{}
			fakeSimulator.GetStateReturns([]byte("old-cc-version"), nil)

			txParams = &ccprovider.TransactionParams{
				ChannelID:   "channel-id",
				TXSimulator: fakeSimulator,
			}

			cccid = &ccprovider.CCContext{
				Name:         "cc-name",
				Version:      "cc-version",
				InitRequired: true,
			}

			input = &pb.ChaincodeInput{
				IsInit: true,
			}
		})

		It("indicates that it is init", func() {
			isInit, err := chaincodeSupport.CheckInit(txParams, cccid, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(isInit).To(BeTrue())

			Expect(fakeSimulator.GetStateCallCount()).To(Equal(1))
			namespace, key := fakeSimulator.GetStateArgsForCall(0)
			Expect(namespace).To(Equal("cc-name"))
			Expect(key).To(Equal("\x00" + string(utf8.MaxRune) + "initialized"))

			Expect(fakeSimulator.SetStateCallCount()).To(Equal(1))
			namespace, key, value := fakeSimulator.SetStateArgsForCall(0)
			Expect(namespace).To(Equal("cc-name"))
			Expect(key).To(Equal("\x00" + string(utf8.MaxRune) + "initialized"))
			Expect(value).To(Equal([]byte("cc-version")))
		})

		Context("when the version is not changed", func() {
			BeforeEach(func() {
				cccid.Version = "old-cc-version"
			})

			It("returns an error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).To(MatchError("chaincode 'cc-name' is already initialized but called as init"))
			})

			Context("when the invocation is not 'init'", func() {
				BeforeEach(func() {
					input.IsInit = false
				})

				It("returns that it is not an init", func() {
					isInit, err := chaincodeSupport.CheckInit(txParams, cccid, input)
					Expect(err).NotTo(HaveOccurred())
					Expect(isInit).To(BeFalse())
					Expect(fakeSimulator.GetStateCallCount()).To(Equal(1))
					Expect(fakeSimulator.SetStateCallCount()).To(Equal(0))
				})
			})
		})

		Context("when init is not required", func() {
			BeforeEach(func() {
				cccid.InitRequired = false
			})

			It("returns that it is not init", func() {
				isInit, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(isInit).To(BeFalse())
				Expect(fakeSimulator.GetStateCallCount()).To(Equal(0))
				Expect(fakeSimulator.SetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the new lifecycle is not enabled", func() {
			BeforeEach(func() {
				fakeApplicationCapabilities.LifecycleV20Returns(false)
			})

			It("returns that it is not init", func() {
				isInit, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(isInit).To(BeFalse())
				Expect(fakeSimulator.GetStateCallCount()).To(Equal(0))
				Expect(fakeSimulator.SetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the invocation is channel-less", func() {
			BeforeEach(func() {
				txParams.ChannelID = ""
			})

			It("returns it is not an init", func() {
				isInit, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(isInit).To(BeFalse())
				Expect(fakeSimulator.GetStateCallCount()).To(Equal(0))
				Expect(fakeSimulator.SetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the application config cannot be retrieved", func() {
			BeforeEach(func() {
				fakeApplicationConfigRetriever.GetApplicationConfigReturns(nil, false)
			})

			It("returns an error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).To(MatchError("could not retrieve application config for channel 'channel-id'"))
			})
		})

		Context("when the invocation is not 'init'", func() {
			BeforeEach(func() {
				input.IsInit = false
			})

			It("returns an error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).To(MatchError("chaincode 'cc-name' has not been initialized for this version, must call as init first"))
			})
		})

		Context("when the txsimulator cannot get state", func() {
			BeforeEach(func() {
				fakeSimulator.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).To(MatchError("could not get 'initialized' key: get-state-error"))
			})
		})

		Context("when the txsimulator cannot set state", func() {
			BeforeEach(func() {
				fakeSimulator.SetStateReturns(fmt.Errorf("set-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, cccid, input)
				Expect(err).To(MatchError("could not set 'initialized' key: set-state-error"))
			})
		})
	})
})
