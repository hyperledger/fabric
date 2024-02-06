/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pluggable

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric/integration"
	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPluggable(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pluggable endorsement and validation EndToEnd Suite")
}

var (
	buildServer *nwo.BuildServer
	components  *nwo.Components
)

var _ = SynchronizedBeforeSuite(func() []byte {
	// need to use the same "generic" build tag for peer as is used for the plugins.
	// "generic" build tag on the plugins is a workaround to ensure github.com/kilic/bls12-381 dependency compiles on amd64, see pluggable_test.go compilePlugin()
	buildServer = nwo.NewBuildServer("-tags=generic")
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
	return integration.PluggableBasePort.StartPortForNode()
}
