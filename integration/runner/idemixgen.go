/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"os/exec"

	"github.com/tedsuo/ifrit/ginkgomon"
)

// Idemixgen creates runners that call idemixgen functions.
type Idemixgen struct {
	// Location of the idemixgen executable
	Path string
	// Output directory
	Output string
	// Enrollment ID for the default signer
	EnrollID string
	// The organizational unit for the default signer
	OrgUnit string
	// Flag for making the default signer an admin
	IsAdmin bool
	// Handle used to revoke the default signer
	RevocationHandle int
}

// CAKeyGen uses idemixgen to generate CA key material for an IdeMix MSP.
func (c *Idemixgen) CAKeyGen(extraArgs ...string) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name:          "idemix ca-keygen",
		AnsiColorCode: "38m",
		Command: exec.Command(
			c.Path,
			append([]string{
				"ca-keygen",
				"--output", c.Output,
			}, extraArgs...)...,
		),
	})
}

// SignerConfig uses idemixgen to generate a signer for an IdeMix MSP.
func (c *Idemixgen) SignerConfig(extraArgs ...string) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name:          "idemix signerconfig",
		AnsiColorCode: "38m",
		Command: exec.Command(
			c.Path,
			append([]string{
				"signerconfig",
				"-e", c.EnrollID,
				"-u", c.OrgUnit,
				"--output", c.Output,
			}, extraArgs...)...,
		),
	})
}
