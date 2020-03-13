/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type CAKeyGen struct {
	Output string
}

func (c CAKeyGen) SessionName() string {
	return "idemixgen-ca-key-gen"
}

func (c CAKeyGen) Args() []string {
	return []string{
		"ca-keygen",
		"--output", c.Output,
	}
}

type SignerConfig struct {
	CAInput          string
	Output           string
	OrgUnit          string
	Admin            bool
	EnrollmentID     string
	RevocationHandle string
}

func (c SignerConfig) SessionName() string {
	return "idemixgen-signerconfig"
}

func (c SignerConfig) Args() []string {
	return []string{
		"signerconfig",
		"--ca-input", c.CAInput,
		"--output", c.Output,
		"--admin",
		"-u", c.OrgUnit,
		"-e", c.EnrollmentID,
		"-r", c.RevocationHandle,
	}
}
