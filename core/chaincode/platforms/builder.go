/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package platforms

import (
	"io"

	docker "github.com/fsouza/go-dockerclient"
)

type Builder struct {
	Registry *Registry
	Client   *docker.Client
}

func (b *Builder) GenerateDockerBuild(ccType, path string, codePackage io.Reader) (io.Reader, error) {
	return b.Registry.GenerateDockerBuild(ccType, path, codePackage, b.Client)
}
