/*
Copyright DTCC 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package java

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/url"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/platforms/ccmetadata"
	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	//	"path/filepath"
)

// Platform for java chaincodes in java
type Platform struct {
}

//ValidateSpec validates the java chaincode specs
func (javaPlatform *Platform) ValidateSpec(spec *pb.ChaincodeSpec) error {
	path, err := url.Parse(spec.ChaincodeId.Path)
	if err != nil || path == nil {
		return fmt.Errorf("invalid path: %s", err)
	}

	return nil
}

func (javaPlatform *Platform) ValidateDeploymentSpec(cds *pb.ChaincodeDeploymentSpec) error {
	// FIXME: Java platform needs to implement its own validation similar to GOLANG
	return nil
}

// WritePackage writes the java chaincode package
func (javaPlatform *Platform) GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error) {

	var err error

	inputbuf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(inputbuf)
	tw := tar.NewWriter(gw)

	//ignore the generated hash. Just use the tw
	//The hash could be used in a future enhancement
	//to check, warn of duplicate installs etc.
	_, err = collectChaincodeFiles(spec, tw)
	if err != nil {
		return nil, err
	}

	err = writeChaincodePackage(spec, tw)

	tw.Close()
	gw.Close()

	if err != nil {
		return nil, err
	}

	payload := inputbuf.Bytes()

	return payload, nil
}

func (javaPlatform *Platform) GenerateDockerfile(cds *pb.ChaincodeDeploymentSpec) (string, error) {
	var buf []string

	buf = append(buf, cutil.GetDockerfileFromConfig("chaincode.java.Dockerfile"))
	buf = append(buf, "ADD codepackage.tgz /root/chaincode-java/chaincode")

	dockerFileContents := strings.Join(buf, "\n")

	return dockerFileContents, nil
}

func (javaPlatform *Platform) GenerateDockerBuild(cds *pb.ChaincodeDeploymentSpec, tw *tar.Writer) error {
	return cutil.WriteBytesToPackage("codepackage.tgz", cds.CodePackage, tw)
}

//GetMetadataProvider fetches metadata provider given deployment spec
func (javaPlatform *Platform) GetMetadataProvider(cds *pb.ChaincodeDeploymentSpec) ccmetadata.MetadataProvider {
	return &ccmetadata.TargzMetadataProvider{cds}
}
