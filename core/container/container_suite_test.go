/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package container_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/container"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/vm_provider.go --fake-name VMProvider . vmProvider
type vmProvider interface {
	container.VMProvider
}

//go:generate counterfeiter -o mock/vm.go --fake-name VM . vm
type vm interface {
	container.VM
}

//go:generate counterfeiter -o mock/vm_req.go --fake-name VMCReq . vmcReq
type vmcReq interface {
	container.VMCReq
}

//go:generate counterfeiter -o mock/builder.go --fake-name Builder . builder
type builder interface {
	container.Builder
}

func TestContainer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Container Suite")
}
