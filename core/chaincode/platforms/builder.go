/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package platforms

import (
	"io"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/core/common/ccprovider"
)

type Builder struct {
	Registry *Registry
	Client   *docker.Client
}

func (b *Builder) GenerateDockerBuild(ccci *ccprovider.ChaincodeContainerInfo, codePackage []byte) (io.Reader, error) {
	return b.Registry.GenerateDockerBuild(ccci.Type, ccci.Path, ccci.Name, ccci.Version, codePackage, b.Client)
}
