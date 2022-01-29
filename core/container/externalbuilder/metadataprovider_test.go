/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder_test

import (
	"archive/tar"
	"bytes"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/container/externalbuilder"
)

var _ = Describe("Metadataprovider", func() {
	var mp *externalbuilder.MetadataProvider

	BeforeEach(func() {
		mp = &externalbuilder.MetadataProvider{
			DurablePath: "testdata",
		}
	})

	It("packages up the metadata", func() {
		data, err := mp.PackageMetadata("persisted_build")
		Expect(err).NotTo(HaveOccurred())
		tr := tar.NewReader(bytes.NewBuffer(data))

		headerSizes := map[string]int64{
			"META-INF/":           0,
			"META-INF/index.json": 3,
		}

		headers := 0
		for {
			header, err := tr.Next()
			if err != nil {
				Expect(err).To(Equal(io.EOF))
				break
			}

			headers++
			By("checking " + header.Name)
			size, ok := headerSizes[header.Name]
			Expect(ok).To(BeTrue())
			Expect(size).To(Equal(header.Size))
		}

		Expect(headers).To(Equal(2))
	})

	When("the build does not exist", func() {
		It("returns nil", func() {
			data, err := mp.PackageMetadata("fake_missing_build")
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(BeNil())
		})
	})

	When("the build is not a directory", func() {
		It("returns an error", func() {
			_, err := mp.PackageMetadata("normal_archive.tar.gz")
			Expect(err).To(MatchError("could not stat path: stat testdata/normal_archive.tar.gz/release: not a directory"))
		})
	})
})
