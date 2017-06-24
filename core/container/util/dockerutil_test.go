/*
Copyright London Stock Exchange 2016 All Rights Reserved.

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

package util

import (
	"testing"

	"github.com/hyperledger/fabric/common/metadata"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestUtil_DockerfileTemplateParser(t *testing.T) {
	expected := "FROM foo:" + getArch() + "-" + metadata.Version
	actual := ParseDockerfileTemplate("FROM foo:$(ARCH)-$(PROJECT_VERSION)")
	assert.Equal(t, expected, actual, "Error parsing Dockerfile Template. Expected \"%s\", got \"%s\"",
		expected, actual)
}

func TestUtil_GetDockerfileFromConfig(t *testing.T) {
	expected := "FROM " + metadata.DockerNamespace + ":" + getArch() + "-" + metadata.Version
	path := "dt"
	viper.Set(path, "FROM $(DOCKER_NS):$(ARCH)-$(PROJECT_VERSION)")
	actual := GetDockerfileFromConfig(path)
	assert.Equal(t, expected, actual, "Error parsing Dockerfile Template. Expected \"%s\", got \"%s\"",
		expected, actual)
}

func TestUtil_GetDockertClient(t *testing.T) {
	viper.Set("vm.endpoint", "unix:///var/run/docker.sock")
	_, err := NewDockerClient()
	assert.NoError(t, err, "Error getting docker client")
}
