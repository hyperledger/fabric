/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package helpers

import (
	"fmt"
	"os/exec"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gexec"
)

func StartSession(cmd *exec.Cmd, name, ansiColorCode string) (*gexec.Session, error) {
	return gexec.Start(
		cmd,
		gexec.NewPrefixedWriter(fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name), ginkgo.GinkgoWriter),
		gexec.NewPrefixedWriter(fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name), ginkgo.GinkgoWriter),
	)
}
