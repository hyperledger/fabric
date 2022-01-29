/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/container"
)

var _ = Describe("BuildRegistry", func() {
	var br *container.BuildRegistry

	BeforeEach(func() {
		br = &container.BuildRegistry{}
	})

	It("returns a new build status when one does not exist", func() {
		bs, ok := br.BuildStatus("ccid")
		Expect(ok).To(BeFalse())
		Expect(bs).NotTo(BeNil())
		Expect(bs.Done()).NotTo(BeClosed())
	})

	When("the ccid is already building", func() {
		var initialBS *container.BuildStatus

		BeforeEach(func() {
			var ok bool
			initialBS, ok = br.BuildStatus("ccid")
			Expect(ok).To(BeFalse())
		})

		It("returns the existing build status", func() {
			newBS, ok := br.BuildStatus("ccid")
			Expect(ok).To(BeTrue())
			Expect(newBS).To(Equal(initialBS))
		})
	})

	When("a previous build status had an error", func() {
		BeforeEach(func() {
			bs, ok := br.BuildStatus("ccid")
			Expect(ok).To(BeFalse())
			Expect(bs).NotTo(BeNil())
			bs.Notify(fmt.Errorf("fake-error"))
			Expect(bs.Done()).To(BeClosed())
			Expect(bs.Err()).To(MatchError(fmt.Errorf("fake-error")))
		})

		It("can be reset", func() {
			bs := br.ResetBuildStatus("ccid")
			Expect(bs).NotTo(BeNil())
			Expect(bs.Done()).NotTo(BeClosed())
			Expect(bs.Err()).To(BeNil())
		})
	})
})

var _ = Describe("BuildStatus", func() {
	var bs *container.BuildStatus

	BeforeEach(func() {
		bs = container.NewBuildStatus()
	})

	It("has a blocking done channel", func() {
		Expect(bs.Done()).NotTo(BeClosed())
	})

	When("notify is called with nil", func() {
		BeforeEach(func() {
			bs.Notify(nil)
		})

		It("closes the blocking done channel", func() {
			Expect(bs.Done()).To(BeClosed())
		})

		It("leaves err set to nil", func() {
			Expect(bs.Err()).To(BeNil())
		})
	})

	When("notify is called with an error", func() {
		BeforeEach(func() {
			bs.Notify(fmt.Errorf("fake-error"))
		})

		It("closes the blocking done channel", func() {
			Expect(bs.Done()).To(BeClosed())
		})

		It("sets err to the error", func() {
			Expect(bs.Err()).To(MatchError(fmt.Errorf("fake-error")))
		})
	})
})
