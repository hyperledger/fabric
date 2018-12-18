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

type Extend struct {
	Config string
	Input  string
}

func (c Extend) SessionName() string {
	return "cryptogen-extend"
}

func (c Extend) Args() []string {
	return []string{
		"extend",
		"--config", c.Config,
		"--input", c.Input,
	}
}
