/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package platforms

import (
	"io"

	dcli "github.com/moby/moby/client"
)

type Builder struct {
	Registry *Registry
	Client   dcli.APIClient
}

func (b *Builder) GenerateDockerBuild(ccType, path string, codePackage io.Reader) (io.Reader, error) {
	return b.Registry.GenerateDockerBuild(ccType, path, codePackage, b.Client)
}
