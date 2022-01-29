/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"io"

	lb "github.com/hyperledger/fabric-protos-go/peer/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/ledger"
	ledgermock "github.com/hyperledger/fabric/core/ledger/mock"
	. "github.com/onsi/ginkgo/v2"
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
		ebMetadata         *externalbuilder.MetadataProvider
	)

	BeforeEach(func() {
		fakeListener = &ledgermock.ChaincodeLifecycleEventListener{}
		chaincodeStore = &mock.ChaincodeStore{}
		pkgParser = &mock.PackageParser{}
		ebMetadata = &externalbuilder.MetadataProvider{
			DurablePath: "testdata",
		}
		eventBroker = lifecycle.NewEventBroker(chaincodeStore, pkgParser, ebMetadata)
		cachedChaincodeDef = &lifecycle.CachedChaincodeDefinition{}
		localChaincode = &lifecycle.LocalChaincode{
			Info: &lifecycle.ChaincodeInstallInfo{
				PackageID: "PackageID",
			},
			References: make(map[string]map[string]*lifecycle.CachedChaincodeDefinition),
		}
		eventBroker.RegisterListener("channel-1", fakeListener, nil)
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

		It("invokes listener when ProcessInstallEvent is called", func() {
			eventBroker.ProcessInstallEvent(localChaincode)
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
			def, md := fakeListener.HandleChaincodeDeployArgsForCall(0)
			Expect(def).To(Equal(&ledger.ChaincodeDefinition{
				Name:    "chaincode-1",
				Hash:    []byte("PackageID"),
				Version: "version-1",
			}))
			Expect(md).To(Equal([]byte("db-artifacts")))

			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
		})

		It("invokes listener when ProcessApproveOrDefineEvent is called", func() {
			eventBroker.ProcessApproveOrDefineEvent("channel-1", "chaincode-1", cachedChaincodeDef)
			Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(0))
			eventBroker.ApproveOrDefineCommitted("channel-1")
			Expect(fakeListener.ChaincodeDeployDoneCallCount()).To(Equal(1))
		})

		When("the metadata is defined by the external builders", func() {
			BeforeEach(func() {
				localChaincode.Info.PackageID = "external-built-cc"
			})

			It("does not invoke listener", func() {
				eventBroker.ProcessInstallEvent(localChaincode)
				Expect(fakeListener.HandleChaincodeDeployCallCount()).To(Equal(1))
				def, md := fakeListener.HandleChaincodeDeployArgsForCall(0)
				Expect(def).To(Equal(&ledger.ChaincodeDefinition{
					Name:    "chaincode-1",
					Hash:    []byte("external-built-cc"),
					Version: "version-1",
				}))

				mdContents := map[string]struct{}{}
				tr := tar.NewReader(bytes.NewBuffer(md))
				for {
					hdr, err := tr.Next()
					if err == io.EOF {
						break
					}
					Expect(err).NotTo(HaveOccurred())
					mdContents[hdr.Name] = struct{}{}
				}
				Expect(mdContents).To(HaveKey("META-INF/"))
				Expect(mdContents).To(HaveKey("META-INF/index.json"))
			})
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
