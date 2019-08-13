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
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
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
		fakeLifecycle                  *mock.Lifecycle
		fakeSimulator                  *mock.TxSimulator

		txParams *ccprovider.TransactionParams
		input    *pb.ChaincodeInput
	)

	BeforeEach(func() {
		fakeApplicationCapabilities = &mock.ApplicationCapabilities{}
		fakeApplicationCapabilities.LifecycleV20Returns(true)

		fakeApplicationConfig = &mock.ApplicationConfig{}
		fakeApplicationConfig.CapabilitiesReturns(fakeApplicationCapabilities)

		fakeApplicationConfigRetriever = &fake.ApplicationConfigRetriever{}
		fakeApplicationConfigRetriever.GetApplicationConfigReturns(fakeApplicationConfig, true)

		fakeLifecycle = &mock.Lifecycle{}
		fakeSimulator = &mock.TxSimulator{}
		fakeSimulator.GetStateReturns([]byte("old-cc-version"), nil)

		txParams = &ccprovider.TransactionParams{
			ChannelID:   "channel-id",
			TXSimulator: fakeSimulator,
		}

		input = &pb.ChaincodeInput{
			IsInit: true,
		}

		chaincodeSupport = &chaincode.ChaincodeSupport{
			AppConfig: fakeApplicationConfigRetriever,
			Lifecycle: fakeLifecycle,
		}
	})

	Describe("CheckInvocation", func() {
		Context("when the definition is a _lifecycle definition", func() {
			BeforeEach(func() {
				fakeLifecycle.ChaincodeDefinitionReturns(&lifecycle.LegacyDefinition{
					Name:              "definition-name",
					Version:           "cc-version",
					RequiresInitField: true,
					CCIDField:         "definition-ccid",
				}, nil)
			})

			It("fetches the definition, skips the security checks, and does the init checks", func() {
				ccid, cctype, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
				Expect(err).NotTo(HaveOccurred())
				Expect(ccid).To(Equal("definition-ccid"))
				Expect(cctype).To(Equal(pb.ChaincodeMessage_INIT))

				Expect(fakeLifecycle.ChaincodeDefinitionCallCount()).To(Equal(1))
				channelID, chaincodeName, txSim := fakeLifecycle.ChaincodeDefinitionArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(chaincodeName).To(Equal("test-chaincode-name"))
				Expect(txSim).To(Equal(fakeSimulator))
			})

			Context("when the definition does not require init", func() {
				BeforeEach(func() {
					fakeLifecycle.ChaincodeDefinitionReturns(&lifecycle.LegacyDefinition{
						Name:              "definition-name",
						Version:           "cc-version",
						RequiresInitField: false,
						CCIDField:         "definition-ccid",
					}, nil)
				})

				It("returns a normal transaction type", func() {
					ccid, cctype, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
					Expect(err).NotTo(HaveOccurred())
					Expect(ccid).To(Equal("definition-ccid"))
					Expect(cctype).To(Equal(pb.ChaincodeMessage_TRANSACTION))
				})
			})
		})

		Context("when the definition is a legacy definition", func() {
			var (
				fakeLegacyDefinition *mock.LegacyChaincodeDefinition
			)

			BeforeEach(func() {
				fakeLegacyDefinition = &mock.LegacyChaincodeDefinition{}

				ccDef := struct {
					*ccprovider.ChaincodeData
					*mock.LegacyChaincodeDefinition
				}{
					ChaincodeData: &ccprovider.ChaincodeData{
						Name:    "definition-name",
						Version: "cc-version",
						Id:      []byte("id"),
					},
					LegacyChaincodeDefinition: fakeLegacyDefinition,
				}

				fakeLifecycle.ChaincodeDefinitionReturns(ccDef, nil)
			})

			It("fetches the definition, performs security checks, and skips the init checks", func() {
				ccid, cctype, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
				Expect(err).NotTo(HaveOccurred())
				Expect(ccid).To(Equal("definition-name:cc-version"))
				Expect(cctype).To(Equal(pb.ChaincodeMessage_TRANSACTION))

				Expect(fakeLifecycle.ChaincodeDefinitionCallCount()).To(Equal(1))
				channelID, chaincodeName, txSim := fakeLifecycle.ChaincodeDefinitionArgsForCall(0)
				Expect(channelID).To(Equal("channel-id"))
				Expect(chaincodeName).To(Equal("test-chaincode-name"))
				Expect(txSim).To(Equal(fakeSimulator))

				Expect(fakeLegacyDefinition.ExecuteLegacySecurityChecksCallCount()).To(Equal(1))
			})

			Context("when the legacy security check fails", func() {
				BeforeEach(func() {
					fakeLegacyDefinition.ExecuteLegacySecurityChecksReturns(fmt.Errorf("fake-security-error"))
				})

				It("wraps and returns the error", func() {
					_, _, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
					Expect(err).To(MatchError("[channel channel-id] failed the chaincode security checks for test-chaincode-name: fake-security-error"))
				})
			})
		})

		Context("when lifecycle returns an error", func() {
			BeforeEach(func() {
				fakeLifecycle.ChaincodeDefinitionReturns(nil, fmt.Errorf("fake-lifecycle-error"))
			})

			It("wraps and returns the error", func() {
				_, _, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
				Expect(err).To(MatchError("[channel channel-id] failed to get chaincode container info for test-chaincode-name: fake-lifecycle-error"))
			})
		})
	})

	Describe("CheckInit", func() {
		It("indicates that it is init", func() {
			isInit, err := chaincodeSupport.CheckInit(txParams, "cc-name", "cc-version", input)
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
			It("returns an error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, "cc-name", "old-cc-version", input)
				Expect(err).To(MatchError("chaincode 'cc-name' is already initialized but called as init"))
			})

			Context("when the invocation is not 'init'", func() {
				BeforeEach(func() {
					input.IsInit = false
				})

				It("returns that it is not an init", func() {
					isInit, err := chaincodeSupport.CheckInit(txParams, "cc-name", "old-cc-version", input)
					Expect(err).NotTo(HaveOccurred())
					Expect(isInit).To(BeFalse())
					Expect(fakeSimulator.GetStateCallCount()).To(Equal(1))
					Expect(fakeSimulator.SetStateCallCount()).To(Equal(0))
				})
			})
		})

		Context("when the new lifecycle is not enabled", func() {
			BeforeEach(func() {
				fakeApplicationCapabilities.LifecycleV20Returns(false)
			})

			It("returns that it is not init", func() {
				isInit, err := chaincodeSupport.CheckInit(txParams, "cc-name", "cc-version", input)
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
				isInit, err := chaincodeSupport.CheckInit(txParams, "cc-name", "cc-version", input)
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
				_, err := chaincodeSupport.CheckInit(txParams, "cc-name", "cc-version", input)
				Expect(err).To(MatchError("could not retrieve application config for channel 'channel-id'"))
			})
		})

		Context("when the invocation is not 'init'", func() {
			BeforeEach(func() {
				input.IsInit = false
			})

			It("returns an error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, "cc-name", "cc-version", input)
				Expect(err).To(MatchError("chaincode 'cc-name' has not been initialized for this version, must call as init first"))
			})
		})

		Context("when the txsimulator cannot get state", func() {
			BeforeEach(func() {
				fakeSimulator.GetStateReturns(nil, fmt.Errorf("get-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, "cc-name", "cc-version", input)
				Expect(err).To(MatchError("could not get 'initialized' key: get-state-error"))
			})
		})

		Context("when the txsimulator cannot set state", func() {
			BeforeEach(func() {
				fakeSimulator.SetStateReturns(fmt.Errorf("set-state-error"))
			})

			It("wraps and returns the error", func() {
				_, err := chaincodeSupport.CheckInit(txParams, "cc-name", "cc-version", input)
				Expect(err).To(MatchError("could not set 'initialized' key: set-state-error"))
			})
		})
	})
})
