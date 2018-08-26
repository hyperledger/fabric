/*
Copyright DTCC 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package java

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/ccmetadata"
	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var logger = flogging.MustGetLogger("java-platform")

// Platform for java chaincodes in java
type Platform struct {
}

// Name returns the name of this platform
func (javaPlatform *Platform) Name() string {
	return pb.ChaincodeSpec_JAVA.String()
}

//ValidatePath validates the java chaincode paths
func (javaPlatform *Platform) ValidatePath(rawPath string) error {
	path, err := url.Parse(rawPath)
	if err != nil || path == nil {
		logger.Errorf("invalid chaincode path %s %v", rawPath, err)
		return fmt.Errorf("invalid path: %s", err)
	}

	return nil
}

func (javaPlatform *Platform) ValidateCodePackage(code []byte) error {
	// FIXME: Java platform needs to implement its own validation similar to GOLANG
	return nil
}

// WritePackage writes the java chaincode package
func (javaPlatform *Platform) GetDeploymentPayload(path string) ([]byte, error) {

	logger.Debugf("Packaging java project from path %s", path)
	var err error

	// --------------------------------------------------------------------------------------
	// Write out our tar package
	// --------------------------------------------------------------------------------------
	payload := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(payload)
	tw := tar.NewWriter(gw)

	folder := path
	if folder == "" {
		logger.Error("ChaincodeSpec's path cannot be empty")
		return nil, errors.New("ChaincodeSpec's path cannot be empty")
	}

	// trim trailing slash if it exists
	if folder[len(folder)-1] == '/' {
		folder = folder[:len(folder)-1]
	}

	if err = cutil.WriteJavaProjectToPackage(tw, folder); err != nil {

		logger.Errorf("Error writing java project to tar package %s", err)
		return nil, fmt.Errorf("Error writing Chaincode package contents: %s", err)
	}

	tw.Close()
	gw.Close()

	return payload.Bytes(), nil
}

func (javaPlatform *Platform) GenerateDockerfile() (string, error) {
	var buf []string

	buf = append(buf, "FROM "+cutil.GetDockerfileFromConfig("chaincode.java.runtime"))
	buf = append(buf, "ADD binpackage.tar /root/chaincode-java/chaincode")

	dockerFileContents := strings.Join(buf, "\n")

	return dockerFileContents, nil
}

func (javaPlatform *Platform) GenerateDockerBuild(path string, code []byte, tw *tar.Writer) error {
	codepackage := bytes.NewReader(code)
	binpackage := bytes.NewBuffer(nil)
	buildOptions := util.DockerBuildOptions{
		Image:        cutil.GetDockerfileFromConfig("chaincode.java.runtime"),
		Cmd:          "./build.sh",
		InputStream:  codepackage,
		OutputStream: binpackage,
	}
	logger.Debugf("Executing docker build %v, %v", buildOptions.Image, buildOptions.Cmd)
	err := util.DockerBuild(buildOptions)
	if err != nil {
		logger.Errorf("Can't build java chaincode %v", err)
		return err
	}

	resultBytes := binpackage.Bytes()
	return cutil.WriteBytesToPackage("binpackage.tar", resultBytes, tw)
}

//GetMetadataProvider fetches metadata provider given deployment spec
func (javaPlatform *Platform) GetMetadataProvider(code []byte) platforms.MetadataProvider {
	return &ccmetadata.TargzMetadataProvider{Code: code}
}
