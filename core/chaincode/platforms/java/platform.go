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
	"io"
	"net/url"
	"regexp"
	"strings"

	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms/util"
)

var logger = flogging.MustGetLogger("chaincode.platform.java")

// Platform for java chaincodes in java
type Platform struct{}

// Name returns the name of this platform
func (p *Platform) Name() string {
	return pb.ChaincodeSpec_JAVA.String()
}

// ValidatePath validates the java chaincode paths
func (p *Platform) ValidatePath(rawPath string) error {
	path, err := url.Parse(rawPath)
	if err != nil || path == nil {
		logger.Errorf("invalid chaincode path %s %v", rawPath, err)
		return fmt.Errorf("invalid path: %s", err)
	}

	return nil
}

func (p *Platform) ValidateCodePackage(code []byte) error {
	// File to be valid should match first RegExp and not match second one.
	filesToMatch := regexp.MustCompile(`^(/)?src/((src|META-INF)/.*|(build\.gradle|settings\.gradle|pom\.xml))`)
	filesToIgnore := regexp.MustCompile(`.*\.class$`)
	is := bytes.NewReader(code)
	gr, err := gzip.NewReader(is)
	if err != nil {
		return fmt.Errorf("failure opening codepackage gzip stream: %s", err)
	}
	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// --------------------------------------------------------------------------------------
		// Check name for conforming path
		// --------------------------------------------------------------------------------------
		if !filesToMatch.MatchString(header.Name) || filesToIgnore.MatchString(header.Name) {
			return fmt.Errorf("illegal file detected in payload: \"%s\"", header.Name)
		}

		// --------------------------------------------------------------------------------------
		// Check that file mode makes sense
		// --------------------------------------------------------------------------------------
		// Acceptable flags:
		//      ISREG      == 0100000
		//      -rw-rw-rw- == 0666
		//
		// Anything else is suspect in this context and will be rejected
		// --------------------------------------------------------------------------------------
		if header.Mode&^0o100666 != 0 {
			return fmt.Errorf("illegal file mode detected for file %s: %o", header.Name, header.Mode)
		}
	}
	return nil
}

// WritePackage writes the java chaincode package
func (p *Platform) GetDeploymentPayload(path string) ([]byte, error) {
	logger.Debugf("Packaging java project from path %s", path)

	if path == "" {
		logger.Error("ChaincodeSpec's path cannot be empty")
		return nil, errors.New("ChaincodeSpec's path cannot be empty")
	}

	// trim trailing slash if it exists
	if path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	buf := &bytes.Buffer{}
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	excludedDirs := []string{"target", "build", "out"}
	excludedFileTypes := map[string]bool{".class": true}
	err := util.WriteFolderToTarPackage(tw, path, excludedDirs, nil, excludedFileTypes)
	if err != nil {
		logger.Errorf("Error writing java project to tar package %s", err)
		return nil, fmt.Errorf("failed to create chaincode package: %s", err)
	}

	tw.Close()
	gw.Close()

	return buf.Bytes(), nil
}

func (p *Platform) GenerateDockerfile() (string, error) {
	var buf []string

	buf = append(buf, "FROM "+util.GetDockerImageFromConfig("chaincode.java.runtime"))
	buf = append(buf, "ADD binpackage.tar /root/chaincode-java/chaincode")

	dockerFileContents := strings.Join(buf, "\n")

	return dockerFileContents, nil
}

func (p *Platform) DockerBuildOptions(path string) (util.DockerBuildOptions, error) {
	return util.DockerBuildOptions{
		Image: util.GetDockerImageFromConfig("chaincode.java.runtime"),
		Cmd:   "./build.sh",
	}, nil
}
