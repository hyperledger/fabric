/*
Copyright London Stock Exchange 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"runtime"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/core/config"
	"github.com/spf13/viper"
)

//NewDockerClient creates a docker client
func NewDockerClient() (client *docker.Client, err error) {
	endpoint := viper.GetString("vm.endpoint")
	tlsenabled := viper.GetBool("vm.docker.tls.enabled")
	if tlsenabled {
		cert := config.GetPath("vm.docker.tls.cert.file")
		key := config.GetPath("vm.docker.tls.key.file")
		ca := config.GetPath("vm.docker.tls.ca.file")
		client, err = docker.NewTLSClient(endpoint, cert, key, ca)
	} else {
		client, err = docker.NewClient(endpoint)
	}
	return
}

func ParseDockerfileTemplate(template string) string {
	r := strings.NewReplacer(
		"$(ARCH)", runtime.GOARCH,
		"$(PROJECT_VERSION)", metadata.Version,
		"$(TWO_DIGIT_VERSION)", twoDigitVersion(metadata.Version),
		"$(BASE_VERSION)", metadata.BaseVersion,
		"$(DOCKER_NS)", metadata.DockerNamespace,
		"$(BASE_DOCKER_NS)", metadata.BaseDockerNamespace)

	return r.Replace(template)
}

// twoDigitVersion truncates a 3 digit version (e.g. 2.0.0) to a 2 digit version (e.g. 2.0),
// If version does not include dots (e.g. latest), just return the passed version
func twoDigitVersion(version string) string {
	if strings.LastIndex(version, ".") < 0 {
		return version
	}
	return version[0:strings.LastIndex(version, ".")]
}

func GetDockerfileFromConfig(path string) string {
	return ParseDockerfileTemplate(viper.GetString(path))
}
