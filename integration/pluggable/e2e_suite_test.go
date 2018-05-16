/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/integration/world"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pluggable endorsement and validation EndToEnd Suite")
}

var (
	components                *world.Components
	testDir                   string
	endorsementPluginFilePath string
	validationPluginFilePath  string
)

var _ = SynchronizedBeforeSuite(func() []byte {
	components = &world.Components{}
	components.Build()

	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())

	testDir, err = ioutil.TempDir("", "e2e-suite")
	Expect(err).NotTo(HaveOccurred())

	// Compile plugins
	endorsementPluginFilePath = compilePlugin("endorsement")
	validationPluginFilePath = compilePlugin("validation")

	// Create directories for endorsement and validation activation
	dir := filepath.Join(testDir, "endorsement")
	err = os.Mkdir(dir, 0700)
	Expect(err).NotTo(HaveOccurred())
	SetEndorsementPluginActivationFolder(dir)

	dir = filepath.Join(testDir, "validation")
	err = os.Mkdir(dir, 0700)
	Expect(err).NotTo(HaveOccurred())
	SetValidationPluginActivationFolder(dir)
})

var _ = SynchronizedAfterSuite(func() {
	os.RemoveAll(testDir)
	os.Remove(endorsementPluginFilePath)
	os.RemoveAll(validationPluginFilePath)
}, func() {
	components.Cleanup()
})
