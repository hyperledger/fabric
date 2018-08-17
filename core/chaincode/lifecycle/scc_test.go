/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/chaincode/shim"
)

var _ = Describe("SCC", func() {
	var (
		scc *lifecycle.SCC
	)

	BeforeEach(func() {
		scc = &lifecycle.SCC{}
	})

	Describe("Name", func() {
		It("returns the name", func() {
			Expect(scc.Name()).To(Equal("+lifecycle"))
		})
	})

	Describe("Path", func() {
		It("returns the path", func() {
			Expect(scc.Path()).To(Equal("github.com/hyperledger/fabric/core/chaincode/lifecycle"))
		})
	})

	Describe("InitArgs", func() {
		It("returns no args", func() {
			Expect(scc.InitArgs()).To(BeNil())
		})
	})

	Describe("Chaincode", func() {
		It("returns a reference to itself", func() {
			Expect(scc.Chaincode()).To(Equal(scc))
		})
	})

	Describe("InvokableExternal", func() {
		It("is invokable externally", func() {
			Expect(scc.InvokableExternal()).To(BeTrue())
		})
	})

	Describe("InvokableCC2CC", func() {
		It("is invokable chaincode to chaincode", func() {
			Expect(scc.InvokableCC2CC()).To(BeTrue())
		})
	})

	Describe("Enabled", func() {
		It("is enabled", func() {
			Expect(scc.Enabled()).To(BeTrue())
		})
	})

	Describe("Init", func() {
		It("does nothing", func() {
			Expect(scc.Init(nil)).To(Equal(shim.Success(nil)))
		})
	})

	Describe("Invoke", func() {
		var (
			fakeStub *mock.ChaincodeStub
		)

		BeforeEach(func() {
			fakeStub = &mock.ChaincodeStub{}
		})

		Context("when no arguments are provided", func() {
			It("returns an error", func() {
				Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("lifecycle scc must be invoked with arguments")))
			})
		})

		Context("when an unknown function is provided as the first argument", func() {
			BeforeEach(func() {
				fakeStub.GetArgsReturns([][]byte{[]byte("bad-function")})
			})

			It("returns an error", func() {
				Expect(scc.Invoke(fakeStub)).To(Equal(shim.Error("unknown lifecycle function: bad-function")))
			})
		})
	})
})
