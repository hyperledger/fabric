/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/server/mock"
)

var _ = Describe("AccessControl", func() {
	var (
		fakePolicyChecker *mock.SignedDataPolicyChecker
		pbac              *server.PolicyBasedAccessControl

		importRequest    *token.ImportRequest
		command          *token.Command
		marshaledCommand []byte
		signedCommand    *token.SignedCommand
	)

	BeforeEach(func() {
		fakePolicyChecker = &mock.SignedDataPolicyChecker{}
		pbac = &server.PolicyBasedAccessControl{
			SignedDataPolicyChecker: fakePolicyChecker,
		}

		importRequest = &token.ImportRequest{}
		command = &token.Command{
			Header: &token.Header{
				ChannelId: "channel-id",
				Creator:   []byte("creator"),
			},
			Payload: &token.Command_ImportRequest{
				ImportRequest: importRequest,
			},
		}

		marshaledCommand = ProtoMarshal(command)
		signedCommand = &token.SignedCommand{
			Command:   marshaledCommand,
			Signature: []byte("signature"),
		}
	})

	It("checks the policy", func() {
		err := pbac.Check(signedCommand, command)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakePolicyChecker.CheckPolicyBySignedDataCallCount()).To(Equal(1))
		channelID, policyName, signedData := fakePolicyChecker.CheckPolicyBySignedDataArgsForCall(0)
		Expect(channelID).To(Equal("channel-id"))
		Expect(policyName).To(Equal(policies.ChannelApplicationWriters))
		Expect(signedData).To(ConsistOf(&common.SignedData{
			Data:      marshaledCommand,
			Identity:  []byte("creator"),
			Signature: []byte("signature"),
		}))
	})

	Context("when the policy checker returns an error", func() {
		BeforeEach(func() {
			fakePolicyChecker.CheckPolicyBySignedDataReturns(errors.New("no-can-do"))
		})

		It("returns the error", func() {
			err := pbac.Check(signedCommand, command)
			Expect(err).To(MatchError("no-can-do"))
		})
	})

	Context("when the command payload type is not recognized", func() {
		BeforeEach(func() {
			command.Payload = nil
		})

		It("skips the access control check", func() {
			pbac.Check(signedCommand, command)
			Expect(fakePolicyChecker.CheckPolicyBySignedDataCallCount()).To(Equal(0))
		})

		It("returns a error", func() {
			err := pbac.Check(signedCommand, command)
			Expect(err).To(MatchError("command type not recognized: <nil>"))
		})
	})
})
