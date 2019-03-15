/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/hyperledger/fabric/integration/nwo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Token EndToEnd Suite")
}

var components *nwo.Components
var suiteBase = 30000

var _ = SynchronizedBeforeSuite(func() []byte {
	components = &nwo.Components{}

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

func StartPort() int {
	return suiteBase + (GinkgoParallelNode()-1)*100
}

func ToHex(q uint64) string {
	return "0x" + strconv.FormatUint(q, 16)
}

func ToDecimal(q uint64) string {
	return strconv.FormatUint(q, 10)
}

func BasicSoloV20() *nwo.Config {
	basicSolo := nwo.BasicSolo()
	for _, profile := range basicSolo.Profiles {
		if profile.Consortium != "" {
			profile.AppCapabilities = []string{"V2_0"}
		}
	}
	return basicSolo
}
