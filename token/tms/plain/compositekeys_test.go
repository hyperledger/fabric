/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Composite keys", func() {
	var (
		namespace string
		txID      string
		index     int
		chaincode *shim.ChaincodeStub
	)

	BeforeEach(func() {
		namespace = "tms"
		txID = "tx0"
		index = 0
		chaincode = &shim.ChaincodeStub{}
	})

	Describe("Copied composite keys generator function", func() {
		Context("when a composite key for an output is generated", func() {
			It("the output is the same as from the function in the chaincode shim", func() {
				verifierKey, err := createCompositeKey(namespace, []string{txID, strconv.Itoa(index)})
				Expect(err).ToNot(HaveOccurred())
				shimKey, err := chaincode.CreateCompositeKey(namespace, []string{txID, strconv.Itoa(index)})
				Expect(err).ToNot(HaveOccurred())
				Expect(verifierKey).To(Equal(shimKey))
			})
		})

		Context("when a composite key for a transaction is generated", func() {
			It("the output is the same as from the function in the chaincode shim", func() {
				verifierKey, err := createCompositeKey(namespace, []string{txID})
				Expect(err).ToNot(HaveOccurred())
				shimKey, err := chaincode.CreateCompositeKey(namespace, []string{txID})
				Expect(err).ToNot(HaveOccurred())
				Expect(verifierKey).To(Equal(shimKey))
			})
		})

		Context("when a minRune namespace is passed", func() {
			It("the error string is the same as from the function in the chaincode shim", func() {
				_, err := createCompositeKey(string(0), []string{txID})
				Expect(err).To(HaveOccurred())
				_, shimErr := chaincode.CreateCompositeKey(string(0), []string{txID})
				Expect(shimErr).To(HaveOccurred())
				Expect(err.Error()).To(Equal(shimErr.Error()))
			})
		})

		Context("when a minRune txID is passed", func() {
			It("the error string is the same as from the function in the chaincode shim", func() {
				_, err := createCompositeKey(namespace, []string{string(0)})
				Expect(err).To(HaveOccurred())
				_, shimErr := chaincode.CreateCompositeKey(namespace, []string{string(0)})
				Expect(shimErr).To(HaveOccurred())
				Expect(err.Error()).To(Equal(shimErr.Error()))
			})
		})

		Context("when a txID with the MSB set is passed", func() {
			It("the error string is the same as from the function in the chaincode shim", func() {
				_, err := createCompositeKey(namespace, []string{string([]byte{0x80})})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("not a valid utf8 string: [80]"))
				_, shimErr := chaincode.CreateCompositeKey(namespace, []string{string([]byte{0x80})})
				Expect(shimErr).To(HaveOccurred())
				Expect(err.Error()).To(Equal(shimErr.Error()))
			})
		})
	})
})
