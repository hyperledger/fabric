/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"

	. "github.com/onsi/gomega"
)

// PackageChaincodeBinary is a helper function to package
// an already built chaincode and write it to the location
// specified by Chaincode.PackageFile.
func PackageChaincodeBinary(c Chaincode) {
	file, err := os.Create(c.PackageFile)
	Expect(err).NotTo(HaveOccurred())
	defer file.Close()
	writeTarGz(c, file)
}

func writeTarGz(c Chaincode, w io.Writer) {
	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)
	defer closeAll(tw, gw)

	writeMetadataJSON(tw, c.Path, c.Lang, c.Label)

	writeCodeTarGz(tw, c.CodeFiles)
}

// packageMetadata holds the path, type, and label for a chaincode package
type packageMetadata struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

func writeMetadataJSON(tw *tar.Writer, path, ccType, label string) {
	metadata, err := json.Marshal(&packageMetadata{
		Path:  path,
		Type:  ccType,
		Label: label,
	})
	Expect(err).NotTo(HaveOccurred())

	// write it to the package as metadata.json
	err = tw.WriteHeader(&tar.Header{
		Name: "metadata.json",
		Size: int64(len(metadata)),
		Mode: 0o100644,
	})
	Expect(err).NotTo(HaveOccurred())
	_, err = tw.Write(metadata)
	Expect(err).NotTo(HaveOccurred())
}

func writeCodeTarGz(tw *tar.Writer, codeFiles map[string]string) {
	// create temp file to hold code.tar.gz
	tempfile, err := ioutil.TempFile("", "code.tar.gz")
	Expect(err).NotTo(HaveOccurred())
	defer os.Remove(tempfile.Name())

	gzipWriter := gzip.NewWriter(tempfile)
	tarWriter := tar.NewWriter(gzipWriter)

	for source, target := range codeFiles {
		file, err := os.Open(source)
		Expect(err).NotTo(HaveOccurred())
		writeFileToTar(tarWriter, file, target)
		file.Close()
	}

	// close down the inner tar
	closeAll(tarWriter, gzipWriter)

	writeFileToTar(tw, tempfile, "code.tar.gz")
}

func writeFileToTar(tw *tar.Writer, file *os.File, name string) {
	_, err := file.Seek(0, 0)
	Expect(err).NotTo(HaveOccurred())

	fi, err := file.Stat()
	Expect(err).NotTo(HaveOccurred())
	header, err := tar.FileInfoHeader(fi, "")
	Expect(err).NotTo(HaveOccurred())

	header.Name = name
	err = tw.WriteHeader(header)
	Expect(err).NotTo(HaveOccurred())

	_, err = io.Copy(tw, file)
	Expect(err).NotTo(HaveOccurred())
}

func closeAll(closers ...io.Closer) {
	for _, c := range closers {
		Expect(c.Close()).To(Succeed())
	}
}
