/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

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

func copyFile(src, dest string) {
	data, err := ioutil.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(dest, data, 0775)
	Expect(err).NotTo(HaveOccurred())
}

func execute(r ifrit.Runner) (err error) {
	p := ifrit.Invoke(r)
	EventuallyWithOffset(1, p.Ready()).Should(BeClosed())
	EventuallyWithOffset(1, p.Wait(), 5*time.Second).Should(Receive(&err))
	return err
}

func copyDir(src, dest string) {
	os.MkdirAll(dest, 0755)
	objects, err := ioutil.ReadDir(src)
	for _, obj := range objects {
		srcfileptr := src + "/" + obj.Name()
		destfileptr := dest + "/" + obj.Name()
		if obj.IsDir() {
			copyDir(srcfileptr, destfileptr)
		} else {
			copyFile(srcfileptr, destfileptr)
		}
	}
	Expect(err).NotTo(HaveOccurred())
}
