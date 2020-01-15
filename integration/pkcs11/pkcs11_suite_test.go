/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"encoding/json"
	"testing"

	bpkcs11 "github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPKCS11(t *testing.T) {
	RegisterFailHandler(Fail)
	lib, pin, label := bpkcs11.FindPKCS11Lib()
	if lib == "" || pin == "" || label == "" {
		t.Skip("Skipping PKCS11 Suite: Required ENV variables not set")
	}
	RunSpecs(t, "PKCS11 Suite")
}

var components *nwo.Components

var _ = SynchronizedBeforeSuite(func() []byte {
	components = &nwo.Components{}
	components.Build("-tags=pkcs11")

	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	components.Cleanup()
})

func BasePort() int {
	return 40000 + 1000*GinkgoParallelNode()
}
