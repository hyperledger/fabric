/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package client_test

import (
	"io"

	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/token/client"
	"github.com/hyperledger/fabric/token/client/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("OrdererClient", func() {
	var (
		broadcastResp *ab.BroadcastResponse

		fakeSigner    *mock.SignerIdentity
		fakeBroadcast *mock.Broadcast
	)

	BeforeEach(func() {
		fakeSigner = &mock.SignerIdentity{}
		fakeSigner.SignReturns([]byte("envelop-signature"), nil)

		broadcastResp = &ab.BroadcastResponse{Status: common.Status_SUCCESS}
		fakeBroadcast = &mock.Broadcast{}
		fakeBroadcast.SendReturns(nil)
		fakeBroadcast.CloseSendReturns(nil)
		fakeBroadcast.RecvReturnsOnCall(0, broadcastResp, nil)
		fakeBroadcast.RecvReturnsOnCall(1, nil, io.EOF)

	})

	Describe("BroadcastSend", func() {
		var envelope *common.Envelope

		BeforeEach(func() {
			envelope = &common.Envelope{
				Payload:   []byte("envelope-payload"),
				Signature: []byte("envelop-signature"),
			}
		})

		It("returns without error", func() {
			err := client.BroadcastSend(fakeBroadcast, "dummyAddress", envelope)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when Broadcast.Send returns error", func() {
			BeforeEach(func() {
				fakeBroadcast.SendReturns(errors.New("flying-pineapple"))
			})

			It("returns an error", func() {
				err := client.BroadcastSend(fakeBroadcast, "dummyAddress", envelope)
				Expect(err.Error()).To(ContainSubstring("flying-pineapple"))
			})
		})
	})

	Describe("BroadcastReceive", func() {
		var (
			responses chan common.Status
			errs      chan error
		)

		BeforeEach(func() {
			responses = make(chan common.Status)
			errs = make(chan error, 1)
		})

		It("returns with success status", func() {
			go client.BroadcastReceive(fakeBroadcast, "dummyAddress", responses, errs)
			status, err := client.BroadcastWaitForResponse(responses, errs)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(common.Status_SUCCESS))
		})

		Context("when Broadcast.Recv returns error", func() {
			BeforeEach(func() {
				fakeBroadcast.RecvReturnsOnCall(1, nil, errors.New("flying-banana"))
			})

			It("returns an error", func() {
				go client.BroadcastReceive(fakeBroadcast, "dummyAddress", responses, errs)
				_, err := client.BroadcastWaitForResponse(responses, errs)
				Expect(err.Error()).To(ContainSubstring("flying-banana"))
			})
		})
	})
})
