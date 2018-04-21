/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package java_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"strings"

	"io/ioutil"
	"os"

	"github.com/hyperledger/fabric/core/chaincode/platforms/java"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

const chaincodePathFolder = "testdata"
const chaincodePath = chaincodePathFolder + "/chaincode.jar"

const expected = `
ADD codepackage.tgz /root/chaincode-java/chaincode`

var spec = &pb.ChaincodeSpec{
	Type: pb.ChaincodeSpec_JAVA,
	ChaincodeId: &pb.ChaincodeID{
		Name: "ssample",
		Path: chaincodePath},
	Input: &pb.ChaincodeInput{
		Args: [][]byte{[]byte("f")}}}

func TestValidateSpec(t *testing.T) {
	platform := java.Platform{}

	err := platform.ValidateSpec(spec)
	assert.NoError(t, err)
}

func TestValidateDeploymentSpec(t *testing.T) {
	platform := java.Platform{}
	err := platform.ValidateSpec(spec)
	assert.NoError(t, err)
}

func TestGetDeploymentPayload(t *testing.T) {
	platform := java.Platform{}
	spec.ChaincodeId.Path = "pathdoesnotexist"

	_, err := platform.GetDeploymentPayload(spec)
	assert.Contains(t, err.Error(), "no such file or directory")

	spec.ChaincodeId.Path = chaincodePath
	_, err = os.Stat(chaincodePath)
	if os.IsNotExist(err) {
		createTestJar(t)
		defer os.RemoveAll(chaincodePathFolder)
	}

	payload, err := platform.GetDeploymentPayload(spec)
	assert.NoError(t, err)
	assert.NotZero(t, len(payload))

	is := bytes.NewReader(payload)
	gr, err := gzip.NewReader(is)
	if err != nil {
		assert.Failf(t, "Can't open zip stream %s", err.Error())
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err != nil {
			break
		}

		if strings.Contains(header.Name, "chaincode.jar") {
			return
		}
	}
	assert.NoError(t, err, "Error while looking for jar in tar file")
	assert.Fail(t, "Didn't find expected jar")

}

func TestGenerateDockerfile(t *testing.T) {
	platform := java.Platform{}

	spec.ChaincodeId.Path = chaincodePath
	_, err := os.Stat(chaincodePath)
	if os.IsNotExist(err) {
		createTestJar(t)
		defer os.RemoveAll(chaincodePathFolder)
	}
	payload, err := platform.GetDeploymentPayload(spec)
	if err != nil {
		t.Fatalf("failed to get Java CC payload: %s", err)
	}
	cds := &pb.ChaincodeDeploymentSpec{
		CodePackage: payload}

	dockerfile, err := platform.GenerateDockerfile(cds)
	assert.NoError(t, err)
	assert.Equal(t, expected, dockerfile)
}

func TestGenerateDockerBuild(t *testing.T) {
	platform := java.Platform{}
	cds := &pb.ChaincodeDeploymentSpec{
		CodePackage: []byte{}}
	tw := tar.NewWriter(gzip.NewWriter(bytes.NewBuffer(nil)))

	err := platform.GenerateDockerBuild(cds, tw)
	assert.NoError(t, err)
}

func createTestJar(t *testing.T) {
	os.MkdirAll(chaincodePathFolder, 0755)
	// No need for real jar at this point, so any binary file will work
	err := ioutil.WriteFile(chaincodePath, []byte("Hello, world"), 0644)
	if err != nil {
		assert.Fail(t, "Can't create test jar file", err.Error())
	}
}
