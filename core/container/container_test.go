/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/mock"
	"github.com/pkg/errors"
)

var _ = Describe("Container", func() {
	Describe("LockingVM", func() {
		var (
			fakeVM         *mock.VM
			lockingVM      *container.LockingVM
			containerLocks *container.ContainerLocks
		)

		BeforeEach(func() {
			fakeVM = &mock.VM{}
			containerLocks = container.NewContainerLocks()
			lockingVM = &container.LockingVM{
				Underlying:     fakeVM,
				ContainerLocks: containerLocks,
			}
		})

		Describe("Start", func() {
			BeforeEach(func() {
				fakeVM.StartReturns(errors.New("fake-start-error"))
			})

			It("passes through to the underlying impl", func() {
				err := lockingVM.Start(
					ccintf.CCID("start:name"),
					[]string{"foo", "bar"},
					[]string{"Bar", "Foo"},
					map[string][]byte{
						"Foo": []byte("bar"),
					},
				)

				Expect(err).To(MatchError("fake-start-error"))
				Expect(fakeVM.StartCallCount()).To(Equal(1))
				ccid, args, env, filesToUpload := fakeVM.StartArgsForCall(0)
				Expect(ccid).To(Equal(ccintf.CCID("start:name")))
				Expect(args).To(Equal([]string{"foo", "bar"}))
				Expect(env).To(Equal([]string{"Bar", "Foo"}))
				Expect(filesToUpload).To(Equal(map[string][]byte{
					"Foo": []byte("bar"),
				}))
			})
		})

		Describe("Stop", func() {
			BeforeEach(func() {
				fakeVM.StopReturns(errors.New("Boo"))
			})

			It("passes through to the underlying impl", func() {
				err := lockingVM.Stop(ccintf.CCID("stop:name"))
				Expect(err).To(MatchError("Boo"))
				Expect(fakeVM.StopCallCount()).To(Equal(1))
				Expect(fakeVM.StopArgsForCall(0)).To(Equal(ccintf.CCID("stop:name")))
			})
		})

		Describe("Build", func() {
			BeforeEach(func() {
				fakeVM.BuildReturns(errors.New("fake-build-error"))
			})

			It("passes through to the underlying impl", func() {
				err := lockingVM.Build(
					ccintf.CCID("stop:name"),
					"type",
					"path",
					"name",
					"version",
					[]byte("code-bytes"),
				)
				Expect(err).To(MatchError("fake-build-error"))
				Expect(fakeVM.BuildCallCount()).To(Equal(1))
				ccid, ccType, path, name, version, codePackage := fakeVM.BuildArgsForCall(0)
				Expect(ccid).To(Equal(ccintf.CCID("stop:name")))
				Expect(ccType).To(Equal("type"))
				Expect(path).To(Equal("path"))
				Expect(name).To(Equal("name"))
				Expect(version).To(Equal("version"))
				Expect(codePackage).To(Equal([]byte("code-bytes")))
			})
		})

		Describe("Wait", func() {
			BeforeEach(func() {
				fakeVM.WaitReturns(7, errors.New("fake-build-error"))
			})

			It("passes through to the underlying impl", func() {
				res, err := lockingVM.Wait(
					ccintf.CCID("stop:name"),
				)
				Expect(res).To(Equal(7))
				Expect(err).To(MatchError("fake-build-error"))
				Expect(fakeVM.WaitCallCount()).To(Equal(1))
				ccid := fakeVM.WaitArgsForCall(0)
				Expect(ccid).To(Equal(ccintf.CCID("stop:name")))
			})
		})
	})
})
