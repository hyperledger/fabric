/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

func TestMissingArguments(t *testing.T) {
	gt := NewGomegaWithT(t)
	discover, err := Build("github.com/hyperledger/fabric/cmd/discover")
	gt.Expect(err).NotTo(HaveOccurred())
	defer CleanupBuildArtifacts()

	// key and cert flags are missing
	cmd := exec.Command(discover, "--configFile", "conf.yaml", "--MSP", "SampleOrg", "saveConfig")
	process, err := Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(process, 5*time.Second).Should(Exit(1))
	gt.Expect(process.Err).To(gbytes.Say("empty string that is mandatory"))
}
