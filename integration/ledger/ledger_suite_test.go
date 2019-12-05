/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

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

func TestLedger(t *testing.T) {
	RegisterFailHandler(Fail)

	if os.Getenv("ENABLE_JUNIT") == "true" {
		defaultReporter := reporters.NewDefaultReporter(config.DefaultReporterConfig, stenographer.New(!config.DefaultReporterConfig.NoColor, false, os.Stdout))
		junitReporter := reporters.NewJUnitReporter("integration_report.xml")
		RunSpecsWithCustomReporters(t, "Ledger Suite", []Reporter{defaultReporter, junitReporter})
	} else {
		RunSpecs(t, "Ledger Suite")
	}
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
)

var _ = SynchronizedBeforeSuite(func() []byte {
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
	return integration.LedgerPort.StartPortForNode()
}
