/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package java_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/platforms/java"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

var chaincodePath = "testdata/SimpleSample"
var spec = &pb.ChaincodeSpec{
	Type: pb.ChaincodeSpec_JAVA,
	ChaincodeId: &pb.ChaincodeID{
		Name: "ssample",
		Path: chaincodePath},
	Input: &pb.ChaincodeInput{
		Args: [][]byte{[]byte("f")}}}

var expected = `
ADD codepackage.tgz /root/chaincode
RUN  cd /root/chaincode/src && gradle -b build.gradle clean && gradle -b build.gradle build
RUN  cp /root/chaincode/src/build/chaincode.jar /root
RUN  cp /root/chaincode/src/build/libs/* /root/libs`

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
	assert.Contains(t, err.Error(), "code does not exist")

	spec.ChaincodeId.Path = chaincodePath
	payload, err := platform.GetDeploymentPayload(spec)
	assert.NoError(t, err)
	assert.NotZero(t, len(payload))
}

func TestGenerateDockerfile(t *testing.T) {
	platform := java.Platform{}
	cds := &pb.ChaincodeDeploymentSpec{
		CodePackage: []byte{}}

	_, err := platform.GenerateDockerfile(cds)
	assert.Error(t, err)

	spec.ChaincodeId.Path = chaincodePath
	payload, err := platform.GetDeploymentPayload(spec)
	if err != nil {
		t.Fatalf("failed to get Java CC payload: %s", err)
	}
	cds.CodePackage = payload

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
