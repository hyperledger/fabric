/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os/exec"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestArguments(t *testing.T) {
	testCases := map[string]struct {
		exitCode int
		args     []string
	}{
		"ledger": {
			exitCode: 0,
			args:     []string{},
		},
		"ledger-help": {
			exitCode: 0,
			args:     []string{"--help"},
		},
		"compare-help": {
			exitCode: 0,
			args:     []string{"compare", "--help"},
		},
		"compare": {
			exitCode: 1,
			args:     []string{"compare"},
		},
		"one-snapshot": {
			exitCode: 1,
			args:     []string{"compare", "snapshotDir1"},
		},
		"invalid-snapshot-dirs": {
			exitCode: 1,
			args:     []string{"compare", "/non-existent/snapshot1", "/non-existent/snapshot2"},
		},
		"identifytxs-help": {
			exitCode: 0,
			args:     []string{"identifytxs", "--help"},
		},
		"identifytxs": {
			exitCode: 1,
			args:     []string{"identifytxs"},
		},
		"verify-help": {
			exitCode: 0,
			args:     []string{"verify", "--help"},
		},
		"verify": {
			exitCode: 1,
			args:     []string{"verify"},
		},
	}

	// Build ledger binary
	gt := gomega.NewWithT(t)
	ledgerutil, err := gexec.Build("github.com/hyperledger/fabric/cmd/ledgerutil")
	gt.Expect(err).NotTo(gomega.HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			cmd := exec.Command(ledgerutil, testCase.args...)
			session, err := gexec.Start(cmd, nil, nil)
			gt.Expect(err).NotTo(gomega.HaveOccurred())
			gt.Eventually(session, 5*time.Second).Should(gexec.Exit(testCase.exitCode))
		})
	}
}
