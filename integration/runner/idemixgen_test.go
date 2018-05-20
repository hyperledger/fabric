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
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("Idemixgen", func() {
	var (
		idemixgen *runner.Idemixgen
		tempDir   string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "idemix")
		Expect(err).NotTo(HaveOccurred())

		idemixgen = &runner.Idemixgen{
			Path:     components.Paths["idemixgen"],
			EnrollID: "IdeMixUser1",
			OrgUnit:  "IdeMixOU",
			Output:   tempDir,
		}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("CAKeyGen", func() {
		It("creates a runner that calls idemixgen ca-keygen", func() {
			igRunner := idemixgen.CAKeyGen()
			process := ifrit.Invoke(igRunner)
			Eventually(process.Ready()).Should(BeClosed())
			Eventually(process.Wait()).Should(Receive(BeNil()))
			Expect(igRunner.ExitCode()).To(Equal(0))

			Expect(filepath.Join(tempDir, "ca")).To(BeADirectory())
			Expect(filepath.Join(tempDir, "msp")).To(BeADirectory())
		})

		Context("when idemixgen ca-keygen fails", func() {
			It("returns an error", func() {
				igRunner := idemixgen.CAKeyGen("bogus")
				process := ifrit.Invoke(igRunner)
				Eventually(process.Wait()).Should(Receive(HaveOccurred()))
			})
		})
	})

	Describe("SignerConfig", func() {
		BeforeEach(func() {
			keygen := ginkgomon.Invoke(idemixgen.CAKeyGen())
			Eventually(keygen.Wait()).Should(Receive(BeNil()))
		})

		It("creates a runner that calls idemixgen signerconfig", func() {
			igRunner := idemixgen.SignerConfig()
			process := ifrit.Invoke(igRunner)
			Eventually(process.Ready()).Should(BeClosed())
			Eventually(process.Wait()).Should(Receive(BeNil()))
			Expect(igRunner.ExitCode()).To(Equal(0))

			Expect(filepath.Join(tempDir, "user")).To(BeADirectory())
		})

		Context("when idemixgen signerconfig fails", func() {
			It("returns an error", func() {
				igRunner := idemixgen.SignerConfig("bogus")
				process := ifrit.Invoke(igRunner)
				Eventually(process.Wait()).Should(Receive(HaveOccurred()))
			})
		})
	})
})
