/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package car_test

import (
	"bytes"
	"fmt"

	"github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/car"
	"github.com/hyperledger/fabric/core/container"
	cutil "github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// VM implementation of VM management functionality.
type VM struct {
	Client *docker.Client
}

// NewVM creates a new VM instance.
func NewVM() (*VM, error) {
	client, err := cutil.NewDockerClient()
	if err != nil {
		return nil, err
	}
	VM := &VM{Client: client}
	return VM, nil
}

// BuildChaincodeContainer builds the container for the supplied chaincode specification
func (vm *VM) BuildChaincodeContainer(spec *pb.ChaincodeSpec) error {
	codePackage, err := container.GetChaincodePackageBytes(platforms.NewRegistry(&car.Platform{}), spec)
	if err != nil {
		return fmt.Errorf("Error getting chaincode package bytes: %s", err)
	}

	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackage}
	dockerSpec, err := platforms.NewRegistry(&car.Platform{}).GenerateDockerBuild(
		cds.CCType(),
		cds.Path(),
		cds.Name(),
		cds.Version(),
		cds.Bytes(),
	)
	if err != nil {
		return fmt.Errorf("Error getting chaincode docker image: %s", err)
	}

	output := bytes.NewBuffer(nil)

	err = vm.Client.BuildImage(docker.BuildImageOptions{
		Name:         spec.ChaincodeId.Name,
		InputStream:  dockerSpec,
		OutputStream: output,
	})
	if err != nil {
		return fmt.Errorf("Error building docker: %s (output = %s)", err, output.String())
	}

	return nil
}
