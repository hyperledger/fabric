/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/deliver/mock"
	. "github.com/hyperledger/fabric/internal/test"
	oqs "github.com/hyperledger/fabric/pq-crypto"
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
		Expect(env).To(ProtoEqual(envelope))
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

		for range 5 {
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

		It("rechecks hybrid identity authorization on every block", func() {
			profile, err := oqs.HybridCertificateProfileByName(oqs.TemporaryHybridProfileV1, oqs.ProfileUseTest)
			Expect(err).NotTo(HaveOccurred())
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			Expect(err).NotTo(HaveOccurred())
			template := &x509.Certificate{
				SerialNumber: big.NewInt(1),
				Subject:      pkix.Name{CommonName: "hybrid deliver test"},
				NotBefore:    time.Now().Add(-time.Hour),
				NotAfter:     time.Now().Add(time.Hour),
				ExtraExtensions: []pkix.Extension{{
					Id: profile.ProfileKeyOID(), Value: []byte{0x30, 0x00},
				}},
			}
			certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
			Expect(err).NotTo(HaveOccurred())
			identity := protoutil.MarshalOrPanic(&mspprotos.SerializedIdentity{
				Mspid: "HybridMSP",
				IdBytes: pem.EncodeToMemory(&pem.Block{
					Type: "CERTIFICATE", Bytes: certDER,
				}),
			})
			Expect(crypto.IsHybridIdentity(identity)).To(BeTrue())
			envelope = &cb.Envelope{Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{Creator: identity})},
			})}
			fakePolicyChecker.CheckPolicyReturnsOnCall(0, nil)
			fakePolicyChecker.CheckPolicyReturnsOnCall(1, errors.New("hybrid certificate chain expired"))
			sac, err := deliver.NewSessionAC(fakeChain, envelope, fakePolicyChecker, "chain-id", expiresAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(sac.Evaluate()).To(Succeed())
			Expect(sac.Evaluate()).To(MatchError("hybrid certificate chain expired"))
			Expect(fakePolicyChecker.CheckPolicyCallCount()).To(Equal(2))
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
