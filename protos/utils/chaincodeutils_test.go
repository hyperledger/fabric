/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils_test

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChaincodeUtils", func() {
	var _ = Describe("ConvertCDSToChaincodeInstallPackage", func() {
		var cds *peer.ChaincodeDeploymentSpec

		BeforeEach(func() {
			cds = createChaincodeDeploymentSpec("testcc", "version1", "codepath", "GOLANG", []byte("codefortest"))
		})

		It("converts a ChaincodeDeploymentSpec to a ChaincodeInstallPackage", func() {
			name, version, cip, err := utils.ConvertCDSToChaincodeInstallPackage(cds)

			Expect(err).To(BeNil())
			Expect(name).To(Equal("testcc"))
			Expect(version).To(Equal("version1"))
			Expect(cip.CodePackage).To(Equal([]byte("codefortest")))
			Expect(cip.Path).To(Equal("codepath"))
			Expect(cip.Type).To(Equal("GOLANG"))
		})

		Context("when the ChaincodeSpec is nil", func() {
			BeforeEach(func() {
				cds.ChaincodeSpec = nil
			})

			It("returns an error", func() {
				name, version, cip, err := utils.ConvertCDSToChaincodeInstallPackage(cds)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("nil ChaincodeSpec"))
				Expect(name).To(Equal(""))
				Expect(version).To(Equal(""))
				Expect(cip).To(BeNil())
			})
		})

		Context("when the ChaincodeId is nil", func() {
			BeforeEach(func() {
				cds.ChaincodeSpec.ChaincodeId = nil
			})

			It("returns an error", func() {
				name, version, cip, err := utils.ConvertCDSToChaincodeInstallPackage(cds)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("nil ChaincodeId"))
				Expect(name).To(Equal(""))
				Expect(version).To(Equal(""))
				Expect(cip).To(BeNil())
			})
		})
	})

	var _ = Describe("ConvertSignedCDSToChaincodeInstallPackage", func() {
		var scds *peer.SignedChaincodeDeploymentSpec

		BeforeEach(func() {
			cds := createChaincodeDeploymentSpec("testcc", "version1", "codepath", "GOLANG", []byte("codefortest"))
			cdsBytes, err := proto.Marshal(cds)
			Expect(err).NotTo(HaveOccurred())

			scds = &peer.SignedChaincodeDeploymentSpec{
				ChaincodeDeploymentSpec: cdsBytes,
			}
		})

		It("converts a ChaincodeDeploymentSpec to a ChaincodeInstallPackage", func() {
			name, version, cip, err := utils.ConvertSignedCDSToChaincodeInstallPackage(scds)

			Expect(err).To(BeNil())
			Expect(name).To(Equal("testcc"))
			Expect(version).To(Equal("version1"))
			Expect(cip.CodePackage).To(Equal([]byte("codefortest")))
			Expect(cip.Path).To(Equal("codepath"))
			Expect(cip.Type).To(Equal("GOLANG"))
		})

		Context("when the ChaincodeDeploymentSpec bytes fail to unmarshal", func() {
			BeforeEach(func() {
				scds.ChaincodeDeploymentSpec = []byte("garbage")
			})

			It("returns an error", func() {
				name, version, cip, err := utils.ConvertSignedCDSToChaincodeInstallPackage(scds)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error unmarshaling ChaincodeDeploymentSpec"))
				Expect(name).To(Equal(""))
				Expect(version).To(Equal(""))
				Expect(cip).To(BeNil())
			})
		})
	})
})

func createChaincodeDeploymentSpec(name, version, path, ccType string, codePackage []byte) *peer.ChaincodeDeploymentSpec {
	spec := createChaincodeSpec(name, version, path, ccType)
	cds := &peer.ChaincodeDeploymentSpec{
		ChaincodeSpec: spec,
		CodePackage:   codePackage,
	}

	return cds
}

func createChaincodeSpec(name, version, path, ccType string) *peer.ChaincodeSpec {
	spec := &peer.ChaincodeSpec{
		Type: peer.ChaincodeSpec_Type(peer.ChaincodeSpec_Type_value[ccType]),
		ChaincodeId: &peer.ChaincodeID{
			Path:    path,
			Name:    name,
			Version: version,
		},
	}

	return spec
}
