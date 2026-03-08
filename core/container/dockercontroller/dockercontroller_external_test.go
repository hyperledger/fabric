/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dockercontroller_test

import (
	"io"
	"net"

	"github.com/hyperledger/fabric/core/container/dockercontroller"
	dcli "github.com/moby/moby/client"
)

//go:generate counterfeiter -o mock/platform_builder.go --fake-name PlatformBuilder . platformBuilder
type platformBuilder interface {
	dockercontroller.PlatformBuilder
}

//go:generate counterfeiter -o mock/dockerclient.go --fake-name DockerClient . dockerClient
type dockerClient interface {
	dcli.APIClient
}

//go:generate counterfeiter -o mock/readcloser.go --fake-name ReadCloser . readCloser
type readCloser interface {
	io.ReadCloser
}

//go:generate counterfeiter -o mock/conner.go --fake-name Conner . conner
type conner interface {
	net.Conn
}
