/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package world_test

import (
	"encoding/json"

	"github.com/hyperledger/fabric/integration/world"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestWorld(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "World Suite")
}

var components *world.Components

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
	//fmt.Printf("\n---\n%s\n---\n", components.Paths)
	components.Cleanup()
})
