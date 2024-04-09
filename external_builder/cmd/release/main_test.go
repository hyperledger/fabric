/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestArugments(t *testing.T) {

	testCases := map[string]struct {
		exitCode int
		args     []string
	}{
		"toofew": {
			exitCode: 1,
			args:     []string{"metadatadir"},
		},
		"twomany": {
			exitCode: 1,
			args:     []string{"wibble", "wibble", "wibble"},
		},
	}

	// Build ledger binary
	gt := NewWithT(t)
	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/release")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			cmd := exec.Command(releaseCmd, testCase.args...)
			session, err := gexec.Start(cmd, nil, nil)
			gt.Expect(err).NotTo(HaveOccurred())
			gt.Eventually(session, 5*time.Second).Should(gexec.Exit(testCase.exitCode))
		})
	}
}

func TestGoodPath(t *testing.T) {
	gt := NewWithT(t)

	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/release")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	testPath := t.TempDir()

	os.MkdirAll(path.Join(testPath, "in-builder-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "out-release-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	connectionJson := path.Join(testPath, "in-builder-dir", "connection.json")
	file, err := os.Create(connectionJson)
	gt.Expect(err).NotTo(HaveOccurred())
	file.WriteString(`{
		"address": "audit-trail-ccaas:9999",
		"dial_timeout": "10s",
		"tls_required": false
	  }`)

	args := []string{path.Join(testPath, "in-builder-dir"), path.Join(testPath, "out-release-dir")}
	cmd := exec.Command(releaseCmd, args...)
	session, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(session, 5*time.Second).Should(gexec.Exit(0))

	// check that the files have been copied
	destConnectionJson := path.Join(testPath, "out-release-dir", "chaincode", "server", "connection.json")
	_, err = os.Stat(destConnectionJson)
	gt.Expect(err).NotTo(HaveOccurred())

}

func TestMissingConnection(t *testing.T) {
	gt := NewWithT(t)

	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/release")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	testPath := t.TempDir()

	os.MkdirAll(path.Join(testPath, "in-builder-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "out-release-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	args := []string{path.Join(testPath, "in-builder-dir"), path.Join(testPath, "out-release-dir")}
	cmd := exec.Command(releaseCmd, args...)
	session, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(session, 5*time.Second).Should(gexec.Exit(1))

	gt.Expect(session.Err).To(gbytes.Say("connection.json not found"))
}
