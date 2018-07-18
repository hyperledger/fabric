/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container_test

import (
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/mock"
	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Container", func() {
	Describe("VMCReqs", func() {
		var (
			fakeVM *mock.VM
		)

		BeforeEach(func() {
			fakeVM = &mock.VM{}
		})

		Describe("StartContainerReq", func() {
			var (
				startReq *container.StartContainerReq
			)

			BeforeEach(func() {
				startReq = &container.StartContainerReq{
					CCID: ccintf.CCID{Name: "start-name"},
					Args: []string{"foo", "bar"},
					Env:  []string{"Bar", "Foo"},
					FilesToUpload: map[string][]byte{
						"Foo": []byte("bar"),
					},
					Builder: &mock.Builder{},
				}
			})

			Describe("Do", func() {
				It("starts a vm", func() {
					err := startReq.Do(fakeVM)
					Expect(err).NotTo(HaveOccurred())
					Expect(fakeVM.StartCallCount()).To(Equal(1))
					ccid, args, env, filesToUpload, builder := fakeVM.StartArgsForCall(0)
					Expect(ccid).To(Equal(ccintf.CCID{Name: "start-name"}))
					Expect(args).To(Equal([]string{"foo", "bar"}))
					Expect(env).To(Equal([]string{"Bar", "Foo"}))
					Expect(filesToUpload).To(Equal(map[string][]byte{
						"Foo": []byte("bar"),
					}))
					Expect(builder).To(Equal(&mock.Builder{}))
				})

				Context("when the vm provider fails", func() {
					It("returns the error", func() {
						fakeVM.StartReturns(errors.New("Boo"))
						err := startReq.Do(fakeVM)
						Expect(err).To(MatchError("Boo"))
					})
				})
			})

			Describe("GetCCID", func() {
				It("Returns the CCID embedded in the structure", func() {
					Expect(startReq.GetCCID()).To(Equal(ccintf.CCID{Name: "start-name"}))
				})
			})
		})

		Describe("StopContainerReq", func() {
			var (
				stopReq *container.StopContainerReq
			)

			BeforeEach(func() {
				stopReq = &container.StopContainerReq{
					CCID:       ccintf.CCID{Name: "stop-name"},
					Timeout:    283,
					Dontkill:   true,
					Dontremove: false,
				}
			})

			Describe("Do", func() {
				It("stops the vm", func() {
					resp := stopReq.Do(fakeVM)
					Expect(resp).To(BeNil())
					Expect(fakeVM.StopCallCount()).To(Equal(1))
					ccid, timeout, dontKill, dontRemove := fakeVM.StopArgsForCall(0)
					Expect(ccid).To(Equal(ccintf.CCID{Name: "stop-name"}))
					Expect(timeout).To(Equal(uint(283)))
					Expect(dontKill).To(Equal(true))
					Expect(dontRemove).To(Equal(false))
				})

				Context("when the vm provider fails", func() {
					It("returns the error", func() {
						fakeVM.StopReturns(errors.New("Boo"))
						err := stopReq.Do(fakeVM)
						Expect(err).To(MatchError("Boo"))
					})
				})
			})

			Describe("GetCCID", func() {
				It("Returns the CCID embedded in the structure", func() {
					Expect(stopReq.GetCCID()).To(Equal(ccintf.CCID{Name: "stop-name"}))
				})
			})
		})
	})

	Describe("VMController", func() {
		var (
			vmProvider   *mock.VMProvider
			vmController *container.VMController
			vmcReq       *mock.VMCReq
		)

		BeforeEach(func() {
			vmProvider = &mock.VMProvider{}
			vmController = container.NewVMController(map[string]container.VMProvider{
				"FakeProvider": vmProvider,
			})
			vmProvider.NewVMReturns(&mock.VM{})
			vmcReq = &mock.VMCReq{}
		})

		Describe("Process", func() {
			It("completes the request using the correct vm provider", func() {
				err := vmController.Process("FakeProvider", vmcReq)
				Expect(vmProvider.NewVMCallCount()).To(Equal(1))
				Expect(err).NotTo(HaveOccurred())
			})

			Context("the request is for an unknown VM provider type", func() {
				It("causes the system to halt as this is a serious bug", func() {
					Expect(func() { vmController.Process("Unknown-Type", nil) }).To(Panic())
					Expect(vmProvider.NewVMCallCount()).To(Equal(0))
				})
			})
		})
	})
})
