/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package container_test

import (
	"fmt"

	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/mock"
	pb "github.com/hyperledger/fabric/protos/peer"
	"golang.org/x/net/context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Container", func() {
	Describe("StartContainerReq", func() {
		var (
			ctxt     = context.Background()
			startReq *container.StartContainerReq
			fakeVM   *mock.VM
		)

		BeforeEach(func() {
			startReq = &container.StartContainerReq{
				CCID: ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "start-name"}}},
				Args: []string{"foo", "bar"},
				Env:  []string{"Bar", "Foo"},
				FilesToUpload: map[string][]byte{
					"Foo": []byte("bar"),
				},
				Builder: &mock.Builder{},
			}
			fakeVM = &mock.VM{}
		})

		Describe("Do", func() {
			It("Returns a response with no error when things go fine", func() {
				fakeVM.StartReturns(nil)
				err := startReq.Do(ctxt, fakeVM)
				Expect(err).NotTo(HaveOccurred())
				Expect(fakeVM.StartCallCount()).To(Equal(1))
				rctxt, ccid, args, env, filesToUpload, builder := fakeVM.StartArgsForCall(0)
				Expect(rctxt).To(Equal(ctxt))
				Expect(ccid).To(Equal(startReq.CCID))
				Expect(args).To(Equal(startReq.Args))
				Expect(env).To(Equal(startReq.Env))
				Expect(filesToUpload).To(Equal(startReq.FilesToUpload))
				Expect(builder).To(Equal(startReq.Builder))
			})

			It("Returns an error when the vm does", func() {
				err := fmt.Errorf("Boo")
				fakeVM.StartReturns(err)
				rerr := startReq.Do(ctxt, fakeVM)
				Expect(rerr).To(Equal(err))
			})
		})

		Describe("GetCCID", func() {
			It("Returns the CCID embedded in the structure", func() {
				Expect(startReq.GetCCID()).To(Equal(startReq.CCID))
			})
		})
	})

	Describe("StopContainerReq", func() {
		var (
			ctxt    = context.Background()
			stopReq *container.StopContainerReq
			fakeVM  *mock.VM
		)

		BeforeEach(func() {
			stopReq = &container.StopContainerReq{
				CCID:       ccintf.CCID{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: "stop-name"}}},
				Timeout:    283,
				Dontkill:   true,
				Dontremove: false,
			}
			fakeVM = &mock.VM{}
		})

		Describe("Do", func() {
			It("Returns a response with no error when things go fine", func() {
				fakeVM.StartReturns(nil)
				resp := stopReq.Do(ctxt, fakeVM)
				Expect(resp).To(BeNil())
				Expect(fakeVM.StopCallCount()).To(Equal(1))
				rctxt, ccid, timeout, dontKill, dontRemove := fakeVM.StopArgsForCall(0)
				Expect(rctxt).To(Equal(ctxt))
				Expect(ccid).To(Equal(stopReq.CCID))
				Expect(timeout).To(Equal(stopReq.Timeout))
				Expect(dontKill).To(Equal(stopReq.Dontkill))
				Expect(dontRemove).To(Equal(stopReq.Dontremove))
			})

			It("Returns an error when the vm does", func() {
				err := fmt.Errorf("Boo")
				fakeVM.StopReturns(err)
				rerr := stopReq.Do(ctxt, fakeVM)
				Expect(rerr).To(Equal(err))
			})
		})

		Describe("GetCCID", func() {
			It("Returns the CCID embedded in the structure", func() {
				Expect(stopReq.GetCCID()).To(Equal(stopReq.CCID))
			})
		})
	})
})
