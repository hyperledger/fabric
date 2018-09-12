/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager", func() {
	Describe("GetIssuer", func() {
		It("returns a plain issuer", func() {
			Manager := &server.Manager{}
			issuer, err := Manager.GetIssuer("test-channel", []byte("private-credential"), []byte("public-credential"))
			Expect(err).NotTo(HaveOccurred())
			Expect(issuer).To(Equal(&plain.Issuer{}))
		})
	})
})
