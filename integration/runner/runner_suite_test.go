/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"encoding/json"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/world"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"

	"testing"
)

func TestRunner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Runner Suite")
}

var (
	components *world.Components
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
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	components.Cleanup()
})

func ContainerExists(client *docker.Client, name string) func() bool {
	return func() bool {
		_, err := client.InspectContainer(name)
		if err != nil {
			_, ok := err.(*docker.NoSuchContainer)
			return !ok
		}
		return false
	}
}

func execute(r ifrit.Runner) error {
	var err error
	p := ifrit.Invoke(r)
	EventuallyWithOffset(1, p.Ready()).Should(BeClosed())
	EventuallyWithOffset(1, p.Wait(), 30*time.Second).Should(Receive(&err))
	return err
}
