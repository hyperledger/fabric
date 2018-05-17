/*
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
*/

package platforms

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"

	"strings"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/core/chaincode/platforms/car"
	"github.com/hyperledger/fabric/core/chaincode/platforms/ccmetadata"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/chaincode/platforms/java"
	"github.com/hyperledger/fabric/core/chaincode/platforms/node"
	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Interface for validating the specification and and writing the package for
// the given platform
type Platform interface {
	ValidateSpec(spec *pb.ChaincodeSpec) error
	ValidateDeploymentSpec(spec *pb.ChaincodeDeploymentSpec) error
	GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error)
	GenerateDockerfile(spec *pb.ChaincodeDeploymentSpec) (string, error)
	GenerateDockerBuild(spec *pb.ChaincodeDeploymentSpec, tw *tar.Writer) error
	GetMetadataProvider(spec *pb.ChaincodeDeploymentSpec) ccmetadata.MetadataProvider
}

var logger = flogging.MustGetLogger("chaincode-platform")

// XXX Temporary singleton hack to allow tests to continue to work
var r = NewRegistry()

// Added for unit testing purposes
var _Find = find
var _CUtilWriteBytesToPackage = cutil.WriteBytesToPackage
var _generateDockerfile = r.generateDockerfile
var _generateDockerBuild = r.generateDockerBuild

type Registry struct{}

// TODO, ultimately this should take the platforms as parameters
func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) ValidateSpec(spec *pb.ChaincodeSpec) error {
	p, err := _Find(spec.Type)
	if err != nil {
		return err
	}
	return p.ValidateSpec(spec)
}

func (r *Registry) ValidateDeploymentSpec(spec *pb.ChaincodeDeploymentSpec) error {
	p, err := _Find(spec.ChaincodeSpec.Type)
	if err != nil {
		return err
	}
	return p.ValidateDeploymentSpec(spec)
}

func (r *Registry) GetMetadataProvider(spec *pb.ChaincodeDeploymentSpec) (ccmetadata.MetadataProvider, error) {
	p, err := _Find(spec.ChaincodeSpec.Type)
	if err != nil {
		return nil, err
	}
	return p.GetMetadataProvider(spec), nil
}

// find returns the platform interface for the given platform type
func find(chaincodeType pb.ChaincodeSpec_Type) (Platform, error) {
	switch chaincodeType {
	case pb.ChaincodeSpec_GOLANG:
		return &golang.Platform{}, nil
	case pb.ChaincodeSpec_CAR:
		return &car.Platform{}, nil
	case pb.ChaincodeSpec_JAVA:
		return &java.Platform{}, nil
	case pb.ChaincodeSpec_NODE:
		return &node.Platform{}, nil
	default:
		return nil, fmt.Errorf("Unknown chaincodeType: %s", chaincodeType)
	}
}

func (r *Registry) GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error) {
	platform, err := _Find(spec.Type)
	if err != nil {
		return nil, err
	}

	return platform.GetDeploymentPayload(spec)
}

func (r *Registry) generateDockerfile(platform Platform, cds *pb.ChaincodeDeploymentSpec) ([]byte, error) {

	var buf []string

	// ----------------------------------------------------------------------------------------------------
	// Let the platform define the base Dockerfile
	// ----------------------------------------------------------------------------------------------------
	base, err := platform.GenerateDockerfile(cds)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate platform-specific Dockerfile: %s", err)
	}
	buf = append(buf, base)

	// ----------------------------------------------------------------------------------------------------
	// Add some handy labels
	// ----------------------------------------------------------------------------------------------------
	buf = append(buf, fmt.Sprintf("LABEL %s.chaincode.id.name=\"%s\" \\", metadata.BaseDockerLabel, cds.ChaincodeSpec.ChaincodeId.Name))
	buf = append(buf, fmt.Sprintf("      %s.chaincode.id.version=\"%s\" \\", metadata.BaseDockerLabel, cds.ChaincodeSpec.ChaincodeId.Version))
	buf = append(buf, fmt.Sprintf("      %s.chaincode.type=\"%s\" \\", metadata.BaseDockerLabel, cds.ChaincodeSpec.Type.String()))
	buf = append(buf, fmt.Sprintf("      %s.version=\"%s\" \\", metadata.BaseDockerLabel, metadata.Version))
	buf = append(buf, fmt.Sprintf("      %s.base.version=\"%s\"", metadata.BaseDockerLabel, metadata.BaseVersion))
	// ----------------------------------------------------------------------------------------------------
	// Then augment it with any general options
	// ----------------------------------------------------------------------------------------------------
	//append version so chaincode build version can be compared against peer build version
	buf = append(buf, fmt.Sprintf("ENV CORE_CHAINCODE_BUILDLEVEL=%s", metadata.Version))

	// ----------------------------------------------------------------------------------------------------
	// Finalize it
	// ----------------------------------------------------------------------------------------------------
	contents := strings.Join(buf, "\n")
	logger.Debugf("\n%s", contents)

	return []byte(contents), nil
}

type InputFiles map[string][]byte

func (r *Registry) generateDockerBuild(platform Platform, cds *pb.ChaincodeDeploymentSpec, inputFiles InputFiles, tw *tar.Writer) error {

	var err error

	// ----------------------------------------------------------------------------------------------------
	// First stream out our static inputFiles
	// ----------------------------------------------------------------------------------------------------
	for name, data := range inputFiles {
		err = _CUtilWriteBytesToPackage(name, data, tw)
		if err != nil {
			return fmt.Errorf("Failed to inject \"%s\": %s", name, err)
		}
	}

	// ----------------------------------------------------------------------------------------------------
	// Now give the platform an opportunity to contribute its own context to the build
	// ----------------------------------------------------------------------------------------------------
	err = platform.GenerateDockerBuild(cds, tw)
	if err != nil {
		return fmt.Errorf("Failed to generate platform-specific docker build: %s", err)
	}

	return nil
}

func (r *Registry) GenerateDockerBuild(cds *pb.ChaincodeDeploymentSpec) (io.Reader, error) {

	inputFiles := make(InputFiles)

	// ----------------------------------------------------------------------------------------------------
	// Determine our platform driver from the spec
	// ----------------------------------------------------------------------------------------------------
	platform, err := _Find(cds.ChaincodeSpec.Type)
	if err != nil {
		return nil, fmt.Errorf("Failed to determine platform type: %s", err)
	}

	// ----------------------------------------------------------------------------------------------------
	// Generate the Dockerfile specific to our context
	// ----------------------------------------------------------------------------------------------------
	dockerFile, err := _generateDockerfile(platform, cds)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate a Dockerfile: %s", err)
	}

	inputFiles["Dockerfile"] = dockerFile

	// ----------------------------------------------------------------------------------------------------
	// Finally, launch an asynchronous process to stream all of the above into a docker build context
	// ----------------------------------------------------------------------------------------------------
	input, output := io.Pipe()

	generateDockerBuild := _generateDockerBuild
	go func() {
		gw := gzip.NewWriter(output)
		tw := tar.NewWriter(gw)
		err := generateDockerBuild(platform, cds, inputFiles, tw)
		if err != nil {
			logger.Error(err)
		}

		tw.Close()
		gw.Close()
		output.CloseWithError(err)
	}()

	return input, nil
}
