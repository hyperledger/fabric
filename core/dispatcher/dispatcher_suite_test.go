/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dispatcher_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/dispatcher"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/protobuf.go --fake-name Protobuf . protobuf
type protobuf interface {
	dispatcher.Protobuf
}

func TestDispatcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dispatcher Suite")
}
