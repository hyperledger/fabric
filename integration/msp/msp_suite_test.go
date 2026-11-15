/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"encoding/json/v2"
	"testing"

	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestMSP(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MSP Suite")
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
	pqGenerator string
)

var _ = SynchronizedBeforeSuite(func() []byte {
	buildServer = nwo.NewBuildServer()
	buildServer.Serve()

	components = buildServer.Components()
	var err error
	pqGenerator, err = gexec.Build("github.com/hyperledger/fabric/cmd/pactway-pq-gen")
	Expect(err).NotTo(HaveOccurred())
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
	gexec.CleanupBuildArtifacts()
})

func StartPort() int {
	return integration.MSPPort.StartPortForNode()
}
