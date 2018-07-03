/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package persistence_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/persistence"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/iowriter.go -fake-name IOWriter . ioWriter
type ioWriter interface {
	persistence.IOWriter
}

func TestPersistence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Persistence Suite")
}
