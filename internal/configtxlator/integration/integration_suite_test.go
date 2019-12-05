/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/ginkgo/reporters/stenographer"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	if os.Getenv("ENABLE_JUNIT") == "true" {
		defaultReporter := reporters.NewDefaultReporter(config.DefaultReporterConfig, stenographer.New(!config.DefaultReporterConfig.NoColor, false, os.Stdout))
		junitReporter := reporters.NewJUnitReporter("integration_report.xml")
		RunSpecsWithCustomReporters(t, "Integration Suite", []Reporter{defaultReporter, junitReporter})
	} else {
		RunSpecs(t, "Integration Suite")
	}
}

var configtxlatorPath string

var _ = SynchronizedBeforeSuite(func() []byte {
	configtxlatorPath, err := gexec.Build("github.com/hyperledger/fabric/cmd/configtxlator")
	Expect(err).NotTo(HaveOccurred())

	return []byte(configtxlatorPath)
}, func(payload []byte) {
	configtxlatorPath = string(payload)
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
