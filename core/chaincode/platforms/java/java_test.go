/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package java_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/java"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

var _ = platforms.Platform(&java.Platform{})

const chaincodePathFolder = "testdata"
const chaincodePathFolderGradle = chaincodePathFolder + "/gradle"

var spec = &pb.ChaincodeSpec{
	Type: pb.ChaincodeSpec_JAVA,
	ChaincodeId: &pb.ChaincodeID{
		Name: "ssample",
		Path: chaincodePathFolderGradle},
	Input: &pb.ChaincodeInput{
		Args: [][]byte{[]byte("f")}}}

func TestValidatePath(t *testing.T) {
	platform := java.Platform{}

	err := platform.ValidatePath(spec.ChaincodeId.Path)
	assert.NoError(t, err)
}

func TestValidateCodePackage(t *testing.T) {
	platform := java.Platform{}
	b, _ := generateMockPackegeBytes("src/pom.xml", 0100400)
	assert.NoError(t, platform.ValidateCodePackage(b))

	b, _ = generateMockPackegeBytes("src/pom.xml", 0100555)
	assert.Error(t, platform.ValidateCodePackage(b))

	b, _ = generateMockPackegeBytes("src/build.gradle", 0100400)
	assert.NoError(t, platform.ValidateCodePackage(b))

	b, _ = generateMockPackegeBytes("src/build.xml", 0100400)
	assert.Error(t, platform.ValidateCodePackage(b))

	b, _ = generateMockPackegeBytes("src/src/Main.java", 0100400)
	assert.NoError(t, platform.ValidateCodePackage(b))

	b, _ = generateMockPackegeBytes("src/build/Main.java", 0100400)
	assert.Error(t, platform.ValidateCodePackage(b))

	b, _ = generateMockPackegeBytes("src/src/xyz/main.java", 0100400)
	assert.NoError(t, platform.ValidateCodePackage(b))

	b, _ = generateMockPackegeBytes("src/src/xyz/main.class", 0100400)
	assert.Error(t, platform.ValidateCodePackage(b))

	b, _ = platform.GetDeploymentPayload(chaincodePathFolderGradle)
	assert.NoError(t, platform.ValidateCodePackage(b))
}

func TestGetDeploymentPayload(t *testing.T) {
	platform := java.Platform{}

	_, err := platform.GetDeploymentPayload("")
	assert.Contains(t, err.Error(), "ChaincodeSpec's path cannot be empty")

	spec.ChaincodeId.Path = chaincodePathFolderGradle

	payload, err := platform.GetDeploymentPayload(chaincodePathFolderGradle)
	assert.NoError(t, err)
	assert.NotZero(t, len(payload))

	buildFileFound := false
	settingsFileFound := false
	pomFileFound := false
	srcFileFound := false

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

		if strings.Contains(header.Name, ".class") {
			assert.Fail(t, "Result package can't contain class file")
		}
		if strings.Contains(header.Name, "target/") {
			assert.Fail(t, "Result package can't contain target folder")
		}
		if strings.Contains(header.Name, "build/") {
			assert.Fail(t, "Result package can't contain build folder")
		}
		if strings.Contains(header.Name, "src/build.gradle") {
			buildFileFound = true
		}
		if strings.Contains(header.Name, "src/settings.gradle") {
			settingsFileFound = true
		}
		if strings.Contains(header.Name, "src/pom.xml") {
			pomFileFound = true
		}
		if strings.Contains(header.Name, "src/main/java/example/ExampleCC.java") {
			srcFileFound = true
		}
	}
	assert.True(t, buildFileFound, "Can't find build.gradle file in tar")
	assert.True(t, settingsFileFound, "Can't find settings.gradle file in tar")
	assert.True(t, pomFileFound, "Can't find pom.xml file in tar")
	assert.True(t, srcFileFound, "Can't find example.cc file in tar")
	assert.NoError(t, err, "Error while scanning tar file")
}

func TestGenerateDockerfile(t *testing.T) {
	platform := java.Platform{}

	spec.ChaincodeId.Path = chaincodePathFolderGradle
	_, err := platform.GetDeploymentPayload(spec.Path())
	if err != nil {
		t.Fatalf("failed to get Java CC payload: %s", err)
	}

	dockerfile, err := platform.GenerateDockerfile()
	assert.NoError(t, err)

	var buf []string

	buf = append(buf, "FROM "+util.GetDockerfileFromConfig("chaincode.java.runtime"))
	buf = append(buf, "ADD binpackage.tar /root/chaincode-java/chaincode")

	dockerFileContents := strings.Join(buf, "\n")

	assert.Equal(t, dockerFileContents, dockerfile)
}

func TestGenerateDockerBuild(t *testing.T) {
	t.Skip()
	platform := java.Platform{}
	ccSpec := &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_JAVA,
		ChaincodeId: &pb.ChaincodeID{Path: chaincodePathFolderGradle},
		Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("init")}}}

	cp, _ := platform.GetDeploymentPayload(ccSpec.Path())

	cds := &pb.ChaincodeDeploymentSpec{
		ChaincodeSpec: ccSpec,
		CodePackage:   cp}

	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	err := platform.GenerateDockerBuild(cds.Path(), cds.Bytes(), tw)
	assert.NoError(t, err)
}

func TestMain(m *testing.M) {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	configtest.AddDevConfigPath(nil)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("could not read config %s\n", err)
		os.Exit(-1)
	}
	os.Exit(m.Run())
}

func generateMockPackegeBytes(fileName string, mode int64) ([]byte, error) {
	var zeroTime time.Time
	codePackage := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(codePackage)
	tw := tar.NewWriter(gw)
	payload := make([]byte, 25, 25)
	err := tw.WriteHeader(&tar.Header{Name: fileName, Size: int64(len(payload)), ModTime: zeroTime, AccessTime: zeroTime, ChangeTime: zeroTime, Mode: mode})
	if err != nil {
		return nil, err
	}
	_, err = tw.Write(payload)
	if err != nil {
		return nil, err
	}
	tw.Close()
	gw.Close()
	return codePackage.Bytes(), nil
}
