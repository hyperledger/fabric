/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type Generate struct {
	Config string
	Output string
}

func (c Generate) SessionName() string {
	return "cryptogen-generate"
}

func (c Generate) Args() []string {
	return []string{
		"generate",
		"--config", c.Config,
		"--output", c.Output,
	}
}
