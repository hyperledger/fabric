/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"fmt"
	"unicode/utf8"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CheckInvocation", func() {
	var (
		chaincodeSupport *chaincode.ChaincodeSupport

		fakeLifecycle *mock.Lifecycle
		fakeSimulator *mock.TxSimulator

		txParams *ccprovider.TransactionParams
		input    *pb.ChaincodeInput
	)

	BeforeEach(func() {
		fakeLifecycle = &mock.Lifecycle{}
		fakeSimulator = &mock.TxSimulator{}
		fakeSimulator.GetStateReturns([]byte("old-cc-version"), nil)

		txParams = &ccprovider.TransactionParams{
			ChannelID:   "channel-id",
			TXSimulator: fakeSimulator,
		}

		input = &pb.ChaincodeInput{}

		chaincodeSupport = &chaincode.ChaincodeSupport{
			Lifecycle: fakeLifecycle,
		}
	})

	Describe("CheckInvocation for a non-legacy (_lifecycle) chaincode definition", func() {
		var (
			chaincodeDefinition *lifecycle.LegacyDefinition
		)

		BeforeEach(func() {
			chaincodeDefinition = &lifecycle.LegacyDefinition{
				Version:          "definition-version",
				ChaincodeIDField: "definition-ccid",
			}

			fakeLifecycle.ChaincodeDefinitionReturns(chaincodeDefinition, nil)
		})

		It("fetches the definition and skips the legacy security checks", func() {
			ccid, cctype, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
			Expect(err).NotTo(HaveOccurred())
			Expect(ccid).To(Equal("definition-ccid"))
			Expect(cctype).To(Equal(pb.ChaincodeMessage_TRANSACTION))

			Expect(fakeLifecycle.ChaincodeDefinitionCallCount()).To(Equal(1))
			channelID, chaincodeName, txSim := fakeLifecycle.ChaincodeDefinitionArgsForCall(0)
			Expect(channelID).To(Equal("channel-id"))
			Expect(chaincodeName).To(Equal("test-chaincode-name"))
			Expect(txSim).To(Equal(fakeSimulator))
		})

		Context("when the invocation is an init", func() {
			BeforeEach(func() {
				input.IsInit = true
			})

			It("returns an error for chaincodes which do not require init", func() {
				_, _, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
				Expect(err).To(MatchError("chaincode 'test-chaincode-name' does not require initialization but called as init"))
			})

			Context("when the chaincode requires init", func() {
				BeforeEach(func() {
					chaincodeDefinition.RequiresInitField = true
				})

				It("enforces init exactly once semantics", func() {
					ccid, cctype, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
					Expect(err).NotTo(HaveOccurred())
					Expect(ccid).To(Equal("definition-ccid"))
					Expect(cctype).To(Equal(pb.ChaincodeMessage_INIT))

					Expect(fakeSimulator.GetStateCallCount()).To(Equal(1))
					namespace, key := fakeSimulator.GetStateArgsForCall(0)
					Expect(namespace).To(Equal("test-chaincode-name"))
					Expect(key).To(Equal("\x00" + string(utf8.MaxRune) + "initialized"))

					Expect(fakeSimulator.SetStateCallCount()).To(Equal(1))
					namespace, key, value := fakeSimulator.SetStateArgsForCall(0)
					Expect(namespace).To(Equal("test-chaincode-name"))
					Expect(key).To(Equal("\x00" + string(utf8.MaxRune) + "initialized"))
					Expect(value).To(Equal([]byte("definition-version")))
				})

				Context("when the invocation is not an init", func() {
					BeforeEach(func() {
						input.IsInit = false
					})

					It("returns an error", func() {
						_, _, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
						Expect(err).To(MatchError("chaincode 'test-chaincode-name' has not been initialized for this version, must call as init first"))
					})
				})

				Context("when the chaincode is already initialized", func() {
					BeforeEach(func() {
						fakeSimulator.GetStateReturns([]byte("definition-version"), nil)
					})

					It("returns an error", func() {
						_, _, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
						Expect(err).To(MatchError("chaincode 'test-chaincode-name' is already initialized but called as init"))
					})
				})

				Context("when the txsimulator cannot get state", func() {
					BeforeEach(func() {
						fakeSimulator.GetStateReturns(nil, fmt.Errorf("get-state-error"))
					})

					It("wraps and returns the error", func() {
						_, _, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
						Expect(err).To(MatchError("could not get 'initialized' key: get-state-error"))
					})
				})

				Context("when the txsimulator cannot set state", func() {
					BeforeEach(func() {
						fakeSimulator.SetStateReturns(fmt.Errorf("set-state-error"))
					})

					It("wraps and returns the error", func() {
						_, _, err := chaincodeSupport.CheckInvocation(txParams, "test-chaincode-name", input)
						Expect(err).To(MatchError("could not set 'initialized' key: set-state-error"))
					})
				})
			})

		})
	})

	Describe("CheckInvocation for a legacy (lscc) chaincode definition", func() {
		var (
			fakeLegacyDefinition *mock.LegacyChaincodeDefinition
		)

		BeforeEach(func() {
			input.IsInit = false
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
