/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"os"
	"os/exec"
)

type Command interface {
	Args() []string
	SessionName() string
}

type Enver interface {
	Env() []string
}

type WorkingDirer interface {
	WorkingDir() string
}

func ConnectsToOrderer(c Command) bool {
	for _, arg := range c.Args() {
		if arg == "--orderer" {
			return true
		}
	}
	return false
}

func NewCommand(path string, command Command) *exec.Cmd {
	cmd := exec.Command(path, command.Args()...)
	cmd.Env = os.Environ()
	if ce, ok := command.(Enver); ok {
		cmd.Env = append(cmd.Env, ce.Env()...)
	}
	if wd, ok := command.(WorkingDirer); ok {
		cmd.Dir = wd.WorkingDir()
	}
	return cmd
}
