/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
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

// The store type is not a command line argument, so the commands take a path
// only. A path is handed to the tool, while a store type is refused while the
// command line is still being parsed.
func TestStoreTypeIsNotAnArgument(t *testing.T) {
	testCases := map[string]struct {
		args           []string
		expectedStderr string
	}{
		"identifytxs-path": {
			args:           []string{"identifytxs", "diffs.json"},
			expectedStderr: identifytxsErrorMessage,
		},
		"identifytxs-path-and-store-type": {
			args:           []string{"identifytxs", "diffs.json", "fsPath", "goleveldb"},
			expectedStderr: "unexpected goleveldb",
		},
		"verify-path": {
			args:           []string{"verify", "fsPath"},
			expectedStderr: verifyErrorMessage,
		},
		"verify-path-and-store-type": {
			args:           []string{"verify", "fsPath", "goleveldb"},
			expectedStderr: "unexpected goleveldb",
		},
	}

	gt := gomega.NewWithT(t)
	ledgerutil, err := gexec.Build("github.com/hyperledger/fabric/cmd/ledgerutil")
	gt.Expect(err).NotTo(gomega.HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			gt := gomega.NewWithT(t)
			// The tool is always given a path that does not exist, so it
			// always fails, but only once the command line has been parsed.
			command, path, storeType := testCase.args[0], testCase.args[1], testCase.args[2:]
			args := append(
				[]string{command, filepath.Join(t.TempDir(), path)},
				storeType...,
			)

			var stdout, stderr bytes.Buffer
			session, err := gexec.Start(exec.Command(ledgerutil, args...), &stdout, &stderr)
			gt.Expect(err).NotTo(gomega.HaveOccurred())
			gt.Eventually(session, 5*time.Second).Should(gexec.Exit(1))
			gt.Expect(stdout.String() + stderr.String()).To(gomega.ContainSubstring(testCase.expectedStderr))
		})
	}
}

// The store type is not a command line argument, so the usage of the commands
// lists a path only.
func TestUsageTakesAPathOnly(t *testing.T) {
	testCases := map[string]struct {
		command string
		// The whole usage line, so that an argument in addition to the path
		// does not go unnoticed.
		expectedUsage string
	}{
		"identifytxs": {
			command:       "identifytxs",
			expectedUsage: "usage: ledgerutil identifytxs [<flags>] <snapshotDiffsPath> [<blockStorePath>]",
		},
		"verify": {
			command:       "verify",
			expectedUsage: "usage: ledgerutil verify [<flags>] [<blockStorePath>]",
		},
	}

	gt := gomega.NewWithT(t)
	ledgerutil, err := gexec.Build("github.com/hyperledger/fabric/cmd/ledgerutil")
	gt.Expect(err).NotTo(gomega.HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			gt := gomega.NewWithT(t)
			var stderr bytes.Buffer
			session, err := gexec.Start(exec.Command(ledgerutil, testCase.command, "--help"), nil, &stderr)
			gt.Expect(err).NotTo(gomega.HaveOccurred())
			gt.Eventually(session, 5*time.Second).Should(gexec.Exit(0))
			usage, _, _ := strings.Cut(stderr.String(), "\n")
			gt.Expect(usage).To(gomega.Equal(testCase.expectedUsage))
		})
	}
}
