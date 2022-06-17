/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestArguements(t *testing.T) {

	testCases := map[string]struct {
		exitCode int
		args     []string
	}{
		"toofew1": {
			exitCode: 1,
			args:     []string{"metadatadir"},
		},
		"toofew2": {
			exitCode: 1,
			args:     []string{"wibble", "wibble"},
		},
		"twomany": {
			exitCode: 1,
			args:     []string{"wibble", "wibble", "wibble", "wibble"},
		},
	}

	// Build ledger binary
	gt := NewWithT(t)
	buildCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/build")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			cmd := exec.Command(buildCmd, testCase.args...)
			session, err := gexec.Start(cmd, nil, nil)
			gt.Expect(err).NotTo(HaveOccurred())
			gt.Eventually(session, 5*time.Second).Should(gexec.Exit(testCase.exitCode))
		})
	}
}

func TestGoodPath(t *testing.T) {
	gt := NewWithT(t)

	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/build")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	testPath := t.TempDir()

	// create a basic structure of a chaincode
	os.MkdirAll(path.Join(testPath, "in-builder-dir", "META-INF"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "in-metadata-dir"), 0755)
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

	metadataJson := path.Join(testPath, "in-metadata-dir", "metadata.json")
	fileMetadata, err := os.Create(metadataJson)
	gt.Expect(err).NotTo(HaveOccurred())
	fileMetadata.WriteString(`{
		  "type":"ccaas"
		}`)

	indexJson := path.Join(testPath, "in-builder-dir", "META-INF", "indexes.json")
	fileIndex, err := os.Create(indexJson)
	gt.Expect(err).NotTo(HaveOccurred())
	fileIndex.WriteString(`{
		  "couchindexes": "gohere"
		}`)

	args := []string{path.Join(testPath, "in-builder-dir"), path.Join(testPath, "in-metadata-dir"), path.Join(testPath, "out-release-dir")}
	cmd := exec.Command(releaseCmd, args...)
	session, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(session, 5*time.Second).Should(gexec.Exit(0))

	// check that the files have been copied
	chkfile := path.Join(testPath, "out-release-dir", "connection.json")
	_, err = os.Stat(chkfile)
	gt.Expect(err).NotTo(HaveOccurred())

	chkfile = path.Join(testPath, "out-release-dir", "META-INF", "indexes.json")
	_, err = os.Stat(chkfile)
	gt.Expect(err).NotTo(HaveOccurred())
}

func TestTemplating(t *testing.T) {
	gt := NewWithT(t)

	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/build")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	testPath := t.TempDir()

	// create a basic structure of the chaincode to use
	os.MkdirAll(path.Join(testPath, "in-builder-dir", "META-INF"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "in-metadata-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "out-release-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	connectionJson := path.Join(testPath, "in-builder-dir", "connection.json")
	file, err := os.Create(connectionJson)
	gt.Expect(err).NotTo(HaveOccurred())
	file.WriteString(`{
		"address": "{{.address}}:9999",
		"dial_timeout": "10s",
		"tls_required": true,
		"root_cert":"{{.root_cert}}",
		"client_key":"{{.client_key}}",
		"client_cert":"{{.client_cert}}",
		"client_auth_required":true
	  }`)

	metadataJson := path.Join(testPath, "in-metadata-dir", "metadata.json")
	fileMetadata, err := os.Create(metadataJson)
	gt.Expect(err).NotTo(HaveOccurred())
	fileMetadata.WriteString(`{
		  "type":"ccaas"
		}`)

	// call 'build' with the environment set for the templating to be tested
	args := []string{path.Join(testPath, "in-builder-dir"), path.Join(testPath, "in-metadata-dir"), path.Join(testPath, "out-release-dir")}
	cmd := exec.Command(releaseCmd, args...)
	env := os.Environ()
	env = append(env, `CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG={"address":"URLAddress","root_cert":"a root cert","client_key":"a client key","client_cert":"a client cert"}`)
	cmd.Env = env

	session, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(session, 5*time.Second).Should(gexec.Exit(0))

	// check that the files have been copied
	chkfile := path.Join(testPath, "out-release-dir", "connection.json")
	_, err = os.Stat(chkfile)
	gt.Expect(err).NotTo(HaveOccurred())

	// check that the file has the exepected contents
	connectionFileContents, err := ioutil.ReadFile(chkfile)
	gt.Expect(err).NotTo(HaveOccurred())

	expectedJson := `{
		"address": "URLAddress:9999",
		"dial_timeout": "10s",
		"tls_required": true,
		"root_cert":"a root cert",
		"client_key":"a client key",
		"client_cert":"a client cert",
		"client_auth_required":true
	  }`

	gt.Expect(string(connectionFileContents)).To(MatchJSON(expectedJson))
}

func TestTemplatingFailure(t *testing.T) {
	gt := NewWithT(t)

	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/build")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	testPath := t.TempDir()

	// create a basic structure of the chaincode to use
	os.MkdirAll(path.Join(testPath, "in-builder-dir", "META-INF"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "in-metadata-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "out-release-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	connectionJson := path.Join(testPath, "in-builder-dir", "connection.json")
	file, err := os.Create(connectionJson)
	gt.Expect(err).NotTo(HaveOccurred())
	file.WriteString(`{
		"address": "{{.address}}:9999",
		"dial_timeout": "10s",
		"tls_required": true,
		"root_cert":"{{.root_cert}}",
		"client_key":"{{.client_key}}",
		"client_cert":"{{.client_cert}}",
		"client_auth_required":true
	  }`)

	metadataJson := path.Join(testPath, "in-metadata-dir", "metadata.json")
	fileMetadata, err := os.Create(metadataJson)
	gt.Expect(err).NotTo(HaveOccurred())
	fileMetadata.WriteString(`{
		  "type":"ccaas"
		}`)

	// call 'build' with the environment set for the templating to be tested
	args := []string{path.Join(testPath, "in-builder-dir"), path.Join(testPath, "in-metadata-dir"), path.Join(testPath, "out-release-dir")}
	cmd := exec.Command(releaseCmd, args...)
	env := os.Environ()
	env = append(env, `CHAINCODE_AS_A_SERVICE_BUILDER_CONFIG={"address":"URLAddress","root_cert":"a root cert"}`)
	cmd.Env = env

	session, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(session, 5*time.Second).Should(gexec.Exit(1))
	gt.Expect(session.Err).To(gbytes.Say("Failed to parse the ClientKey field template"))
}

func TestMissingConnection(t *testing.T) {
	gt := NewWithT(t)

	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/build")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	testPath := t.TempDir()

	// create a basic structure of a chaincode
	os.MkdirAll(path.Join(testPath, "in-builder-dir", "META-INF"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "in-metadata-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "out-release-dir"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	metadataJson := path.Join(testPath, "in-metadata-dir", "metadata.json")
	fileMetadata, err := os.Create(metadataJson)
	gt.Expect(err).NotTo(HaveOccurred())
	fileMetadata.WriteString(`{
		  "type":"ccaas"
		}`)

	indexJson := path.Join(testPath, "in-builder-dir", "META-INF", "indexes.json")
	fileIndex, err := os.Create(indexJson)
	gt.Expect(err).NotTo(HaveOccurred())
	fileIndex.WriteString(`{
		  "couchindexes": "gohere"
		}`)

	args := []string{path.Join(testPath, "in-builder-dir"), path.Join(testPath, "in-metadata-dir"), path.Join(testPath, "out-release-dir")}
	cmd := exec.Command(releaseCmd, args...)
	session, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(session, 5*time.Second).Should(gexec.Exit(1))
	gt.Expect(session.Err).To(gbytes.Say("connection.json not found"))
}

func TestMissingMetadata(t *testing.T) {
	gt := NewWithT(t)

	releaseCmd, err := gexec.Build("github.com/hyperledger/fabric/ccaas_builder/cmd/build")
	gt.Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	testPath := t.TempDir()

	// create a basic structure of a chaincode
	os.MkdirAll(path.Join(testPath, "in-builder-dir", "META-INF"), 0755)
	gt.Expect(err).NotTo(HaveOccurred())

	os.MkdirAll(path.Join(testPath, "in-metadata-dir"), 0755)
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

	indexJson := path.Join(testPath, "in-builder-dir", "META-INF", "indexes.json")
	fileIndex, err := os.Create(indexJson)
	gt.Expect(err).NotTo(HaveOccurred())
	fileIndex.WriteString(`{
		  "couchindexes": "gohere"
		}`)

	args := []string{path.Join(testPath, "in-builder-dir"), path.Join(testPath, "in-metadata-dir"), path.Join(testPath, "out-release-dir")}
	cmd := exec.Command(releaseCmd, args...)
	session, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(session, 5*time.Second).Should(gexec.Exit(1))
	gt.Expect(session.Err).To(gbytes.Say("metadata.json not found"))
}
