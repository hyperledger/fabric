/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/tedsuo/ifrit"
)

var _ = Describe("CryptoGen", func() {
	var cryptogen *runner.Cryptogen
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "crypto")
		Expect(err).NotTo(HaveOccurred())

		cryptogen = &runner.Cryptogen{
			Path:   components.Paths["cryptogen"],
			Config: filepath.Join("testdata", "cryptogen-config.yaml"),
			Output: tempDir,
		}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("Generate", func() {
		It("creates a runner that calls cryptogen", func() {
			cgRunner := cryptogen.Generate()
			process := ifrit.Invoke(cgRunner)
			Eventually(process.Ready()).Should(BeClosed())
			Eventually(process.Wait()).Should(Receive(BeNil()))
			Expect(cgRunner.ExitCode()).To(Equal(0))

			Expect(filepath.Join(tempDir, "ordererOrganizations")).To(BeADirectory())
			Expect(filepath.Join(tempDir, "peerOrganizations")).To(BeADirectory())
		})

		Context("when cryptogen fails", func() {
			It("returns an error", func() {
				cgRunner := cryptogen.Generate("bogus")
				process := ifrit.Invoke(cgRunner)
				Eventually(process.Wait()).Should(Receive(HaveOccurred()))
			})
		})
	})

})
