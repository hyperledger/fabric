/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/container"
)

var _ = Describe("Custodian", func() {
	var (
		cc            *lifecycle.ChaincodeCustodian
		fakeBuilder   *mock.ChaincodeBuilder
		fakeLauncher  *mock.ChaincodeLauncher
		buildRegistry *container.BuildRegistry
		doneC         chan struct{}
	)

	BeforeEach(func() {
		fakeBuilder = &mock.ChaincodeBuilder{}
		fakeBuilder.BuildReturnsOnCall(1, fmt.Errorf("fake-build-error"))
		fakeLauncher = &mock.ChaincodeLauncher{}
		buildRegistry = &container.BuildRegistry{}
		cc = lifecycle.NewChaincodeCustodian()
		doneC = make(chan struct{})
		go func() {
			cc.Work(buildRegistry, fakeBuilder, fakeLauncher)
			close(doneC)
		}()
	})

	AfterEach(func() {
		cc.Close()
		Eventually(doneC).Should(BeClosed())
	})

	It("builds chaincodes", func() {
		cc.NotifyInstalled("ccid1")
		cc.NotifyInstalled("ccid2")
		cc.NotifyInstalled("ccid3")
		Eventually(fakeBuilder.BuildCallCount).Should(Equal(3))
		Expect(fakeBuilder.BuildArgsForCall(0)).To(Equal("ccid1"))
		Expect(fakeBuilder.BuildArgsForCall(1)).To(Equal("ccid2"))
		Expect(fakeBuilder.BuildArgsForCall(2)).To(Equal("ccid3"))

		Expect(fakeLauncher.LaunchCallCount()).To(Equal(0))
	})

	It("notifies on the build status", func() {
		cc.NotifyInstalled("ccid1")
		cc.NotifyInstalled("ccid2")
		Eventually(fakeBuilder.BuildCallCount).Should(Equal(2))

		buildStatus, ok := buildRegistry.BuildStatus("ccid1")
		Expect(ok).To(BeTrue())
		Eventually(buildStatus.Done()).Should(BeClosed())
		Expect(buildStatus.Err()).To(BeNil())

		buildStatus, ok = buildRegistry.BuildStatus("ccid2")
		Expect(ok).To(BeTrue())
		Eventually(buildStatus.Done()).Should(BeClosed())
		Expect(buildStatus.Err()).To(MatchError("fake-build-error"))
	})

	When("the chaincode is being built already", func() {
		BeforeEach(func() {
			buildStatus, ok := buildRegistry.BuildStatus("ccid1")
			Expect(ok).To(BeFalse())
			buildStatus.Notify(nil)
		})

		It("skips the build", func() {
			cc.NotifyInstalled("ccid1")
			Consistently(fakeBuilder.BuildCallCount).Should(Equal(0))
		})
	})

	It("launches chaincodes", func() {
		cc.NotifyInstalledAndRunnable("ccid1")
		cc.NotifyInstalledAndRunnable("ccid2")
		cc.NotifyInstalledAndRunnable("ccid3")
		Eventually(fakeLauncher.LaunchCallCount).Should(Equal(3))
		ccid := fakeLauncher.LaunchArgsForCall(0)
		Expect(ccid).To(Equal("ccid1"))
		ccid = fakeLauncher.LaunchArgsForCall(1)
		Expect(ccid).To(Equal("ccid2"))
		ccid = fakeLauncher.LaunchArgsForCall(2)
		Expect(ccid).To(Equal("ccid3"))

		Expect(fakeBuilder.BuildCallCount()).To(Equal(0))
	})

	It("stops chaincodes", func() {
		cc.NotifyStoppable("ccid1")
		cc.NotifyStoppable("ccid2")
		cc.NotifyStoppable("ccid3")
		Eventually(fakeLauncher.StopCallCount).Should(Equal(3))
		ccid := fakeLauncher.StopArgsForCall(0)
		Expect(ccid).To(Equal("ccid1"))
		ccid = fakeLauncher.StopArgsForCall(1)
		Expect(ccid).To(Equal("ccid2"))
		ccid = fakeLauncher.StopArgsForCall(2)
		Expect(ccid).To(Equal("ccid3"))

		Expect(fakeBuilder.BuildCallCount()).To(Equal(0))
		Expect(fakeLauncher.LaunchCallCount()).To(Equal(0))
	})
})
