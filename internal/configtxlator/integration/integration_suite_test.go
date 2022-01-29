/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var configtxlatorPath string

var _ = SynchronizedBeforeSuite(func() []byte {
	configtxlatorPath, err := gexec.Build("github.com/hyperledger/fabric/cmd/configtxlator")
	Expect(err).NotTo(HaveOccurred())

	return []byte(configtxlatorPath)
}, func(payload []byte) {
	configtxlatorPath = string(payload)
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
