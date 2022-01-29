/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver_test

import (
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("SessionAccessControl", func() {
	var (
		fakeChain         *mock.Chain
		envelope          *cb.Envelope
		fakePolicyChecker *mock.PolicyChecker
		expiresAt         deliver.ExpiresAtFunc
	)

	BeforeEach(func() {
		envelope = &cb.Envelope{
			Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{},
			}),
		}

		fakeChain = &mock.Chain{}
		fakePolicyChecker = &mock.PolicyChecker{}
		expiresAt = func([]byte) time.Time { return time.Time{} }
	})

	It("evaluates the policy", func() {
		sac, err := deliver.NewSessionAC(fakeChain, envelope, fakePolicyChecker, "chain-id", expiresAt)
		Expect(err).NotTo(HaveOccurred())

		err = sac.Evaluate()
		Expect(err).NotTo(HaveOccurred())

		Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(1))
		env, cid := fakePolicyChecker.CheckPolicyArgsForCall(0)
		Expect(env).To(Equal(envelope))
		Expect(cid).To(Equal("chain-id"))
	})

	Context("when policy evaluation returns an error", func() {
		BeforeEach(func() {
			fakePolicyChecker.CheckPolicyReturns(errors.New("no-access-for-you"))
		})

		It("returns the evaluation error", func() {
			sac, err := deliver.NewSessionAC(fakeChain, envelope, fakePolicyChecker, "chain-id", expiresAt)
			Expect(err).NotTo(HaveOccurred())

			err = sac.Evaluate()
			Expect(err).To(MatchError("no-access-for-you"))
		})
	})

	It("caches positive policy evaluation", func() {
		sac, err := deliver.NewSessionAC(fakeChain, envelope, fakePolicyChecker, "chain-id", expiresAt)
		Expect(err).NotTo(HaveOccurred())

		for i := 0; i < 5; i++ {
			err = sac.Evaluate()
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(1))
	})

	Context("when the config sequence changes", func() {
		BeforeEach(func() {
			fakePolicyChecker.CheckPolicyReturnsOnCall(2, errors.New("access-now-denied"))
		})

		It("re-evaluates the policy", func() {
			sac, err := deliver.NewSessionAC(fakeChain, envelope, fakePolicyChecker, "chain-id", expiresAt)
			Expect(err).NotTo(HaveOccurred())

			Expect(sac.Evaluate()).To(Succeed())
			Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(1))
			Expect(sac.Evaluate()).To(Succeed())
			Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(1))

			fakeChain.SequenceReturns(2)
			Expect(sac.Evaluate()).To(Succeed())
			Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(2))
			Expect(sac.Evaluate()).To(Succeed())
			Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(2))

			fakeChain.SequenceReturns(3)
			Expect(sac.Evaluate()).To(MatchError("access-now-denied"))
			Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(3))
		})
	})

	Context("when an identity expires", func() {
		BeforeEach(func() {
			expiresAt = func([]byte) time.Time {
				return time.Now().Add(250 * time.Millisecond)
			}
		})

		It("returns an identity expired error", func() {
			sac, err := deliver.NewSessionAC(fakeChain, envelope, fakePolicyChecker, "chain-id", expiresAt)
			Expect(err).NotTo(HaveOccurred())

			err = sac.Evaluate()
			Expect(err).NotTo(HaveOccurred())

			Eventually(sac.Evaluate).Should(MatchError(ContainSubstring("deliver client identity expired")))
		})
	})

	Context("when the envelope cannot be represented as signed data", func() {
		BeforeEach(func() {
			envelope = &cb.Envelope{}
		})

		It("returns an error", func() {
			_, expectedError := protoutil.EnvelopeAsSignedData(envelope)
			Expect(expectedError).To(HaveOccurred())

			_, err := deliver.NewSessionAC(fakeChain, envelope, fakePolicyChecker, "chain-id", expiresAt)
			Expect(err).To(Equal(expectedError))
		})
	})
})
