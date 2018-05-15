/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"os/exec"

	"github.com/tedsuo/ifrit/ginkgomon"
)

// Cryptogen creates runners that call cryptogen functions.
type Cryptogen struct {
	// The location of the cryptogen executable
	Path string
	// The location of the config file
	Config string
	// The output directory
	Output string
}

// Generate uses cryptogen to generate cryptographic material for fabric.
func (c *Cryptogen) Generate(extraArgs ...string) *ginkgomon.Runner {
	return ginkgomon.New(ginkgomon.Config{
		Name:          "cryptogen generate",
		AnsiColorCode: "31m",
		Command: exec.Command(
			c.Path,
			append([]string{
				"generate",
				"--config", c.Config,
				"--output", c.Output,
			}, extraArgs...)...,
		),
	})
}
