/*
Copyright London Stock Exchange 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"runtime"
	"testing"

	"github.com/hyperledger/fabric/common/metadata"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestUtil_DockerfileTemplateParser(t *testing.T) {
	expected := "FROM foo:" + runtime.GOARCH + "-" + metadata.Version
	actual := ParseDockerfileTemplate("FROM foo:$(ARCH)-$(PROJECT_VERSION)")
	assert.Equal(t, expected, actual, "Error parsing Dockerfile Template. Expected \"%s\", got \"%s\"",
		expected, actual)
}

func TestUtil_GetDockerfileFromConfig(t *testing.T) {
	expected := "FROM " + metadata.DockerNamespace + ":" + runtime.GOARCH + "-" + metadata.Version
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
