/*
Copyright London Stock Exchange 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"runtime"
	"strings"

	"github.com/hyperledger/fabric/common/metadata"
	"github.com/spf13/viper"
)

func GetDockerfileFromConfig(path string) string {
	r := strings.NewReplacer(
		"$(ARCH)", runtime.GOARCH,
		"$(PROJECT_VERSION)", metadata.Version,
		"$(DOCKER_NS)", metadata.DockerNamespace,
		"$(BASE_DOCKER_NS)", metadata.BaseDockerNamespace)

	return r.Replace(viper.GetString(path))
}
