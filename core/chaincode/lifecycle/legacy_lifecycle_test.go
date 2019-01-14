/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle_test

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle/mock"
	"github.com/hyperledger/fabric/core/common/ccprovider"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Lifecycle", func() {
	Describe("LegacyShim", func() {
		var (
			l              *lifecycle.Lifecycle
			fakeLegacyImpl *mock.LegacyLifecycle
		)

		BeforeEach(func() {
			fakeLegacyImpl = &mock.LegacyLifecycle{}

			l = &lifecycle.Lifecycle{
				LegacyImpl: fakeLegacyImpl,
			}
		})

		Describe("ChaincodeDefinition", func() {
			var (
				fakeQueryExecutor         *mock.SimpleQueryExecutor
				legacyChaincodeDefinition *ccprovider.ChaincodeData
			)

			BeforeEach(func() {
				fakeQueryExecutor = &mock.SimpleQueryExecutor{}

				legacyChaincodeDefinition = &ccprovider.ChaincodeData{
					Name:    "definition-name",
					Version: "definition-version",
				}

				fakeLegacyImpl.ChaincodeDefinitionReturns(legacyChaincodeDefinition, fmt.Errorf("fake-error"))
			})

			It("passes through the legacy implementation", func() {
				res, err := l.ChaincodeDefinition("cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("fake-error"))
				Expect(res).To(Equal(legacyChaincodeDefinition))
				Expect(fakeLegacyImpl.ChaincodeDefinitionCallCount()).To(Equal(1))
				name, qe := fakeLegacyImpl.ChaincodeDefinitionArgsForCall(0)
				Expect(name).To(Equal("cc-name"))
				Expect(qe).To(Equal(fakeQueryExecutor))
			})
		})

		Describe("ChaincodeContainerInfo", func() {
			var (
				fakeQueryExecutor   *mock.SimpleQueryExecutor
				legacyContainerInfo *ccprovider.ChaincodeContainerInfo
			)

			BeforeEach(func() {
				fakeQueryExecutor = &mock.SimpleQueryExecutor{}

				legacyContainerInfo = &ccprovider.ChaincodeContainerInfo{
					Name:    "definition-name",
					Version: "definition-version",
				}

				fakeLegacyImpl.ChaincodeContainerInfoReturns(legacyContainerInfo, fmt.Errorf("fake-error"))
			})

			It("passes through the legacy implementation", func() {
				res, err := l.ChaincodeContainerInfo("cc-name", fakeQueryExecutor)
				Expect(err).To(MatchError("fake-error"))
				Expect(res).To(Equal(legacyContainerInfo))
				Expect(fakeLegacyImpl.ChaincodeContainerInfoCallCount()).To(Equal(1))
				name, qe := fakeLegacyImpl.ChaincodeContainerInfoArgsForCall(0)
				Expect(name).To(Equal("cc-name"))
				Expect(qe).To(Equal(fakeQueryExecutor))
			})
		})
	})
})
