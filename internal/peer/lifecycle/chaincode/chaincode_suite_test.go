/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"

	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/writer.go -fake-name Writer . writer
type writer interface {
	chaincode.Writer
}

//go:generate counterfeiter -o mock/platform_registry.go -fake-name PlatformRegistry . platformRegistryIntf
type platformRegistryIntf interface {
	chaincode.PlatformRegistry
}

//go:generate counterfeiter -o mock/reader.go -fake-name Reader . reader
type reader interface {
	chaincode.Reader
}

//go:generate counterfeiter -o mock/endorser_client.go -fake-name EndorserClient . endorserClient
type endorserClient interface {
	chaincode.EndorserClient
}

//go:generate counterfeiter -o mock/signer.go -fake-name Signer . signer
type signer interface {
	chaincode.Signer
}

func TestChaincode(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Chaincode Suite")
}
