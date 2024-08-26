/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dispatcher_test

import (
	lc "github.com/hyperledger/fabric-protos-go-apiv2/peer/lifecycle"
	"github.com/hyperledger/fabric/core/dispatcher"
	. "github.com/hyperledger/fabric/internal/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("ProtobufImpl", func() {
	var (
		pi        *dispatcher.ProtobufImpl
		sampleMsg *lc.InstallChaincodeArgs
	)

	BeforeEach(func() {
		pi = &dispatcher.ProtobufImpl{}
		sampleMsg = &lc.InstallChaincodeArgs{
			ChaincodeInstallPackage: []byte("install-package"),
		}
	})

	Describe("Marshal", func() {
		It("passes through to the proto implementation", func() {
			res, err := pi.Marshal(sampleMsg)
			Expect(err).NotTo(HaveOccurred())

			msg := &lc.InstallChaincodeArgs{}
			err = proto.Unmarshal(res, msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg).To(ProtoEqual(sampleMsg))
		})
	})

	Describe("Unmarshal", func() {
		It("passes through to the proto implementation", func() {
			res, err := proto.Marshal(sampleMsg)
			Expect(err).NotTo(HaveOccurred())

			msg := &lc.InstallChaincodeArgs{}
			err = pi.Unmarshal(res, msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg).To(ProtoEqual(sampleMsg))
		})
	})
})
