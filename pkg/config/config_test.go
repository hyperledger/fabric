/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	. "github.com/onsi/gomega"
)

func TestSignConfigUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		gt := NewGomegaWithT(t)

		signer, err := NewSigner([]byte(publicKey), []byte(privateKey), "test-msp")
		gt.Expect(err).NotTo(HaveOccurred())
		configSignature, err := SignConfigUpdate(&common.ConfigUpdate{}, signer)
		gt.Expect(err).NotTo(HaveOccurred())

		expectedCreator, err := signer.Serialize()
		gt.Expect(err).NotTo(HaveOccurred())
		signatureHeader := &common.SignatureHeader{}
		err = proto.Unmarshal(configSignature.SignatureHeader, signatureHeader)
		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(signatureHeader.Creator).To(Equal(expectedCreator))
	})
}
