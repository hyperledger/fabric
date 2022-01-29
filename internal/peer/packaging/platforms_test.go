/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package packaging_test

import (
	"errors"

	"github.com/hyperledger/fabric/internal/peer/packaging"
	"github.com/hyperledger/fabric/internal/peer/packaging/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Platforms", func() {
	var (
		registry     *packaging.Registry
		fakePlatform *mock.Platform
	)

	BeforeEach(func() {
		fakePlatform = &mock.Platform{}
		registry = &packaging.Registry{
			Platforms: map[string]packaging.Platform{
				"fakeType": fakePlatform,
			},
		}
	})

	Describe("pass through functions", func() {
		Describe("ValidateSpec", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.ValidatePathReturns(errors.New("fake-error"))
				err := registry.ValidateSpec("fakeType", "cc-path")
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.ValidatePathCallCount()).To(Equal(1))
				Expect(fakePlatform.ValidatePathArgsForCall(0)).To(Equal("cc-path"))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					err := registry.ValidateSpec("badType", "")
					Expect(err).To(MatchError("unknown chaincodeType: badType"))
				})
			})
		})

		Describe("ValidateDeploymentSpec", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.ValidateCodePackageReturns(errors.New("fake-error"))
				err := registry.ValidateDeploymentSpec("fakeType", []byte("code-package"))
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.ValidateCodePackageCallCount()).To(Equal(1))
				Expect(fakePlatform.ValidateCodePackageArgsForCall(0)).To(Equal([]byte("code-package")))
			})

			Context("when the code package is empty", func() {
				It("does nothing", func() {
					err := registry.ValidateDeploymentSpec("fakeType", []byte{})
					Expect(err).NotTo(HaveOccurred())
					Expect(fakePlatform.ValidateCodePackageCallCount()).To(Equal(0))

					err = registry.ValidateDeploymentSpec("fakeType", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(fakePlatform.ValidateCodePackageCallCount()).To(Equal(0))
				})
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					err := registry.ValidateDeploymentSpec("badType", nil)
					Expect(err).To(MatchError("unknown chaincodeType: badType"))
				})
			})
		})

		Describe("GetDeploymentPayload", func() {
			It("returns the result of the underlying platform", func() {
				fakePlatform.GetDeploymentPayloadReturns([]byte("payload"), errors.New("fake-error"))
				payload, err := registry.GetDeploymentPayload("fakeType", "cc-path")
				Expect(payload).To(Equal([]byte("payload")))
				Expect(err).To(MatchError(errors.New("fake-error")))
				Expect(fakePlatform.GetDeploymentPayloadCallCount()).To(Equal(1))
				Expect(fakePlatform.GetDeploymentPayloadArgsForCall(0)).To(Equal("cc-path"))
			})

			Context("when the platform is unknown", func() {
				It("returns an error", func() {
					payload, err := registry.GetDeploymentPayload("badType", "")
					Expect(payload).To(BeNil())
					Expect(err).To(MatchError("unknown chaincodeType: badType"))
				})
			})
		})
	})

	Describe("NewRegistry", func() {
		It("initializes with the known platform types and util writer", func() {
			fakePlatformFoo := &mock.Platform{}
			fakePlatformFoo.NameReturns("foo")
			fakePlatformBar := &mock.Platform{}
			fakePlatformBar.NameReturns("bar")

			registry = packaging.NewRegistry(fakePlatformFoo, fakePlatformBar)

			Expect(registry.Platforms).To(Equal(map[string]packaging.Platform{
				"foo": fakePlatformFoo,
				"bar": fakePlatformBar,
			}))
		})

		Context("when two platforms report the same name", func() {
			It("panics", func() {
				fakePlatformFoo1 := &mock.Platform{}
				fakePlatformFoo1.NameReturns("foo")
				fakePlatformFoo2 := &mock.Platform{}
				fakePlatformFoo2.NameReturns("foo")
				Expect(func() { packaging.NewRegistry(fakePlatformFoo1, fakePlatformFoo2) }).To(Panic())
			})
		})
	})
})
