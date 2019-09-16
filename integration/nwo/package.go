/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	. "github.com/onsi/gomega"
)

// PackageChaincodeBinary is a helper function to package
// an already built chaincode and write it to the location
// specified by Chaincode.PackageFile.
func PackageChaincodeBinary(c Chaincode) {
	tarGzBytes := getTarGzBytes(c)
	err := ioutil.WriteFile(c.PackageFile, tarGzBytes, 0100644)
	Expect(err).NotTo(HaveOccurred())
}

func getTarGzBytes(c Chaincode) []byte {
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)
	_, normalizedPath := filepath.Split(c.Path)
	metadataBytes := toJSON(normalizedPath, "binary", c.Label)
	writeBytesToPackage(tw, "metadata.json", metadataBytes)
	codeBytes := getCodeBytes(c.Path)
	writeBytesToPackage(tw, "code.tar.gz", codeBytes)
	err := tw.Close()
	Expect(err).NotTo(HaveOccurred())
	err = gw.Close()
	Expect(err).NotTo(HaveOccurred())
	return payload.Bytes()
}

func getCodeBytes(path string) []byte {
	// read the compiled chaincode located at the specified path
	// and tar it with the name "chaincode"
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)
	err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "chaincode",
		Mode:     0100700,
	})
	Expect(err).NotTo(HaveOccurred())
	err = util.WriteFileToPackage(path, "chaincode", tw)
	Expect(err).NotTo(HaveOccurred())
	err = tw.Close()
	Expect(err).NotTo(HaveOccurred())
	err = gw.Close()
	return payload.Bytes()
}

func writeBytesToPackage(tw *tar.Writer, name string, payload []byte) {
	err := tw.WriteHeader(&tar.Header{
		Name: name,
		Size: int64(len(payload)),
		Mode: 0100644,
	})
	Expect(err).NotTo(HaveOccurred())
	_, err = tw.Write(payload)
	Expect(err).NotTo(HaveOccurred())
}

// packageMetadata holds the path and type for a chaincode package
type packageMetadata struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

func toJSON(path, ccType, label string) []byte {
	metadata := &packageMetadata{
		Path:  path,
		Type:  ccType,
		Label: label,
	}
	metadataBytes, err := json.Marshal(metadata)
	Expect(err).NotTo(HaveOccurred())
	return metadataBytes
}
