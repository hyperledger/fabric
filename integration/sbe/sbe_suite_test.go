/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sbe

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/ginkgo/reporters/stenographer"
	. "github.com/onsi/gomega"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)

	if os.Getenv("ENABLE_JUNIT") == "true" {
		defaultReporter := reporters.NewDefaultReporter(config.DefaultReporterConfig, stenographer.New(!config.DefaultReporterConfig.NoColor, false, os.Stdout))
		junitReporter := reporters.NewJUnitReporter("integration_report.xml")
		RunSpecsWithCustomReporters(t, "State-Based Endorsement EndToEnd Suite", []Reporter{defaultReporter, junitReporter})
	} else {
		RunSpecs(t, "State-Based Endorsement EndToEnd Suite")
	}
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
)

var _ = SynchronizedBeforeSuite(func() []byte {
	nwo.RequiredImages = []string{nwo.CCEnvDefaultImage}

	buildServer = nwo.NewBuildServer()
	buildServer.Serve()

	components = buildServer.Components()
	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	buildServer.Shutdown()
})

func StartPort() int {
	return integration.SBEBasePort.StartPortForNode()
}
