/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package client_test

import (
	"testing"

	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Suite")
}

func ProtoMarshal(m proto.Message) []byte {
	bytes, err := proto.Marshal(m)
	Expect(err).NotTo(HaveOccurred())

	return bytes
}
