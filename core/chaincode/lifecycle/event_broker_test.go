/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	ccpersistence "github.com/hyperledger/fabric/core/chaincode/persistence/intf"
	ledgermock "github.com/hyperledger/fabric/core/ledger/mock"
	lb "github.com/hyperledger/fabric/protos/peer/lifecycle"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("EventBroker", func() {
	var (
		fakeListener       *ledgermock.ChaincodeLifecycleEventListener
		chaincodeStore     *mock.ChaincodeStore
		pkgParser          *mock.PackageParser
		eventBroker        *lifecycle.EventBroker
		cachedChaincodeDef *lifecycle.CachedChaincodeDefinition
		localChaincode     *lifecycle.LocalChaincode
	)

	BeforeEach(func() {
		fakeListener = &ledgermock.ChaincodeLifecycleEventListener{}
		chaincodeStore = &mock.ChaincodeStore{}
		pkgParser = &mock.PackageParser{}
		eventBroker = lifecycle.NewEventBroker(chaincodeStore, pkgParser)
		cachedChaincodeDef = &lifecycle.CachedChaincodeDefinition{}
		localChaincode = &lifecycle.LocalChaincode{
			Info: &lifecycle.ChaincodeInstallInfo{
				PackageID: ccpersistence.PackageID("PackageID"),
			},
			References: make(map[string]map[string]*lifecycle.CachedChaincodeDefinition),
		}
		eventBroker.RegisterListener("channel-1", fakeListener)
		pkgParser.ParseReturns(&persistence.ChaincodePackage{
			DBArtifacts: []byte("db-artifacts"),
		}, nil)
	})

	Context("when chaincode is only approved", func() {
		BeforeEach(func() {
			cachedChaincodeDef.Approved = true
		})

		It("does not invoke listener", func() {
			eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
			eventBroker.ApproveOrDefineCommitted("channel-1")
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
		})
	})

	Context("when chaincode is only defined", func() {
		BeforeEach(func() {
			cachedChaincodeDef.Definition = &lifecycle.ChaincodeDefinition{}
		})

		It("does not invoke listener", func() {
			eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
			eventBroker.ApproveOrDefineCommitted("channel-1")
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
		})
	})

	Context("when chaincode is approved and defined", func() {
		BeforeEach(func() {
			cachedChaincodeDef.Approved = true
			cachedChaincodeDef.Definition = &lifecycle.ChaincodeDefinition{}
		})

		It("does not invoke listener", func() {
			eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
			eventBroker.ApproveOrDefineCommitted("channel-1")
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
		})
	})

	Context("when chaincode is only installed", func() {
		It("does not invoke listener", func() {
			eventBroker.ProcessInstallEvent(localChaincode)
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
		})
	})

	Context("when chaincode is first installed and then defined", func() {
		BeforeEach(func() {
			cachedChaincodeDef.InstallInfo = &lifecycle.ChaincodeInstallInfo{}
			cachedChaincodeDef.Definition = &lifecycle.ChaincodeDefinition{}
		})

		It("does not invoke listener", func() {
			eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
			eventBroker.ApproveOrDefineCommitted("channel-1")
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
		})
	})

	Context("when chaincode is first defined and then installed", func() {
		BeforeEach(func() {
			cachedChaincodeDef.InstallInfo = &lifecycle.ChaincodeInstallInfo{}
			cachedChaincodeDef.Definition = &lifecycle.ChaincodeDefinition{}
			localChaincode.References["channel-1"] = map[string]*lifecycle.CachedChaincodeDefinition{
				"chaincode-1": cachedChaincodeDef,
			}
		})

		It("does not invoke listener", func() {
			eventBroker.ProcessInstallEvent(localChaincode)
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
		})
	})

	Context("when chaincode is approved, defined, and installed", func() {
		BeforeEach(func() {
			cachedChaincodeDef.Approved = true
			cachedChaincodeDef.Definition = &lifecycle.ChaincodeDefinition{
				EndorsementInfo: &lb.ChaincodeEndorsementInfo{
					Version: "version-1",
				},
			}
			cachedChaincodeDef.InstallInfo = localChaincode.Info
			localChaincode.References["channel-1"] = map[string]*lifecycle.CachedChaincodeDefinition{
				"chaincode-1": cachedChaincodeDef,
			}
		})

		By("invoking ProcessInstallEvent function")
		It("invokes listener", func() {
			eventBroker.ProcessInstallEvent(localChaincode)
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
		})

		By("invoking ProcessApproveOrDefineEvent function")
		It("invokes listener", func() {
			eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
			eventBroker.ApproveOrDefineCommitted("channel-1")
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
		})

		Context("when chaincode store returns error", func() {
			BeforeEach(func() {
				chaincodeStore.LoadReturns(nil, errors.New("loading-error"))
			})

			It("does not invoke listener", func() {
				eventBroker.ProcessInstallEvent(localChaincode)
				eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
				eventBroker.ApproveOrDefineCommitted("channel-1")
				Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
				Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
			})
		})

		Context("when chaincode package parser returns error", func() {
			BeforeEach(func() {
				pkgParser.ParseReturns(nil, errors.New("parsing-error"))
			})

			It("does not invoke listener", func() {
				eventBroker.ProcessInstallEvent(localChaincode)
				eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
				eventBroker.ApproveOrDefineCommitted("channel-1")
				Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(0))
				Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
			})
		})

		Context("when listener returns error", func() {
			BeforeEach(func() {
				fakeListener.HandleChaincodeDeployReturns(errors.New("listener-error"))
			})

			It("still invokes ChaincodeDeployDone() function on listener", func() {
				eventBroker.ProcessInstallEvent(localChaincode)
				Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
				Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))

				eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
				Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(2))
				Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
				eventBroker.ApproveOrDefineCommitted("channel-1")
				Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(2))
			})
		})
	})
})
