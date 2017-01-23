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

package platforms

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"strings"

	"github.com/hyperledger/fabric/core/chaincode/platforms/car"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/chaincode/platforms/java"
	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

// Interface for validating the specification and and writing the package for
// the given platform
type Platform interface {
	ValidateSpec(spec *pb.ChaincodeSpec) error
	GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error)
	GenerateDockerBuild(spec *pb.ChaincodeDeploymentSpec, tw *tar.Writer) (string, error)
}

var logger = logging.MustGetLogger("chaincode-platform")

// Find returns the platform interface for the given platform type
func Find(chaincodeType pb.ChaincodeSpec_Type) (Platform, error) {

	switch chaincodeType {
	case pb.ChaincodeSpec_GOLANG:
		return &golang.Platform{}, nil
	case pb.ChaincodeSpec_CAR:
		return &car.Platform{}, nil
	case pb.ChaincodeSpec_JAVA:
		return &java.Platform{}, nil
	default:
		return nil, fmt.Errorf("Unknown chaincodeType: %s", chaincodeType)
	}

}

func GetDeploymentPayload(spec *pb.ChaincodeSpec) ([]byte, error) {
	platform, err := Find(spec.Type)
	if err != nil {
		return nil, err
	}

	return platform.GetDeploymentPayload(spec)
}

func generateDockerBuild(cds *pb.ChaincodeDeploymentSpec, tw *tar.Writer) error {

	platform, err := Find(cds.ChaincodeSpec.Type)
	if err != nil {
		return fmt.Errorf("Failed to determine platform type: %s", err)
	}

	dockerFileBase, err := platform.GenerateDockerBuild(cds, tw)
	if err != nil {
		return fmt.Errorf("Failed to generate platform-specific docker build: %s", err)
	}

	var buf []string

	buf = append(buf, dockerFileBase)

	// NOTE: We bake the peer TLS certificate in at the time we build the chaincode container if a cert is
	// found, regardless of whether TLS is enabled or not.  The main implication is that if the adminstrator
	// updates the peer cert, the chaincode containers will need to be invalidated and rebuilt.
	// We will manage enabling or disabling TLS at container run time via CORE_PEER_TLS_ENABLED
	hostTLSPath := viper.GetString("peer.tls.cert.file")
	if _, err = os.Stat(hostTLSPath); err == nil {

		const guestTLSPath = "/etc/hyperledger/fabric/peer.crt"

		buf = append(buf, "ENV CORE_PEER_TLS_CERT_FILE="+guestTLSPath)
		buf = append(buf, "COPY peer.crt "+guestTLSPath)

		err = cutil.WriteFileToPackage(hostTLSPath, "peer.crt", tw)
		if err != nil {
			return fmt.Errorf("Failed to inject peer certificate: %s", err)
		}
	}

	dockerFileContents := strings.Join(buf, "\n")

	err = cutil.WriteBytesToPackage("Dockerfile", []byte(dockerFileContents), tw)
	if err != nil {
		return fmt.Errorf("Failed to inject Dockerfile: %s", err)
	}

	return nil
}

func GenerateDockerBuild(cds *pb.ChaincodeDeploymentSpec) (io.Reader, error) {

	input, output := io.Pipe()

	go func() {
		gw := gzip.NewWriter(output)
		tw := tar.NewWriter(gw)

		err := generateDockerBuild(cds, tw)
		if err != nil {
			logger.Error(err)
		}

		tw.Close()
		gw.Close()
		output.CloseWithError(err)
	}()

	return input, nil
}
